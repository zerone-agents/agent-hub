package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	authdom "control-panel/internal/domain/auth"
	providerdom "control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"
	repository "control-panel/internal/infrastructure/persistence"
)

// CasdoorTenantSource 抽象 casdoor 侧组织/应用读取（测试注入假源；生产用
// SDKCasdoorSource 包 auth.GetClient()）。
type CasdoorTenantSource interface {
	GetOrganizations() ([]*casdoorsdk.Organization, error)
	GetApplications() ([]*casdoorsdk.Application, error)
}

// SDKCasdoorSource 用 casdoor SDK client 实现 CasdoorTenantSource。
type SDKCasdoorSource struct {
	client *casdoorsdk.Client
}

// NewSDKCasdoorSource 用 casdoor SDK client 构造生产源（包外构造入口）。
func NewSDKCasdoorSource(client *casdoorsdk.Client) CasdoorTenantSource {
	return SDKCasdoorSource{client: client}
}

func (s SDKCasdoorSource) GetOrganizations() ([]*casdoorsdk.Organization, error) {
	return s.client.GetOrganizations()
}

func (s SDKCasdoorSource) GetApplications() ([]*casdoorsdk.Application, error) {
	return s.client.GetApplications()
}

// isHubAppName 判定 casdoor 应用名是否为 hub 的 OAuth 应用：前缀
// "agent-hub-"（issue #114 起的命名约定）或后缀 "-hub"（兼容存量
// ai-games-hub——先于约定命名的组织，见 #238 决策评论）。
func isHubAppName(name string) bool {
	return strings.HasPrefix(name, "agent-hub-") || strings.HasSuffix(name, "-hub")
}

// TenantSyncReport 汇总一轮同步的结果（日志与测试断言用）。
type TenantSyncReport struct {
	Added   int      // 新增注册的组织
	Updated int      // 凭据轮换/变更的组织
	Orphans []string // hub 有行但 casdoor 无对应 hub 应用的组织（保留 + 告警）
	Skipped []string // casdoor 有 hub 应用但 org 名不合规的组织（跳过 + 告警）
}

// TenantSyncService 周期同步 casdoor 组织 → tenant_oauth_clients
// （#238 part 2）。决策点落地：casdoor 新组织可直接登录（无闸门）；
// default 归属不动（同步只新增/更新非 default 行）；casdoor 删组织后
// hub 行保留（orphan 告警，不自动删）。
type TenantSyncService struct {
	repo          *repository.TenantOAuthClientRepository
	source        CasdoorTenantSource
	encryptionKey string
	reconcileSec  int
	logger        *log.Logger
}

// NewTenantSyncService 构造同步服务。reconcileSec <= 0 时默认 300s。
func NewTenantSyncService(
	repo *repository.TenantOAuthClientRepository,
	source CasdoorTenantSource,
	encryptionKey string,
	reconcileSec int,
	logger *log.Logger,
) *TenantSyncService {
	if reconcileSec <= 0 {
		reconcileSec = 300
	}
	if logger == nil {
		logger = log.Default()
	}
	return &TenantSyncService{
		repo:          repo,
		source:        source,
		encryptionKey: encryptionKey,
		reconcileSec:  reconcileSec,
		logger:        logger,
	}
}

