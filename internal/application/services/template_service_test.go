// H7.3 模板服务测试：注册/幂等/租户隔离、导出→注册→安装 round-trip、
// 幂等重放、冲突 fail/rename、sections 部分安装、扩展依赖缺失 409/force、
// 事务回滚、跨租户隔离、种子幂等。
package services

import (
	"encoding/json"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/personality"
	rundomain "control-panel/internal/domain/run"
	"control-panel/internal/domain/template"
	"control-panel/internal/domain/workflow"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTemplateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&template.TemplateDefinition{}, &template.TemplateVersion{}, &template.TemplateInstall{},
		&agent.AgentConfig{}, &personality.Template{}, &personality.Version{},
		&collaboration.Group{}, &collaboration.GroupMember{}, &collaboration.Channel{},
		&agentrelation.AgentRelation{},
		&workflow.Definition{}, &workflow.Version{}, &workflow.Step{},
		&rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.CapabilityBinding{}, &rundomain.StateSchema{}, &rundomain.RunState{}, &rundomain.RunStateChange{},
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
	))
	return db
}

func testTemplateSpecJSON() json.RawMessage {
	return json.RawMessage(`{
	  "agents": [
	    {"name":"coordinator","title":"协调者","systemPrompt":"协调","personalityPrompt":"开放","modelRef":"default/general"},
	    {"name":"executor","title":"执行者","systemPrompt":"执行","modelRef":"default/general"}
	  ],
	  "personalityTemplates": [{"name":"pt-open","content":"开放沟通"}],
	  "groups": [{"name":"core-team","description":"核心团队","channels":["general"],"memberRefs":["coordinator","executor"]}],
	  "relations": [{"fromRef":"executor","toRef":"coordinator","relationType":"reports_to","allowedActions":["report","inform"]}],
	  "workflows": [{"name":"daily-sync","description":"每日同步","steps":[{"key":"collect","name":"收集"},{"key":"review","name":"评审","dependsOn":["collect"],"actorRef":"coordinator"}]}],
	  "stateSchemas": [{"namespace":"io.zerone.test","name":"profile","version":"1.0.0","schema":{"type":"object","properties":{"score":{"type":"integer"}},"required":["score"]}}],
	  "extensionDeps": [{"name":"io.zerone.organization.emotion","versionRange":"^1.0.0"}]
	}`)
}

func registerTestTemplate(t *testing.T, svc *TemplateService, tenantID string) *RegisterTemplateResult {
	t.Helper()
	res, err := svc.Register(tenantID, RegisterTemplateInput{
		Name: "io.zerone.test.team", DisplayName: "测试团队",
		Description: "测试用", Category: template.CategoryTeam,
		Version: "1.0.0", Spec: testTemplateSpecJSON(), Source: template.SourceUser,
	})
	require.NoError(t, err)
	return res
}

func TestTemplateServiceRegisterListGetAndTenantIsolation(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)

	res := registerTestTemplate(t, svc, "tenant-a")
	require.False(t, res.AlreadyExisted)

	// 同内容哈希幂等
	dup, err := svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.team", Version: "1.0.0", Spec: testTemplateSpecJSON()})
	require.NoError(t, err)
	require.True(t, dup.AlreadyExisted)
	require.Equal(t, res.Version.ID, dup.Version.ID)

	// 同版本不同内容 → 错误
	conflict, err := svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.team", Version: "1.0.0",
		Spec: json.RawMessage(`{"agents":[{"name":"other","modelRef":"default/general"}]}`)})
	require.Error(t, err)
	require.Nil(t, conflict)
	require.Contains(t, err.Error(), "内容不一致")

	// 断引用 spec → 400 中文错误
	_, err = svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.bad", Version: "1.0.0",
		Spec: json.RawMessage(`{"agents":[{"name":"a"}],"relations":[{"fromRef":"ghost","toRef":"a","relationType":"peer"}]}`)})
	require.Error(t, err)
	require.Contains(t, err.Error(), "模板 spec 校验失败")

	// 列表 + category 过滤
	page, err := svc.List("tenant-a", TemplateListFilter{Category: template.CategoryTeam})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Equal(t, "io.zerone.test.team", page.Items[0].Name)
	require.Equal(t, "1.0.0", page.Items[0].LatestVersion)

	// 跨租户隔离
	other, err := svc.List("tenant-b", TemplateListFilter{})
	require.NoError(t, err)
	require.EqualValues(t, 0, other.Total)
	_, err = svc.Get("tenant-b", res.Template.ID)
	require.Error(t, err)

	// 详情 + 版本 spec
	detail, err := svc.Get("tenant-a", res.Template.ID)
	require.NoError(t, err)
	require.Len(t, detail.Versions, 1)
	require.Equal(t, 2, detail.Versions[0].Summary.Agents)
	ver, err := svc.GetVersion("tenant-a", res.Template.ID, "1.0.0")
	require.NoError(t, err)
	require.Contains(t, ver.Spec, "coordinator")
}

