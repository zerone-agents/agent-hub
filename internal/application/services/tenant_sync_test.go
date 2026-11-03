package services

import (
	"context"
	"errors"
	"testing"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"control-panel/pkg/database"

	authdom "control-panel/internal/domain/auth"
	providerdom "control-panel/internal/domain/provider"
	repository "control-panel/internal/infrastructure/persistence"
)

// tenantSyncTestKey 与 ops handler 测试同款 32 字节 hex key（provider.Encrypt 要求）。
const tenantSyncTestKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// fakeTenantSource 是可编程的假 casdoor 源（orgs/apps/err）。
type fakeTenantSource struct {
	orgs []*casdoorsdk.Organization
	apps []*casdoorsdk.Application
	err  error
}

func (f *fakeTenantSource) GetOrganizations() ([]*casdoorsdk.Organization, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.orgs, nil
}

func (f *fakeTenantSource) GetApplications() ([]*casdoorsdk.Application, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.apps, nil
}

func tsOrg(name string) *casdoorsdk.Organization {
	return &casdoorsdk.Organization{Name: name}
}

// tsApp 构造测试用 hub 应用（默认带合法 redirect——hasHubRedirect 校验要求）。
func tsApp(name, orgName, clientID, secret, cert string) *casdoorsdk.Application {
	return &casdoorsdk.Application{
		Name: name, Organization: orgName,
		ClientId: clientID, ClientSecret: secret, Cert: cert,
		RedirectUris: []string{"https://console.zerone.life/auth/callback"},
	}
}

// tsAppNoRedirect 构造 redirect 不含 hub 回调路径的应用（识别应跳过）。
func tsAppNoRedirect(name, orgName, clientID, secret string) *casdoorsdk.Application {
	return &casdoorsdk.Application{
		Name: name, Organization: orgName,
		ClientId: clientID, ClientSecret: secret,
		RedirectUris: []string{"https://evil.example/callback"},
	}
}

func newTenantSyncFixture(t *testing.T) (*repository.TenantOAuthClientRepository, *fakeTenantSource) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authdom.TenantOAuthClient{}))
	// 仓库测试模式：替换 database 包级 DB（NewTenantOAuthClientRepository 无参
	// 构造读 database.GetDB()），cleanup 还原。
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return repository.NewTenantOAuthClientRepository(), &fakeTenantSource{}
}

// seedDefault 预置一行 default（表非空 + default 归属）。
func seedDefault(t *testing.T, repo *repository.TenantOAuthClientRepository, org, secret string) {
	t.Helper()
	require.NoError(t, repo.Upsert(org, "cid-"+org, secret, "", true))
}

func TestIsHubAppName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"agent-hub", true},     // #114 约定前缀的边界形态
		{"agent-hub-ayu", true}, // 标准前缀
		{"ai-games-hub", true},  // 存量：先于约定的 *-hub 后缀
		{"app-built-in", false}, // 内置租户应用，不是 hub 的
		{"random-app", false},   // 无关应用
		{"hub", false},          // 裸 hub：无前缀/分隔后缀，不收
		{"agent-hubx", false},   // 前缀后无内容
	}
	for _, c := range cases {
		if got := isHubAppName(c.name); got != c.want {
			t.Errorf("isHubAppName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestTenantSync_EmptyTableSkips：表空时整轮跳过——repo.Upsert 会把空表首行
// 自动提为 default，同步绝不代劳 default 的创建（#238 决策点 3）。
func TestTenantSync_EmptyTableSkips(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	src.orgs = []*casdoorsdk.Organization{tsOrg("newco")}
	src.apps = []*casdoorsdk.Application{tsApp("agent-hub-newco", "newco", "cid-new", "sec-new", "")}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, report.Added)

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 0, "empty table must stay empty — default comes from ops registration only")
}

// TestTenantSync_AddsNewOrgs：新增组织落库，secret 加密存储（可解密还原），
// 且不碰 default 行的 default 标志、新行 default_key 为空。
func TestTenantSync_AddsNewOrgs(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-zerone")
	src.orgs = []*casdoorsdk.Organization{tsOrg("zerone"), tsOrg("aigames")}
	src.apps = []*casdoorsdk.Application{
		tsApp("agent-hub-zerone", "zerone", "cid-zerone", "sec-zerone", ""),
		tsApp("ai-games-hub", "aigames", "cid-aigames", "sec-aigames", "cert-pem"),
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, report.Added) // 仅 aigames 新增；zerone 已存在且无变化
	require.Empty(t, report.Updated)

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 2)

	ag, err := repo.Find("aigames")
	require.NoError(t, err)
	require.NotNil(t, ag)
	require.Equal(t, "cid-aigames", ag.ClientID)
	require.NotEqual(t, "sec-aigames", ag.ClientSecretEnc, "secret must be stored encrypted")
	dec, err := providerdom.Decrypt(ag.ClientSecretEnc, tenantSyncTestKey)
	require.NoError(t, err)
	require.Equal(t, "sec-aigames", dec)
	certDec, err := providerdom.Decrypt(ag.CertEnc, tenantSyncTestKey)
	require.NoError(t, err)
	require.Equal(t, "cert-pem", certDec)
	require.Nil(t, ag.DefaultKey, "synced rows never take default")

	zr, err := repo.Find("zerone")
	require.NoError(t, err)
	require.NotNil(t, zr.DefaultKey, "existing default row keeps its default flag")
}

