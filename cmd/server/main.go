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
	"control-panel/internal/auth/jwtutil"
	"control-panel/internal/config"
	"control-panel/internal/directory"
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

	// 一次性数据归一：旧模型升级后，存量行 {role:旧快照, status:pending} 与
	// 新模型语义矛盾（pending 用户仍持有旧角色权限）。此处清空 pending 行的
	// role，使升级后全员待审批、由 admin 重新分配。幂等（新模型 pending 行
	// role 恒为 ""），builtin 模式下 user_identities 表无数据、执行无副作用。
	if err := auth.NormalizePendingRoles(database.GetDB()); err != nil {
		log.Fatalf("Failed to normalize pending roles: %v", err)
	}

	if err := cfg.ValidateAuth(); err != nil {
		log.Fatalf("Invalid auth config: %v", err)
	}

	// ==================== 认证装配（按 auth.mode 二选一） ====================
	var authProvider auth.Provider
	var casdoorProvider *auth.CasdoorProvider
	var casdoorDir *directory.CasdoorDirectory
	var casdoorUserHandler *handler.CasdoorUserHandler
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
		// membershipStore 是 user_identities 的唯一句柄：provider（登录时合成
		// 角色）与 directory（用户管理 CRUD）共享同一实例。
		membershipStore := auth.NewMembershipStore(database.GetDB())
		casdoorProvider = auth.NewCasdoorProvider(membershipStore)
		authProvider = casdoorProvider
		casdoorDir = directory.NewCasdoorDirectory(auth.GetClient(), membershipStore)
		casdoorUserHandler = handler.NewCasdoorUserHandler(casdoorDir, auth.GetClient().GetSignupUrl(true, ""))
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
			authGroup.GET("/callback", handler.Callback(casdoorProvider))
			authGroup.GET("/userinfo", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), handler.UserInfo)
			authGroup.POST("/logout", middleware.JWTAuth(authProvider), handler.Logout)
			authGroup.POST("/refresh", handler.RefreshToken)
		}
	}

	// PendingApprovalGuard 紧跟鉴权中间件挂载（同一链）：casdoor 待审批用户
	// （角色为空）除白名单（/auth/userinfo、/auth/logout、/health*）外一律 403，
	// 前端据此渲染等待审批页。builtin 用户必有角色，guard 直接放行，行为零变化。
	// /auth/* 与 /health 挂在根级（白名单内），静态资源 /static 不在本链，均不受影响。
	v1group := r.Group("/api/v1", middleware.JWTAuthWithCLI(cliTokenSvc, authProvider), jwtutil.PendingApprovalGuard())
	// 管理写操作 + 敏感读：admin | maintainer（member 只读权限见 spec）
	adminWrite := v1group.Group("/admin", middleware.RequireManager())
	// 非敏感只读：admin | maintainer | member（逐条显式授予，见 spec 端点表）
	adminRead := v1group.Group("/admin", middleware.RequireRole("admin", "maintainer", "member"))

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

	// 管理接口：写方法/敏感 GET（files/content）→ write 组；
	// 非敏感 GET（列表、detail、tools、skills、mcps、knowledge、files 列表、deploy 状态）→ read 组（member 只读）
	adminAgentsGroup := adminWrite.Group("/agents")
	adminAgentsReadGroup := adminRead.Group("/agents")
	{
		adminAgentsReadGroup.GET("", agentHandler.ListAdmin)
		adminAgentsGroup.POST("", agentHandler.Create)
		adminAgentsGroup.PUT("/:name", agentHandler.Update)
		adminAgentsGroup.DELETE("/:name", agentHandler.Delete)
		adminAgentsGroup.PUT("/:name/subagents", agentHandler.UpdateSubagents)
		adminAgentsReadGroup.GET("/:name/tools", toolHandler.GetAgentTools)
		adminAgentsGroup.PUT("/:name/tools", toolHandler.UpdateAgentTools)
		adminAgentsReadGroup.GET("/:name/skills", skillHandler.GetAgentSkills)
		adminAgentsGroup.PUT("/:name/skills", skillHandler.UpdateAgentSkills)
		adminAgentsReadGroup.GET("/:name/mcps", mcpHandler.GetAgentMcps)
		adminAgentsGroup.PUT("/:name/mcps", mcpHandler.UpdateAgentMcps)
		adminAgentsReadGroup.GET("/:name/knowledge", agentHandler.GetAgentKnowledge)
		adminAgentsGroup.PUT("/:name/knowledge", agentHandler.UpdateAgentKnowledge)
		adminAgentsGroup.POST("/:name/probe", agentHandler.ProbeAgent)
		adminAgentsGroup.POST("/:name/deploy", agentHandler.DeployAgent)
		// 部署状态：member 可读（含 runtimeUrl/apiKey——内部可信环境，产品已确认）
		adminAgentsReadGroup.GET("/:name/deploy", agentHandler.GetDeployment)
		adminAgentsGroup.POST("/:name/deploy/stop", agentHandler.StopDeployment)
		adminAgentsGroup.POST("/:name/deploy/start", agentHandler.StartDeployment)
		adminAgentsGroup.DELETE("/:name/deploy", agentHandler.DeleteDeployment)
		adminAgentsReadGroup.GET("/:name/detail", agentDetailHandler.GetAgentDetail)
		adminAgentsReadGroup.GET("/:name/files", agentFilesHandler.ListFiles)
		// 文件内容属敏感信息，留 write 组
		adminAgentsGroup.GET("/:name/files/content", agentFilesHandler.GetContent)
		adminAgentsGroup.HEAD("/:name/files/content", agentFilesHandler.HeadContent)
	}

	// ---------- Tool 领域 ----------
	// 写方法 → write 组；GET 列表/详情 → read 组（member 只读）
	toolsGroup := adminWrite.Group("/tools")
	toolsReadGroup := adminRead.Group("/tools")
	{
		toolsReadGroup.GET("", toolHandler.List)
		toolsGroup.POST("", toolHandler.Create)
		toolsReadGroup.GET("/:name", toolHandler.Get)
		toolsGroup.PUT("/:name", toolHandler.Update)
		toolsGroup.DELETE("/:name", toolHandler.Delete)
	}

	// ---------- MCP 领域 ----------
	// 公开接口（客户端按 agent 拉取 MCP 配置，已解密可直接喂给 SDK）
	mcpsGroup := v1group.Group("/mcps")
	{
		mcpsGroup.GET("", mcpHandler.GetClientMcpsByAgent)
	}

	// 管理接口（CRUD）：写方法/probe → write 组；GET 列表/详情 → read 组（member 只读）
	adminMcpsGroup := adminWrite.Group("/mcps")
	adminMcpsReadGroup := adminRead.Group("/mcps")
	{
		adminMcpsReadGroup.GET("", mcpHandler.List)
		adminMcpsGroup.POST("", mcpHandler.Create)
		adminMcpsGroup.POST("/probe", mcpHandler.ProbeByConfig)
		adminMcpsReadGroup.GET("/:name", mcpHandler.Get)
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

	// 管理接口：GET 列表（ListAdmin）/skill-md → read 组（member 只读）；写方法 → write 组
	adminSkillsGroup := adminWrite.Group("/skills")
	adminSkillsReadGroup := adminRead.Group("/skills")
	{
		adminSkillsReadGroup.GET("", skillHandler.ListAdmin)
		adminSkillsGroup.POST("", skillHandler.Create)
		adminSkillsGroup.PUT("/:name", skillHandler.Update)
		adminSkillsGroup.DELETE("/:name", skillHandler.Delete)
		adminSkillsReadGroup.GET("/:name/skill-md", skillHandler.GetSkillMd)
	}

	// ---------- Scene 领域 ----------
	// 公开接口
	scenesGroup := v1group.Group("/scenes")
	{
		scenesGroup.GET("", sceneHandler.List)
		scenesGroup.GET("/:name", sceneHandler.Get)
	}

	// 管理接口：GET 列表（ListAdmin）→ read 组（member 只读）；写方法 → write 组
	adminScenesGroup := adminWrite.Group("/scenes")
	adminScenesReadGroup := adminRead.Group("/scenes")
	{
		adminScenesReadGroup.GET("", sceneHandler.ListAdmin)
		adminScenesGroup.POST("", sceneHandler.Create)
		adminScenesGroup.PUT("/:name", sceneHandler.Update)
		adminScenesGroup.DELETE("/:name", sceneHandler.Delete)
	}

	// ---------- Knowledge MCP 运行时 ----------
	// This endpoint is called by the agent runtime with an Agent Runtime Token,
	// not a user JWT, so it must not be under the JWTAuthWithCLI middleware group.
	r.POST("/api/v1/knowledge/mcp", middleware.AgentRuntimeAuthMiddleware(cfg.Provider.EncryptionKey), knowledgeMcpHandler.HandleMessage)

	// ---------- Knowledge 领域 ----------
	// 非敏感 GET（datasets/documents/chunks/images 等）→ read 组（member 只读），写方法与 POST /retrieval → write 组
	handler.RegisterKnowledgeRoutes(adminWrite, adminRead, knowledgeHandler)

	// /api/v1/services
	v1group.GET("/services/:service", serviceRouter.Route)

	// ---------- Chat 领域 ----------
	chatGroup := v1group.Group("/chat")
	{
		chatGroup.POST("/push", chatHandler.Push)
	}

	// 聊天历史：GET 会话/消息 → read 组（member 只读）；DELETE → write 组
	adminChatGroup := adminWrite.Group("/chat")
	adminChatReadGroup := adminRead.Group("/chat")
	{
		adminChatReadGroup.GET("/sessions", chatHandler.ListSessions)
		adminChatReadGroup.GET("/sessions/:id", chatHandler.GetSession)
		adminChatReadGroup.GET("/sessions/:id/messages", chatHandler.ListMessages)
		adminChatGroup.DELETE("/sessions/:id", chatHandler.DeleteSession)
	}

	// ---------- AIGC 标识配置 ----------
	// 全部端点含敏感配置（密钥），整体留 write 组，member 不开放
	adminAigcGroup := adminWrite.Group("/aigc")
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

	// 管理接口：GET 列表/attr-rules → read 组（member 只读）；
	// 写方法与 reveal-key（敏感）→ write 组
	adminProvidersGroup := adminWrite.Group("/providers")
	adminProvidersReadGroup := adminRead.Group("/providers")
	{
		adminProvidersReadGroup.GET("", providerHandler.ListAdmin)
		adminProvidersGroup.POST("", providerHandler.Create)
		adminProvidersGroup.POST("/probe", providerHandler.ProbeConfig)
		adminProvidersGroup.PUT("/:id", providerHandler.Update)
		adminProvidersGroup.DELETE("/:id", providerHandler.Delete)
		adminProvidersGroup.POST("/:id/probe", providerHandler.Probe)
		adminProvidersGroup.POST("/:id/reveal-key", providerHandler.RevealAPIKey)
		adminProvidersReadGroup.GET("/attr-rules", providerHandler.AttrRules)

		// Per-model CRUD (Task 5): attach/update/delete a single model row.
		adminProvidersGroup.POST("/:id/models", providerHandler.AddModel)
		adminProvidersGroup.PATCH("/:id/models/:selectionId", providerHandler.UpdateModel)
		adminProvidersGroup.DELETE("/:id/models/:selectionId", providerHandler.DeleteModel)

		// MultiRAG sync (Task 6): push provider config to the configured MultiRAG.
		adminProvidersGroup.POST("/:id/sync-multirag", providerHandler.SyncToMultiRAG)
	}

	// ---------- Admin user management (builtin: invites + local users; casdoor: casdoor API passthrough) ----------
	// User management and invites are admin-only. Builtin mode also manages
	// invites; casdoor mode delegates user management to the Casdoor admin API
	// via the directory.
	if cfg.Auth.IsBuiltin() {
		usersAdmin := v1group.Group("/admin", middleware.RequireAdmin())
		usersAdmin.GET("/users", adminUserHandler.ListUsers)
		usersAdmin.PATCH("/users/:id", adminUserHandler.UpdateUser)
		usersAdmin.POST("/users/:id/reset-password", adminUserHandler.ResetUserPassword)
		usersAdmin.POST("/invites", adminUserHandler.CreateInvite)
		usersAdmin.GET("/invites", adminUserHandler.ListInvites)
		usersAdmin.DELETE("/invites/:id", adminUserHandler.RevokeInvite)
	} else {
		usersAdmin := v1group.Group("/admin", middleware.RequireAdmin())
		usersAdmin.GET("/users/signup-url", casdoorUserHandler.SignupURL)
		usersAdmin.GET("/users", casdoorUserHandler.ListUsers)
		usersAdmin.PATCH("/users/:id", casdoorUserHandler.UpdateUser)
		usersAdmin.POST("/users/:id/reset-password", casdoorUserHandler.ResetUserPassword)
	}

	// ---------- CLI Token 领域 ----------
	// CLI token 签发/查看/吊销仅 admin|maintainer（member 不开放；
	// member 历史签发的 token 不吊销，仍可用于公开聊天接口调用）。
	cliGroup := v1group.Group("/cli", middleware.RequireManager())
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