func TestTemplateServiceResolvePreviewAndMapping(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	// 预览：全量计划
	mapping := TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}}
	plan, err := svc.Resolve("tenant-a", res.Template.ID, "", mapping, nil, StrategyFail)
	require.NoError(t, err)
	require.Len(t, plan.Conflicts, 0)
	kinds := map[string]int{}
	for _, it := range plan.Items {
		kinds[it.Kind]++
	}
	require.Equal(t, 2, kinds["agent"])
	require.Equal(t, 1, kinds["group"])
	require.Equal(t, 1, kinds["channel"])
	require.Equal(t, 1, kinds["relation"])
	require.Equal(t, 1, kinds["workflow"])
	require.Equal(t, 1, kinds["stateSchema"])
	require.Equal(t, 1, kinds["personalityTemplate"])
	// 扩展依赖缺失列入计划
	require.Len(t, plan.MissingExtensions, 1)
	require.Contains(t, plan.MissingExtensions[0], "io.zerone.organization.emotion")

	// 缺 mapping → 400
	_, err = svc.Resolve("tenant-a", res.Template.ID, "", TemplateMapping{}, nil, StrategyFail)
	require.Error(t, err)
	require.Contains(t, err.Error(), "缺少模型映射")

	// namePrefix 生效
	plan, err = svc.Resolve("tenant-a", res.Template.ID, "", TemplateMapping{
		ModelRefs: map[string]string{"default/general": "gpt-4o"}, NamePrefix: "acme-",
	}, []string{"agents"}, StrategyFail)
	require.NoError(t, err)
	require.Len(t, plan.Items, 2)
	require.Equal(t, "acme-coordinator", plan.Items[0].Name)
}

func installTestTemplate(t *testing.T, svc *TemplateService, tenantID string, id uint64, key string) (*TemplateInstallResult, error) {
	return svc.Install(tenantID, id, InstallOptions{
		Version:  "1.0.0",
		Mapping:  TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
		Strategy: StrategyFail, Force: true,
		IdempotencyKey: key,
		Actor:          "admin-1",
	})
}

func TestTemplateServiceInstallFullAndResources(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	result, err := installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)
	require.False(t, result.IdempotentReplay)
	require.Len(t, result.Created["agents"], 2)
	require.Len(t, result.Created["groups"], 1)
	require.Len(t, result.Created["relations"], 1)
	require.Len(t, result.Created["workflows"], 1)
	require.Len(t, result.Created["stateSchemas"], 1)
	require.Len(t, result.Created["personalityTemplates"], 1)

	// 资源真实落库
	var agents []agent.AgentConfig
	require.NoError(t, db.Where("tenant_id=?", "tenant-a").Find(&agents).Error)
	require.Len(t, agents, 2)
	require.Equal(t, "gpt-4o", agents[0].ModelID)
	require.NotEmpty(t, agents[0].ContentHash)

	var members []collaboration.GroupMember
	require.NoError(t, db.Find(&members).Error)
	require.Len(t, members, 2)
	// MySQL NO_ZERO_DATE 回归：JoinedAt 必须显式赋值，零值在生产安装会直接报错
	for _, m := range members {
		require.False(t, m.JoinedAt.IsZero(), "GroupMember.JoinedAt 不能是零值（MySQL 严格模式会拒绝写入）")
	}
	var channels []collaboration.Channel
	require.NoError(t, db.Find(&channels).Error)
	require.Len(t, channels, 1)

	var rels []agentrelation.AgentRelation
	require.NoError(t, db.Find(&rels).Error)
	require.Len(t, rels, 1)
	require.Equal(t, "reports_to", rels[0].RelationType)

	var steps []workflow.Step
	require.NoError(t, db.Find(&steps).Error)
	require.Len(t, steps, 2)

	var schemas []rundomain.StateSchema
	require.NoError(t, db.Find(&schemas).Error)
	require.Len(t, schemas, 1)
}

func TestTemplateServiceInstallIdempotencyReplay(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	first, err := installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)
	second, err := installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)
	require.True(t, second.IdempotentReplay)
	require.Equal(t, first.InstallID, second.InstallID)
	require.Equal(t, first.Created, second.Created)

	// 同 key 不同 mapping → 409
	_, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail, Force: true, IdempotencyKey: "key-1",
		Mapping: TemplateMapping{ModelRefs: map[string]string{"default/general": "other-model"}},
	})
	require.Error(t, err)
	code, ok := IsTemplateError(err)
	require.True(t, ok)
	require.Equal(t, 409, code)
}

