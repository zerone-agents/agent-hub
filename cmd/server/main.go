package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"control-panel/internal/application/services"
	"control-panel/internal/auth"
	"control-panel/internal/auth/builtin"
	"control-panel/internal/config"
	knowledgedomain "control-panel/internal/domain/knowledge"
	"control-panel/internal/domain/provider"
	"control-panel/internal/handler"
	"control-panel/internal/infrastructure/deployer"
	knowledgeinfra "control-panel/internal/infrastructure/knowledge"
	"control-panel/internal/infrastructure/kong"
	"control-panel/internal/infrastructure/multirag"
	ossinfra "control-panel/internal/infrastructure/oss"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/internal/infrastructure/runtime"
	"control-panel/internal/middleware"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
)

// SPAFileSystem implements http.FileSystem with SPA fallback: serves index.html
// when the requested file is not found.
type SPAFileSystem struct {
	http.FileSystem
}

func (sfs *SPAFileSystem) Open(name string) (http.File, error) {
	f, err := sfs.FileSystem.Open(name)
	if err != nil {
		return sfs.FileSystem.Open("index.html")
	}
	return f, nil
}

//go:embed all:dist
var staticFiles embed.FS

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := database.InitDatabase(&cfg.Database); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	if err := database.AutoMigrate(); err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	if err := cfg.ValidateAuth(); err != nil {
		log.Fatalf("Invalid auth config: %v", err)
	}

	// ==================== 认证装配（按 auth.mode 二选一） ====================
	var authProvider auth.Provider
	var builtinAuthHandler *handler.BuiltinAuthHandler
	var adminUserHandler *handler.AdminUserHandler

	if cfg.Auth.IsBuiltin() {
		userSvc := services.NewUserService(database.GetDB())
		inviteSvc := services.NewInviteService(database.GetDB())
		builtinProvider := builtin.New(database.GetDB(), cfg.Auth.JWTSecret)
		authProvider = builtinProvider
		builtinAuthHandler = handler.NewBuiltinAuthHandler(builtinProvider, userSvc, inviteSvc)
		adminUserHandler = handler.NewAdminUserHandler(userSvc, inviteSvc, builtinProvider)
		log.Println("Auth mode: builtin")
	} else {
		if err := auth.InitCasdoor(&cfg.Casdoor); err != nil {
			log.Fatalf("Failed to initialize Casdoor: %v", err)
		}
		authProvider = auth.NewCasdoorProvider(cfg.Casdoor.RoleMapping, cfg.Casdoor.DefaultRole)
		log.Println("Auth mode: casdoor")
	}

	// OSS is optional: when unconfigured (empty endpoint) the server runs
	// without file-upload support (InitOSS returns nil) rather than refusing
	// to boot. Configured-but-invalid OSS still fails fast.
	uploader, err := ossinfra.InitOSS(&cfg.OSS)
	if err != nil {
		log.Fatalf("Failed to initialize OSS: %v", err)
	}
	if uploader == nil {
		log.Println("OSS not configured — file uploads disabled (set OSS_ENDPOINT/BUCKET/REGION to enable)")
	}

	r := gin.Default()
	r.MaxMultipartMemory = 50 << 20

	r.Use(middleware.Logger())
	r.Use(middleware.Recovery())
	r.Use(middleware.CORS(cfg.Server.CorsOrigins))

	// 静态资源管理
	dist, err := fs.Sub(staticFiles, "dist")
	if err != nil {
		log.Fatalf("Failed to load static files: %v", err)
	}
	r.StaticFS("/static", &SPAFileSystem{http.FS(dist)})

	// ==================== 处理器初始化 ====================

	deployerClient := deployer.NewClient(cfg.Deployer.URL, cfg.Deployer.APIKey)

	// Kong gateway setup. KongClient is nil when AdminURL is empty, making the
	// entire gateway integration a no-op.
	kongClient := kong.NewClient(cfg.Kong.AdminURL)
	kongService := services.NewKongGatewayService(
		kongClient,
		cfg.Deployer.UpstreamHost,
		cfg.Deployer.PublicHost,
		repository.NewAgentRepository(),
		cfg.Kong.ReconcileSec,
	)

	var knowledgeEngine knowledgedomain.KnowledgeEngine
	if cfg.Knowledge.MultiragBaseURL != "" && cfg.Knowledge.MultiragAPIKey != "" {
		knowledgeEngine = knowledgeinfra.NewRemoteMultiragEngine(
			cfg.Knowledge.MultiragBaseURL,
			cfg.Knowledge.MultiragAPIKey,
			time.Duration(cfg.Knowledge.TimeoutSeconds)*time.Second,
			time.Duration(cfg.Knowledge.UploadTimeoutSeconds)*time.Second,
		)
	}
	// ProviderService is constructed before KnowledgeService so the latter
	// can translate local model_ids (e.g. "bge-large-zh") into MultiRAG
	// full-ID format ("bge-large-zh@ZHIPU-AI") before forwarding.
	providerService := services.NewProviderService(cfg.Provider.EncryptionKey)
	knowledgeService := services.NewKnowledgeService(knowledgeEngine, providerService)

	aigcConfigSvc := services.NewAigcConfigService(database.GetDB(), cfg.Provider.EncryptionKey, repository.NewProviderRepository())
	aigcConfigHandler := handler.NewAigcConfigHandler(aigcConfigSvc)
	deployerService := services.NewAgentDeployerService(deployerClient, cfg.Deployer.PublicHost, cfg.OSS.CDNHost, cfg.Provider.EncryptionKey, cfg.Deployer.RuntimeAPIKey, knowledgeService, kongService, aigcConfigSvc)

	agentService := services.NewAgentService(cfg.Provider.EncryptionKey)
	tenantHandler := handler.NewTenantHandler()
	agentHandler := handler.NewAgentHandler(agentService, deployerService)

	// Agent chat: sessions + messages + SSE streaming proxy to runtime
	runtimeClient := runtime.NewClient()
	agentChatSvc := services.NewAgentChatService(
		repository.NewChatRepository(),
		repository.NewAgentRepository(),
		deployerService,
		runtimeClient,
		cfg.Deployer.PublicHost,
		cfg.Deployer.RuntimeAPIKey,
	)
	agentChatHandler := handler.NewAgentChatHandler(agentChatSvc)
	agentDetailHandler := handler.NewAgentDetailHandler(agentChatSvc)
	agentFilesHandler := handler.NewAgentFilesHandler(agentChatSvc)

	toolService := services.NewToolService()
	toolHandler := handler.NewToolHandler(toolService)

	skillService := services.NewSkillService(uploader, cfg.OSS.CDNHost)
	skillHandler := handler.NewSkillHandler(skillService)

	sceneService := services.NewSceneService()
	sceneHandler := handler.NewSceneHandler(sceneService)

	chatHandler := handler.NewChatHandler()

	// MultiRAG sync client: only construct when both base URL and API key
	// are configured. When nil, the sync-multirag endpoint returns 503 and
	// the knowledge /multirag/models proxy returns 503. The same concrete
	// *multirag.Client backs both the sync (provider) and my_llms (knowledge)
	// features — it implements both provider.MultiRAGClient and
	// provider.MultiRAGMyLLMsSource.
	var (
		multiragSync   provider.MultiRAGClient
		multiragMyLLMs provider.MultiRAGMyLLMsSource
	)
	if cfg.Knowledge.MultiragBaseURL != "" && cfg.Knowledge.MultiragAPIKey != "" {
		c := multirag.NewClient(cfg.Knowledge.MultiragBaseURL, cfg.Knowledge.MultiragAPIKey)
		multiragSync = c
		multiragMyLLMs = c
	}
	providerHandler := handler.NewProviderHandler(providerService, multiragSync)

	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeService, multiragMyLLMs)

	// CLI token lifecycle: opaque cli_<hex> tokens for the zhub CLI.
	// Backed by GORM directly (no repository layer needed) — service tests use
	// sqlite for isolation.
	cliTokenSvc := services.NewCLITokenService(database.GetDB())
	cliTokenHandler := handler.NewCLITokenHandler(cliTokenSvc)

	// 首次启动时插入种子数据
	if err := providerService.SeedIfEmpty(); err != nil {
		log.Printf("Warning: provider seed failed: %v", err)
	}
	if err := toolService.SeedIfEmpty(); err != nil {
		log.Printf("Warning: tool seed failed: %v", err)
	}

	mcpService := services.NewMcpService(cfg.Provider.EncryptionKey)
	if err := mcpService.SeedBuiltins(); err != nil {
		log.Fatalf("Failed to seed builtin MCPs: %v", err)
	}
	if err := toolService.SeedBuiltins(); err != nil {
		log.Fatalf("Failed to seed builtin tools: %v", err)
	}
	if err := toolService.BackfillSubagentToolBindings(); err != nil {
		log.Fatalf("Failed to backfill subagent tool bindings: %v", err)
	}
	mcpHandler := handler.NewMcpHandler(mcpService)

	knowledgeMcpHandler := handler.NewKnowledgeMcpHandler(knowledgeService, agentService)

	serviceRouter := handler.NewServiceRouter()

	// ==================== 路由管理 ====================

	// /health
	r.GET("/health", handler.HealthCheck)
	r.GET("/health/:service", handler.ServiceHealthCheck)

	// /auth — endpoints are mode-conditional. builtin mode serves setup/login/
	// register/refresh/change-password locally; casdoor mode keeps the OAuth
	// redirect flow. /auth/mode and /auth/userinfo are common to both.
	authGroup := r.Group("/auth")
	{
		if cfg.Auth.IsBuiltin() {
			rl := middleware.IPRateLimit(10, time.Minute)
			authGroup.GET("/mode", builtinAuthHandler.GetMode)
			authGroup.POST("/setup", rl, builtinAuthHandler.Setup)
			authGroup.POST("/login", rl, builtinAuthHandler.Login)
			authGroup.POST("/refresh", rl, builtinAuthHandler.Refresh)
			authGroup.POST("/logout", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), builtinAuthHandler.Logout)
			authGroup.POST("/register", rl, builtinAuthHandler.Register)
			authGroup.GET("/invite/:token", rl, builtinAuthHandler.InvitePrecheck)
			authGroup.POST("/change-password", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), builtinAuthHandler.ChangePassword)
			authGroup.GET("/userinfo", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), handler.UserInfo)
		} else {
			authGroup.GET("/mode", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"mode": "casdoor", "initialized": true}})
			})
			authGroup.GET("/login", handler.Login)
			authGroup.GET("/callback", handler.Callback)
			authGroup.GET("/userinfo", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), handler.UserInfo)
			authGroup.POST("/logout", middleware.JWTAuth(authProvider), handler.Logout)
			authGroup.POST("/refresh", handler.RefreshToken)
		}
	}

	v1group := r.Group("/api/v1", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider))
	// Business-resource admin routes: admin OR maintainer.
	v1adminGroup := v1group.Group("/admin", middleware.RequireManager())

	// ---------- Tenant 领域 ----------
	tenantsGroup := v1group.Group("/tenants")
	{
		tenantsGroup.GET("", tenantHandler.List)
		tenantsGroup.POST("", tenantHandler.Create)
		tenantsGroup.GET("/:id", tenantHandler.Get)
		tenantsGroup.PUT("/:id", tenantHandler.Update)
		tenantsGroup.DELETE("/:id", tenantHandler.Delete)
	}

	// ---------- Agent 领域 ----------
	// 公开接口
	agentsGroup := v1group.Group("/agents")
	{
		agentsGroup.GET("/manifest", agentHandler.Manifest)
		agentsGroup.GET("", agentHandler.List)
		agentsGroup.GET("/:name", agentHandler.Get)

		// Agent chat (regular user)
		agentsGroup.GET("/:name/chat/sessions", agentChatHandler.ListSessions)
		agentsGroup.POST("/:name/chat/sessions", agentChatHandler.CreateSession)
		agentsGroup.GET("/:name/chat/sessions/:id/messages", agentChatHandler.ListMessages)
		agentsGroup.DELETE("/:name/chat/sessions/:id", agentChatHandler.DeleteSession)
		agentsGroup.POST("/:name/chat/sessions/:id/messages", agentChatHandler.SendMessage)
	}

	// 管理接口
	adminAgentsGroup := v1adminGroup.Group("/agents")
	{
		adminAgentsGroup.GET("", agentHandler.ListAdmin)
		adminAgentsGroup.POST("", agentHandler.Create)
		adminAgentsGroup.PUT("/:name", agentHandler.Update)
		adminAgentsGroup.DELETE("/:name", agentHandler.Delete)
		adminAgentsGroup.PUT("/:name/subagents", agentHandler.UpdateSubagents)
		adminAgentsGroup.GET("/:name/tools", toolHandler.GetAgentTools)
		adminAgentsGroup.PUT("/:name/tools", toolHandler.UpdateAgentTools)
		adminAgentsGroup.GET("/:name/skills", skillHandler.GetAgentSkills)
		adminAgentsGroup.PUT("/:name/skills", skillHandler.UpdateAgentSkills)
		adminAgentsGroup.GET("/:name/mcps", mcpHandler.GetAgentMcps)
		adminAgentsGroup.PUT("/:name/mcps", mcpHandler.UpdateAgentMcps)
		adminAgentsGroup.GET("/:name/knowledge", agentHandler.GetAgentKnowledge)
		adminAgentsGroup.PUT("/:name/knowledge", agentHandler.UpdateAgentKnowledge)
		adminAgentsGroup.POST("/:name/probe", agentHandler.ProbeAgent)
		adminAgentsGroup.POST("/:name/deploy", agentHandler.DeployAgent)
		adminAgentsGroup.GET("/:name/deploy", agentHandler.GetDeployment)
		adminAgentsGroup.POST("/:name/deploy/stop", agentHandler.StopDeployment)
		adminAgentsGroup.POST("/:name/deploy/start", agentHandler.StartDeployment)
		adminAgentsGroup.DELETE("/:name/deploy", agentHandler.DeleteDeployment)
		adminAgentsGroup.GET("/:name/detail", agentDetailHandler.GetAgentDetail)
		adminAgentsGroup.GET("/:name/files", agentFilesHandler.ListFiles)
		adminAgentsGroup.GET("/:name/files/content", agentFilesHandler.GetContent)
		adminAgentsGroup.HEAD("/:name/files/content", agentFilesHandler.HeadContent)
	}

	// ---------- Tool 领域 ----------
	toolsGroup := v1adminGroup.Group("/tools")
	{
		toolsGroup.GET("", toolHandler.List)
		toolsGroup.POST("", toolHandler.Create)
		toolsGroup.GET("/:name", toolHandler.Get)
		toolsGroup.PUT("/:name", toolHandler.Update)
		toolsGroup.DELETE("/:name", toolHandler.Delete)
	}

	// ---------- MCP 领域 ----------
	// 公开接口（客户端按 agent 拉取 MCP 配置，已解密可直接喂给 SDK）
	mcpsGroup := v1group.Group("/mcps")
	{
		mcpsGroup.GET("", mcpHandler.GetClientMcpsByAgent)
	}

	// 管理接口（CRUD）
	adminMcpsGroup := v1adminGroup.Group("/mcps")
	{
		adminMcpsGroup.GET("", mcpHandler.List)
		adminMcpsGroup.POST("", mcpHandler.Create)
		adminMcpsGroup.POST("/probe", mcpHandler.ProbeByConfig)
		adminMcpsGroup.GET("/:name", mcpHandler.Get)
		adminMcpsGroup.PUT("/:name", mcpHandler.Update)
		adminMcpsGroup.POST("/:name/probe", mcpHandler.ProbeByName)
		adminMcpsGroup.DELETE("/:name", mcpHandler.Delete)
	}

	// ---------- Skill 领域 ----------
	// 公开接口
	skillsGroup := v1group.Group("/skills")
	{
		skillsGroup.GET("", skillHandler.ListPublic)
		skillsGroup.GET("/:name", skillHandler.GetPublic)
		skillsGroup.GET("/:name/download", skillHandler.Download)
	}

	// 管理接口
	adminSkillsGroup := v1adminGroup.Group("/skills")
	{
		adminSkillsGroup.GET("", skillHandler.ListAdmin)
		adminSkillsGroup.POST("", skillHandler.Create)
		adminSkillsGroup.PUT("/:name", skillHandler.Update)
		adminSkillsGroup.DELETE("/:name", skillHandler.Delete)
		adminSkillsGroup.GET("/:name/skill-md", skillHandler.GetSkillMd)
	}

	// ---------- Scene 领域 ----------
	// 公开接口
	scenesGroup := v1group.Group("/scenes")
	{
		scenesGroup.GET("", sceneHandler.List)
		scenesGroup.GET("/:name", sceneHandler.Get)
	}

	// 管理接口
	adminScenesGroup := v1adminGroup.Group("/scenes")
	{
		adminScenesGroup.GET("", sceneHandler.ListAdmin)
		adminScenesGroup.POST("", sceneHandler.Create)
		adminScenesGroup.PUT("/:name", sceneHandler.Update)
		adminScenesGroup.DELETE("/:name", sceneHandler.Delete)
	}

	// ---------- Knowledge MCP 运行时 ----------
	// This endpoint is called by the agent runtime with an Agent Runtime Token,
	// not a user JWT, so it must not be under the JWTAuthWithCLI middleware group.
	r.POST("/api/v1/knowledge/mcp", middleware.AgentRuntimeAuthMiddleware(cfg.Provider.EncryptionKey), knowledgeMcpHandler.HandleMessage)

	// ---------- Knowledge 领域 ----------
	handler.RegisterKnowledgeRoutes(v1adminGroup, knowledgeHandler)

	// /api/v1/services
	v1group.GET("/services/:service", serviceRouter.Route)

	// ---------- Chat 领域 ----------
	chatGroup := v1group.Group("/chat")
	{
		chatGroup.POST("/push", chatHandler.Push)
	}

	adminChatGroup := v1adminGroup.Group("/chat")
	{
		adminChatGroup.GET("/sessions", chatHandler.ListSessions)
		adminChatGroup.GET("/sessions/:id", chatHandler.GetSession)
		adminChatGroup.GET("/sessions/:id/messages", chatHandler.ListMessages)
		adminChatGroup.DELETE("/sessions/:id", chatHandler.DeleteSession)
	}

	// ---------- AIGC 标识配置 ----------
	adminAigcGroup := v1adminGroup.Group("/aigc")
	{
		adminAigcGroup.GET("/config", aigcConfigHandler.Get)
		adminAigcGroup.PUT("/config", aigcConfigHandler.Save)
		adminAigcGroup.POST("/config/rotate-key", aigcConfigHandler.RotateKey)
		adminAigcGroup.DELETE("/config", aigcConfigHandler.Delete)
	}

	// ---------- Provider 领域 ----------
	// 公开接口（Electron 应用读取 provider 配置 + 模型列表）
	providersGroup := v1group.Group("/providers")
	{
		providersGroup.GET("", providerHandler.List)
		providersGroup.GET("/runtime-config", providerHandler.ListRuntimeConfig)
		providersGroup.GET("/:id", providerHandler.Get)
	}

	// 管理接口
	adminProvidersGroup := v1adminGroup.Group("/providers")
	{
		adminProvidersGroup.GET("", providerHandler.ListAdmin)
		adminProvidersGroup.POST("", providerHandler.Create)
		adminProvidersGroup.POST("/probe", providerHandler.ProbeConfig)
		adminProvidersGroup.PUT("/:id", providerHandler.Update)
		adminProvidersGroup.DELETE("/:id", providerHandler.Delete)
		adminProvidersGroup.POST("/:id/probe", providerHandler.Probe)
		adminProvidersGroup.POST("/:id/reveal-key", providerHandler.RevealAPIKey)
		adminProvidersGroup.GET("/attr-rules", providerHandler.AttrRules)

		// Per-model CRUD (Task 5): attach/update/delete a single model row.
		adminProvidersGroup.POST("/:id/models", providerHandler.AddModel)
		adminProvidersGroup.PATCH("/:id/models/:selectionId", providerHandler.UpdateModel)
		adminProvidersGroup.DELETE("/:id/models/:selectionId", providerHandler.DeleteModel)

		// MultiRAG sync (Task 6): push provider config to the configured MultiRAG.
		adminProvidersGroup.POST("/:id/sync-multirag", providerHandler.SyncToMultiRAG)
	}

	// ---------- Builtin 用户管理（仅 builtin 模式注册） ----------
	// 用户管理与邀请由 admin 独享。casdoor 模式下用户管理仍在 Casdoor 后台。
	if cfg.Auth.IsBuiltin() {
		usersAdmin := v1group.Group("/admin", middleware.RequireAdmin())
		usersAdmin.GET("/users", adminUserHandler.ListUsers)
		usersAdmin.PATCH("/users/:id", adminUserHandler.UpdateUser)
		usersAdmin.POST("/users/:id/reset-password", adminUserHandler.ResetUserPassword)
		usersAdmin.POST("/invites", adminUserHandler.CreateInvite)
		usersAdmin.GET("/invites", adminUserHandler.ListInvites)
		usersAdmin.DELETE("/invites/:id", adminUserHandler.RevokeInvite)
	}

	// ---------- CLI Token 领域 ----------
	// 任何已登录用户（Casdoor JWT 或 CLI token）都可以管理自己的 CLI tokens。
	// 注意：CLI token 自身也能调用这些端点（例如轮换）。
	cliGroup := v1group.Group("/cli")
	{
		cliGroup.POST("/issue-token", cliTokenHandler.Issue)
		cliGroup.GET("/tokens", cliTokenHandler.List)
		cliGroup.DELETE("/tokens/:id", cliTokenHandler.Revoke)
	}

	// 未匹配路由重定向到前端 SPA
	r.NoRoute(func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/static")
	})

	// Start Kong gateway reconciler. If Kong is not configured this is a no-op.
	reconcileCtx, reconcileCancel := context.WithCancel(context.Background())
	defer reconcileCancel()
	kongService.StartReconciler(reconcileCtx)

	log.Printf("Server starting on %s", cfg.GetServerAddr())
	if err := r.Run(cfg.GetServerAddr()); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