// TestTenantSync_UpdatesRotatedSecret：casdoor 侧 secret 轮换 → 更新密文；
// default 组织的轮换同样保留 default 标志。
func TestTenantSync_UpdatesRotatedSecret(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-old")
	src.orgs = []*casdoorsdk.Organization{tsOrg("zerone"), tsOrg("ayu")}
	src.apps = []*casdoorsdk.Application{
		tsApp("agent-hub-zerone", "zerone", "cid-zerone", "sec-rotated", ""),
		tsApp("agent-hub-ayu", "ayu", "cid-ayu", "sec-ayu", ""),
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, report.Added)   // ayu
	require.Equal(t, 1, report.Updated) // zerone secret 轮换

	zr, err := repo.Find("zerone")
	require.NoError(t, err)
	dec, err := providerdom.Decrypt(zr.ClientSecretEnc, tenantSyncTestKey)
	require.NoError(t, err)
	require.Equal(t, "sec-rotated", dec)
	require.NotNil(t, zr.DefaultKey, "default org's rotated credentials keep the default flag")
}

// TestTenantSync_OrphansKept：hub 有行但 casdoor 无对应 hub 应用 → 保留并
// 计入 orphan（#238 决策点 4：删组织不自动清行）。
func TestTenantSync_OrphansKept(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	require.NoError(t, repo.Upsert("gone", "cid-gone", "enc-gone", "", false))
	src.orgs = []*casdoorsdk.Organization{tsOrg("zerone")}
	src.apps = []*casdoorsdk.Application{tsApp("agent-hub-zerone", "zerone", "cid-zerone", "sec-zerone", "")}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"gone"}, report.Orphans)

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 2, "orphan row is kept, not deleted")
}

// TestTenantSync_SkipsInvalidOrgNames：casdoor 侧组织名不合租户命名契约
// （#235）→ 跳过 + 告警，不落库。
func TestTenantSync_SkipsInvalidOrgNames(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	src.orgs = []*casdoorsdk.Organization{tsOrg("Bad_Org"), tsOrg("a--b")}
	src.apps = []*casdoorsdk.Application{
		tsApp("agent-hub-bad", "Bad_Org", "cid-bad", "sec-bad", ""),
		tsApp("agent-hub-ab", "a--b", "cid-ab", "sec-ab", ""),
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Bad_Org", "a--b"}, report.Skipped)
	require.Equal(t, 0, report.Added)

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the seeded default row remains")
}

// TestTenantSync_FailClosedOnSourceError：casdoor 读取失败 → 整轮放弃
// （返回 error），不落半套数据。
func TestTenantSync_FailClosedOnSourceError(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	src.err = errors.New("casdoor unreachable")
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	_, err := svc.SyncOnce(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "casdoor")

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 1, "failed round writes nothing")
}