func TestTemplateServiceInstallConflictsFailAndRename(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")
	_, err := installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)

	// fail 策略：全部同名 → 409 且带冲突列表
	_, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Force: true,
		Mapping:  TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
		Strategy: StrategyFail,
	})
	require.Error(t, err)
	code, ok := IsTemplateError(err)
	require.True(t, ok)
	require.Equal(t, 409, code)
	te := err.(*TemplateError)
	require.NotEmpty(t, te.Plan.Conflicts)

	// rename 策略：自动 -2 后缀
	result, err := svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Force: true,
		Mapping:  TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
		Strategy: StrategyRename, IdempotencyKey: "key-2",
	})
	require.NoError(t, err)
	require.Contains(t, result.Created["agents"], "coordinator-2")
	require.NotEmpty(t, result.Plan.Renames)
}

func TestTemplateServiceInstallSectionsPartial(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	result, err := svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail, Force: true,
		Mapping:  TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
		Sections: []string{"agents", "groups"},
	})
	require.NoError(t, err)
	require.Len(t, result.Created["agents"], 2)
	require.Len(t, result.Created["groups"], 1)
	require.Empty(t, result.Created["relations"])
	require.Empty(t, result.Created["workflows"])

	// relations 依赖 agents：只选 relations → 400
	_, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail, Force: true,
		Mapping:  TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
		Sections: []string{"relations"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "relations 段依赖 agents 段")

	// 未知 section → 400
	_, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Sections: []string{"nope"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "未知段")
}

func TestTemplateServiceExtensionDepsBlockingAndForce(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	// 未安装扩展 + 未 force → 409
	_, err := svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail,
		Mapping: TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
	})
	require.Error(t, err)
	code, ok := IsTemplateError(err)
	require.True(t, ok)
	require.Equal(t, 409, code)
	require.Contains(t, err.Error(), "缺少已启用的扩展依赖")

	// 安装并启用扩展后 → 通过
	extSvc := NewExtensionService(db)
	_, err = extSvc.Register("tenant-a", RegisterExtensionInput{
		Manifest: testExtensionManifest("io.zerone.organization.emotion", "1.2.0"), Source: "seed"})
	require.NoError(t, err)
	var ext extension.Extension
	require.NoError(t, db.Where("tenant_id=? AND name=?", "tenant-a", "io.zerone.organization.emotion").First(&ext).Error)
	_, err = NewExtensionLifecycleService(db).Install("tenant-a", ext.ID, "1.2.0", "admin")
	require.NoError(t, err)

	result, err := svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail,
		Mapping: TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
	})
	require.NoError(t, err)
	require.Len(t, result.Created["agents"], 2)
}

func TestTemplateServiceInstallRollbackOnFailure(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)

	// 同一群组内两个同名频道 → 唯一索引冲突，安装应整体回滚（agent 不残留）
	bad := json.RawMessage(`{
	  "agents": [{"name":"coordinator","title":"协调者","systemPrompt":"x","modelRef":"default/general"}],
	  "groups": [{"name":"g1","channels":["dup","dup"],"memberRefs":["coordinator"]}]
	}`)
	res, err := svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.badwf", Version: "1.0.0", Spec: bad})
	require.NoError(t, err)

	_, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyFail,
		Mapping: TemplateMapping{ModelRefs: map[string]string{"default/general": "gpt-4o"}},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("tenant_id=?", "tenant-a").Count(&count).Error)
	require.EqualValues(t, 0, count, "事务回滚后不应残留 agent")
	require.NoError(t, db.Model(&collaboration.Group{}).Where("tenant_id=?", "tenant-a").Count(&count).Error)
	require.EqualValues(t, 0, count, "事务回滚后不应残留 group")
}