// SyncOnce 执行一轮同步。fail-closed：casdoor 读取/加密/落库任一失败整轮
// 放弃（返回 error），绝不落半套数据。
func (s *TenantSyncService) SyncOnce(ctx context.Context) (TenantSyncReport, error) {
	var report TenantSyncReport

	// 表空守卫：repo.Upsert 对空表会把首行自动提为 default——同步绝不代劳
	// default 的创建（default 必须是 ops 显式注册的首行），空表整轮跳过。
	n, err := s.repo.Count()
	if err != nil {
		return report, fmt.Errorf("count existing rows: %w", err)
	}
	if n == 0 {
		s.logger.Printf("tenant-sync: tenant_oauth_clients empty — skipping (default must be registered via ops first)")
		return report, nil
	}

	orgs, err := s.source.GetOrganizations()
	if err != nil {
		return report, fmt.Errorf("casdoor get-organizations: %w", err)
	}
	apps, err := s.source.GetApplications()
	if err != nil {
		return report, fmt.Errorf("casdoor get-applications: %w", err)
	}

	// casdoor 组织名集合：hub 应用必须挂在已存在的组织下。
	orgNames := make(map[string]bool, len(orgs))
	for _, o := range orgs {
		if o != nil && o.Name != "" {
			orgNames[o.Name] = true
		}
	}

	// 识别每个组织的 hub 应用（一个组织多个 → 保留首个 + 告警）。
	hubApps := make(map[string]*casdoorsdk.Application)
	for _, app := range apps {
		if app == nil || !isHubAppName(app.Name) {
			continue
		}
		if !orgNames[app.Organization] {
			s.logger.Printf("tenant-sync: app %q references unknown org %q — skipped", app.Name, app.Organization)
			continue
		}
		if app.ClientId == "" || app.ClientSecret == "" {
			s.logger.Printf("tenant-sync: app %q (org %q) missing clientId/clientSecret — skipped", app.Name, app.Organization)
			continue
		}
		if prev, ok := hubApps[app.Organization]; ok {
			s.logger.Printf("tenant-sync: org %q has multiple hub apps (%q, %q) — keeping %q",
				app.Organization, prev.Name, app.Name, prev.Name)
			continue
		}
		hubApps[app.Organization] = app
	}

	existing, err := s.repo.List()
	if err != nil {
		return report, fmt.Errorf("list existing rows: %w", err)
	}
	existingByOrg := make(map[string]*authdom.TenantOAuthClient, len(existing))
	for i := range existing {
		existingByOrg[existing[i].Org] = &existing[i]
	}

	for org, app := range hubApps {
		if !tenant.ValidateOrgName(org) {
			report.Skipped = append(report.Skipped, org)
			s.logger.Printf("tenant-sync: org %q fails the naming contract — skipped", org)
			continue
		}
		secretEnc, err := providerdom.Encrypt(app.ClientSecret, s.encryptionKey)
		if err != nil {
			return report, fmt.Errorf("encrypt secret for org %q: %w", org, err)
		}
		certEnc := ""
		if app.Cert != "" {
			certEnc, err = providerdom.Encrypt(app.Cert, s.encryptionKey)
			if err != nil {
				return report, fmt.Errorf("encrypt cert for org %q: %w", org, err)
			}
		}
		cur, ok := existingByOrg[org]
		var changed bool
		if ok {
			// 密文不可直接比较：providerdom.Encrypt 用随机 nonce（AES-GCM），
			// 相同明文每次密文不同——密文比较会每轮误判「凭据变更」做无谓
			// Upsert。解密现有行后按明文比较（Decrypt 对非 enc: 值原样返回，
			// dev mode/明文存量天然兼容）。
			curSecret, derr := providerdom.Decrypt(cur.ClientSecretEnc, s.encryptionKey)
			if derr != nil {
				return report, fmt.Errorf("decrypt existing secret for org %q: %w", org, derr)
			}
			curCert, derr := providerdom.Decrypt(cur.CertEnc, s.encryptionKey)
			if derr != nil {
				return report, fmt.Errorf("decrypt existing cert for org %q: %w", org, derr)
			}
			changed = cur.ClientID != app.ClientId || curSecret != app.ClientSecret || curCert != app.Cert
		}
		switch {
		case !ok:
			// 新增：isDefault=false——Count>0 已保证 Upsert 不会提 default。
			if err := s.repo.Upsert(org, app.ClientId, secretEnc, certEnc, false); err != nil {
				return report, fmt.Errorf("upsert new org %q: %w", org, err)
			}
			report.Added++
			s.logger.Printf("tenant-sync: + org %q (client %s)", org, app.ClientId)
		case changed:
			// 凭据轮换/变更：保留原 default 标志（Upsert 对 default 行传 false
			// 会走 ErrDefaultRequired，故按现值传）。
			isDefault := cur.DefaultKey != nil
			if err := s.repo.Upsert(org, app.ClientId, secretEnc, certEnc, isDefault); err != nil {
				return report, fmt.Errorf("upsert org %q: %w", org, err)
			}
			report.Updated++
			s.logger.Printf("tenant-sync: ~ org %q credentials updated", org)
		}
	}

	// orphan：hub 有行但 casdoor 无对应 hub 应用——保留（casdoor 删组织不
	// 自动清行，防误删租户数据），仅告警。
	for org := range existingByOrg {
		if _, ok := hubApps[org]; !ok {
			report.Orphans = append(report.Orphans, org)
			s.logger.Printf("tenant-sync: orphan org %q (no casdoor hub app) — kept", org)
		}
	}

	return report, nil
}

// StartSyncer 启动周期同步 goroutine（仿 KongGatewayService.StartReconciler）：
// 立即跑一轮，随后按 reconcileSec tick。ctx 取消时退出。
func (s *TenantSyncService) StartSyncer(ctx context.Context) {
	go func() {
		s.runSyncOnce(ctx)
		ticker := time.NewTicker(time.Duration(s.reconcileSec) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runSyncOnce(ctx)
			}
		}
	}()
}

func (s *TenantSyncService) runSyncOnce(ctx context.Context) {
	report, err := s.SyncOnce(ctx)
	if err != nil {
		s.logger.Printf("tenant-sync: %v", err)
		return
	}
	if report.Added > 0 || report.Updated > 0 || len(report.Orphans) > 0 || len(report.Skipped) > 0 {
		s.logger.Printf("tenant-sync: done +%d ~%d orphans=%v skipped=%v",
			report.Added, report.Updated, report.Orphans, report.Skipped)
	}
}