// TestTenantSync_UnknownOrgAndMissingCredsSkipped：hub 应用挂在不存在的
// 组织下、或缺 clientId/secret → 跳过，不落库。
func TestTenantSync_UnknownOrgAndMissingCredsSkipped(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	src.orgs = []*casdoorsdk.Organization{tsOrg("realorg")}
	src.apps = []*casdoorsdk.Application{
		tsApp("agent-hub-ghost", "ghostorg", "cid-ghost", "sec-ghost", ""), // 组织不存在
		tsApp("agent-hub-nosec", "realorg", "cid-nosec", "", ""),           // 缺 secret
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, report.Added)

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

// TestTenantSync_PrefixBeatsSuffix：同 org 多 hub 应用时前缀 agent-hub-*
// 确定胜出（review 重要 2：无关 *-hub 应用不得靠遍历顺序覆盖 ops 注册的
// 凭据——否则轮换路径会把正确的 client_id/secret 覆写成登录坏）。
func TestTenantSync_PrefixBeatsSuffix(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	src.orgs = []*casdoorsdk.Organization{tsOrg("acme")}
	// 顺序故意后缀应用在前：两轮扫描（前缀轮先跑）必须让 agent-hub-acme 胜出。
	src.apps = []*casdoorsdk.Application{
		tsApp("decoy-hub", "acme", "cid-decoy", "sec-decoy", ""),
		tsApp("agent-hub-acme", "acme", "cid-acme", "sec-acme", ""),
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, report.Added)

	row, err := repo.Find("acme")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, "cid-acme", row.ClientID, "prefix app must win regardless of list order")
}

// TestTenantSync_SkipsAppsWithoutHubRedirect：redirect_uris 不含 hub 回调
// 路径的应用不是给本 hub 的——跳过，不落库（review 重要 2 的兜底校验）。
func TestTenantSync_SkipsAppsWithoutHubRedirect(t *testing.T) {
	repo, src := newTenantSyncFixture(t)
	seedDefault(t, repo, "zerone", "sec-x")
	src.orgs = []*casdoorsdk.Organization{tsOrg("suspicious")}
	src.apps = []*casdoorsdk.Application{
		tsAppNoRedirect("agent-hub-suspicious", "suspicious", "cid-x", "sec-x"),
	}
	svc := NewTenantSyncService(repo, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, report.Added)

	row, err := repo.Find("suspicious")
	require.NoError(t, err)
	require.Nil(t, row)
}

// failingUpsertRepo 包一层真 repo：第 failAt 次（从 1 计）Upsert 返回 error，
// 用于钉住写阶段逐行收敛语义（review 重要 1）。
type failingUpsertRepo struct {
	*repository.TenantOAuthClientRepository
	calls  int
	failAt int
}

func (f *failingUpsertRepo) Upsert(org, clientID, secretEnc, certEnc string, isDefault bool) error {
	f.calls++
	if f.calls == f.failAt {
		return errors.New("db down mid-write")
	}
	return f.TenantOAuthClientRepository.Upsert(org, clientID, secretEnc, certEnc, isDefault)
}

// TestTenantSync_MidWriteFailureConverges：写阶段逐行收敛——按 org 排序后第 2
// 个 org 落库失败时，第 1 个已提交、本轮返回 error、失败行不存在；差异由下轮
// tick 补齐（周期同步的收敛模型，非单事务）。
func TestTenantSync_MidWriteFailureConverges(t *testing.T) {
	real, _ := newTenantSyncFixture(t)
	require.NoError(t, real.Upsert("zerone", "cid-zerone", "sec-zerone", "", true)) // default 行（表非空）

	failing := &failingUpsertRepo{TenantOAuthClientRepository: real, failAt: 2}
	src := &fakeTenantSource{
		orgs: []*casdoorsdk.Organization{tsOrg("zerone"), tsOrg("orga"), tsOrg("orgb")},
		apps: []*casdoorsdk.Application{
			tsApp("agent-hub-zerone", "zerone", "cid-zerone", "sec-zerone", ""),
			tsApp("agent-hub-orga", "orga", "cid-orga", "sec-orga", ""),
			tsApp("agent-hub-orgb", "orgb", "cid-orgb", "sec-orgb", ""),
		},
	}
	svc := NewTenantSyncService(failing, src, tenantSyncTestKey, 0, nil)

	report, err := svc.SyncOnce(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "orgb") // 排序 orga<orgb<zerone；zerone 无变化不落库
	require.Equal(t, 1, report.Added, "前序行（orga）已提交并计入报告")

	orga, ferr := real.Find("orga")
	require.NoError(t, ferr)
	require.NotNil(t, orga, "排序首行已提交")
	orgb, ferr := real.Find("orgb")
	require.NoError(t, ferr)
	require.Nil(t, orgb, "失败行未落库——差异由下轮收敛")
}