func TestTemplateServiceExportRoundTrip(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")
	_, err := installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)

	// 导出当前配置
	spec, err := svc.Export("tenant-a", ExportSelection{})
	require.NoError(t, err)
	require.Len(t, spec.Agents, 2)
	require.Len(t, spec.Groups, 1)
	require.Len(t, spec.Relations, 1)
	require.Len(t, spec.Workflows, 1)
	require.Len(t, spec.StateSchemas, 1)
	require.Equal(t, "gpt-4o", spec.Agents[0].ModelRef)

	// 导出的 spec 可直接注册为模板
	raw, err := json.Marshal(spec)
	require.NoError(t, err)
	_, err = svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.exported", Version: "1.0.0", Spec: raw, Source: template.SourceUser})
	require.NoError(t, err, "导出 spec 应能通过模板校验")

	// 换租户安装导出模板（round-trip）
	// 导出 spec 在 tenant-b 注册后安装（跨租户私有模板不可直接安装）
	res3, err := svc.Register("tenant-b", RegisterTemplateInput{
		Name: "io.zerone.test.exported", Version: "1.0.0", Spec: raw, Source: template.SourceUser})
	require.NoError(t, err)
	result, err := svc.Install("tenant-b", res3.Template.ID, InstallOptions{
		Version: "1.0.0", Strategy: StrategyRename, Force: true,
		Mapping: TemplateMapping{
			ModelRefs:  map[string]string{"gpt-4o": "claude-4"},
			NamePrefix: "b-",
		},
		IdempotencyKey: "rt-1",
	})
	require.NoError(t, err)
	require.Contains(t, result.Created["agents"], "b-coordinator")
	var agents []agent.AgentConfig
	require.NoError(t, db.Where("tenant_id=?", "tenant-b").Find(&agents).Error)
	require.Len(t, agents, 2)
	require.Equal(t, "claude-4", agents[0].ModelID)
}

func TestTemplateServiceSampleDataSkippedAndWritten(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	runSvc := NewRunService(db)
	svc.SetRunService(runSvc)

	spec := json.RawMessage(`{
	  "stateSchemas": [{"namespace":"io.zerone.test","name":"profile","version":"1.0.0","schema":{"type":"object","properties":{"score":{"type":"integer"}},"required":["score"]}}],
	  "sampleData": [{"kind":"state","namespace":"io.zerone.test","subjectType":"agent","subjectId":"agent-1","data":{"score":42}}]
	}`)
	res, err := svc.Register("tenant-a", RegisterTemplateInput{
		Name: "io.zerone.test.sample", Version: "1.0.0", Spec: spec})
	require.NoError(t, err)

	// 未提供 target_run_id → skipped
	result, err := svc.Install("tenant-a", res.Template.ID, InstallOptions{Version: "1.0.0"})
	require.NoError(t, err)
	require.Len(t, result.Skipped, 1)
	require.Contains(t, result.Skipped[0], "target_run_id")

	// 提供 target_run_id → 写入 run_states
	created, err := runSvc.Create("tenant-a", CreateRunInput{Name: "run-1"})
	require.NoError(t, err)
	result, err = svc.Install("tenant-a", res.Template.ID, InstallOptions{
		Version: "1.0.0", TargetRunID: created.ID, IdempotencyKey: "sd-1",
	})
	require.NoError(t, err)
	require.Len(t, result.Created["sampleData"], 1)
	var states []rundomain.RunState
	require.NoError(t, db.Where("tenant_id=? AND run_id=?", "tenant-a", created.ID).Find(&states).Error)
	require.Len(t, states, 1)
	require.EqualValues(t, 42, states[0].Data["score"])
}

func TestTemplateServiceEnsureSeedTemplatesIdempotent(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)

	require.NoError(t, svc.EnsureSeedTemplates())
	require.NoError(t, svc.EnsureSeedTemplates())

	var count int64
	require.NoError(t, db.Model(&template.TemplateDefinition{}).Where("tenant_id=?", "default").Count(&count).Error)
	require.EqualValues(t, 1, count)

	// 共享种子：其他租户可见可预览
	page, err := svc.List("tenant-a", TemplateListFilter{})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	names := []string{page.Items[0].Name}
	require.Contains(t, names, "io.zerone.team.basic")
	require.NotContains(t, names, "io.zerone.speeding.sample-crew", "vertical examples must not be seeded by Hub Core")

	plan, err := svc.Resolve("tenant-a", page.Items[0].TemplateDefinition.ID, "",
		TemplateMapping{ModelRefs: map[string]string{
			"default/general": "gpt-4o", "default/roleplay": "gpt-4o",
		}},
		nil, StrategyFail)
	require.NoError(t, err)
	require.Len(t, plan.Items, 8) // 3 agents + group + 2 channels + 2 relations
}

func TestTemplateServiceCrossTenantInstallIsolation(t *testing.T) {
	db := newTemplateTestDB(t)
	svc := NewTemplateService(db)
	res := registerTestTemplate(t, svc, "tenant-a")

	// tenant-b 不能安装 tenant-a 的私有模板
	_, err := svc.Install("tenant-b", res.Template.ID, InstallOptions{Version: "1.0.0"})
	require.Error(t, err)
	code, ok := IsTemplateError(err)
	require.True(t, ok)
	require.Equal(t, 404, code)

	// tenant-a 安装后 tenant-b 无资源
	_, err = installTestTemplate(t, svc, "tenant-a", res.Template.ID, "key-1")
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&agent.AgentConfig{}).Where("tenant_id=?", "tenant-b").Count(&count).Error)
	require.EqualValues(t, 0, count)
}
