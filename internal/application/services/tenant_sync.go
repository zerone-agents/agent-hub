package services

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	authdom "control-panel/internal/domain/auth"
	providerdom "control-panel/internal/domain/provider"
	"control-panel/internal/domain/tenant"
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

const (
	// hubAppPrefix 是 ops 注册的标准 hub 应用名前缀（issue #114 起）。
	hubAppPrefix = "agent-hub-"
	// hubAppSuffix 是兼容形态后缀（存量 ai-games-hub——先于前缀约定命名）。
	hubAppSuffix = "-hub"
	// hubRedirectPath 是 hub OAuth 回调路径：应用的 redirect_uris 必须包含
	// 它，否则不是给这个 hub 用的应用（防误认无关 *-hub 应用，review 重要 2）。
	hubRedirectPath = "/auth/callback"
)

// isHubAppName 判定 casdoor 应用名是否可能是 hub 的 OAuth 应用：前缀
// "agent-hub-"（issue #114 起的命名约定）或后缀 "-hub"（兼容存量
// ai-games-hub——先于约定命名的组织，见 #238 决策评论）。识别优先级
// 见 SyncOnce 的两轮扫描（前缀优先，勿依赖单一遍历顺序）。
func isHubAppName(name string) bool {
	return strings.HasPrefix(name, hubAppPrefix) || strings.HasSuffix(name, hubAppSuffix)
}

// hasHubRedirect 报告应用的 redirect_uris 是否包含 hub 回调路径。
func hasHubRedirect(app *casdoorsdk.Application) bool {
	for _, uri := range app.RedirectUris {
		if strings.Contains(uri, hubRedirectPath) {
			return true
		}
	}
	return false
}

// TenantSyncReport 汇总一轮同步的结果（日志与测试断言用）。
type TenantSyncReport struct {
	Added   int      // 新增注册的组织
	Updated int      // 凭据轮换/变更的组织
	Orphans []string // hub 有行但 casdoor 无对应 hub 应用的组织（保留 + 告警）
	Skipped []string // casdoor 有 hub 应用但 org 名不合规的组织（跳过 + 告警）
}

// tenantSyncRepo 是 TenantSyncService 依赖的窄接口（生产为
// *repository.TenantOAuthClientRepository；测试可注入失败实现）。
type tenantSyncRepo interface {
	Count() (int64, error)
	List() ([]authdom.TenantOAuthClient, error)
	Upsert(org, clientID, secretEnc, certEnc string, isDefault bool) error
}

// TenantSyncService 周期同步 casdoor 组织 → tenant_oauth_clients
// （#238 part 2）。决策点落地：casdoor 新组织可直接登录（无闸门）；
// default 归属不动（同步只新增/更新非 default 行）；casdoor 删组织后
// hub 行保留（orphan 告警，不自动删）。
type TenantSyncService struct {
	repo          tenantSyncRepo
	source        CasdoorTenantSource
	encryptionKey string
	reconcileSec  int
	logger        *log.Logger
}

// NewTenantSyncService 构造同步服务。reconcileSec <= 0 时默认 300s。
func NewTenantSyncService(
	repo tenantSyncRepo,
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

// SyncOnce 执行一轮同步。两段式失败语义（review 精确化，勿再笼统称
// fail-closed）：
//   - 读阶段 fail-closed：casdoor 读取或加密失败 → 整轮不动笔（返回 error）；
//   - 写阶段逐行收敛：第 k 个 org 落库失败 → 前 k-1 个已提交、剩余放弃，
//     返回 error——下轮 tick 自然补齐差异（周期同步的收敛模型）。
//
// 不提供单事务全量应用：repo 无批量事务入口，且逐行收敛对周期性同步足够。
func (s *TenantSyncService) SyncOnce(ctx context.Context) (TenantSyncReport, error) {
	var report TenantSyncReport
	if err := ctx.Err(); err != nil {
		return report, err
	}

	// 表空守卫：repo.Upsert 对空表会把首行自动提为 default——同步绝不代劳
	// default 的创建（default 必须是 ops 显式注册的首行），空表整轮跳过。
	// 已知窄窗（TOCTOU）：Count 与后续 Upsert 之间 ops 若并发注册首行，同步
	// 新行理论上可能赶上「空表首行自动 default」判定——需 ops 首注册与同步
	// 首轮在无 default 时精确并发（灰度初期一次性窗口）；default_key 哨兵
	// 唯一索引兜底防双 default。
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

	// 识别每个组织的 hub 应用（review 重要 2：前缀优先两轮扫描 + redirect
	// 校验——无关的 *-hub 应用不得靠遍历顺序覆盖 ops 注册的 agent-hub-<org>，
	// redirect_uri 含 /auth/callback 兜底防误认）。
	hubApps := make(map[string]*casdoorsdk.Application)
	consider := func(match func(string) bool) {
		for _, app := range apps {
			if app == nil || !match(app.Name) {
				continue
			}
			org := app.Organization
			if !orgNames[org] {
				s.logger.Printf("tenant-sync: app %q references unknown org %q — skipped", app.Name, org)
				continue
			}
			if app.ClientId == "" || app.ClientSecret == "" {
				s.logger.Printf("tenant-sync: app %q (org %q) missing clientId/clientSecret — skipped", app.Name, org)
				continue
			}
			if !hasHubRedirect(app) {
				s.logger.Printf("tenant-sync: app %q (org %q) has no %s redirect — skipped", app.Name, org, hubRedirectPath)
				continue
			}
			if prev, ok := hubApps[org]; ok {
				s.logger.Printf("tenant-sync: org %q has multiple hub apps (%q, %q) — keeping %q",
					org, prev.Name, app.Name, prev.Name)
				continue
			}
			hubApps[org] = app
		}
	}
	// 第一轮：agent-hub-* 前缀（ops 注册的标准命名）优先。
	consider(func(name string) bool { return strings.HasPrefix(name, hubAppPrefix) })
	// 第二轮：*-hub 后缀且非前缀形态（补充存量 ai-games-hub；若前缀轮已命中
	// 该 org，下面的 prev 检查保留前缀应用）。
	consider(func(name string) bool {
		return !strings.HasPrefix(name, hubAppPrefix) && strings.HasSuffix(name, hubAppSuffix)
	})

	existing, err := s.repo.List()
	if err != nil {
		return report, fmt.Errorf("list existing rows: %w", err)
	}
	existingByOrg := make(map[string]*authdom.TenantOAuthClient, len(existing))
	for i := range existing {
		existingByOrg[existing[i].Org] = &existing[i]
	}

	// 写阶段按 org 名排序后逐行应用：顺序确定（失败点可推、日志/测试稳定），
	// 失败语义见 SyncOnce 注释（前序行已提交、本轮放弃剩余、下轮收敛）。
	sortedOrgs := make([]string, 0, len(hubApps))
	for org := range hubApps {
		sortedOrgs = append(sortedOrgs, org)
	}
	sort.Strings(sortedOrgs)

	for _, org := range sortedOrgs {
		app := hubApps[org]
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
	// 自动清行，防误删租户数据），仅告警。被合规门跳过的 org（app 存在但
	// 名字不合规）不算 orphan——那不是「casdoor 侧已删除」，是本地拒绝。
	skippedSet := make(map[string]bool, len(report.Skipped))
	for _, org := range report.Skipped {
		skippedSet[org] = true
	}
	for org := range existingByOrg {
		if _, ok := hubApps[org]; !ok && !skippedSet[org] {
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
