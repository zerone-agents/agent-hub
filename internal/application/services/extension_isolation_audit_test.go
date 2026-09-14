// H7.4 租户/Run 隔离审计测试矩阵 + 敏感字段扫描测试。
//
// 矩阵（不改动业务代码，只验证既有隔离行为）：
//
//	| # | 对象          | 操作                       | 期望              |
//	|---|---------------|----------------------------|-------------------|
//	| 1 | extensions    | B 租户 List                | 不含 A 的扩展     |
//	| 2 | extensions    | B 租户 Get(A.id)           | 404               |
//	| 3 | extensions    | B 租户 GetVersion(A.id)    | 404               |
//	| 4 | extensions    | B 租户 Install(A.id)       | 404               |
//	| 5 | extension 授权| B 租户 Enforce 借 A 的授权 | false             |
//	| 6 | runs          | B 租户 Get(A.run)          | not found         |
//	| 7 | runs          | B 租户 List                | 空                |
//	| 8 | run_states    | B 租户按 run 读状态        | 空（Run 隔离）    |
//	| 9 | agents        | B 租户 ListAll/GetByID     | 空 / not found    |
//	|10 | groups        | B 租户 Groups/Group        | 空 / not found    |
//	|11 | channels+消息 | B 租户 Channel/ListForChannel | not found / 空 |
//	|12 | providers DTO | 敏感字段扫描               | 无明文密钥        |
package services

import (
	"encoding/json"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/agentrelation"
	"control-panel/internal/domain/collaboration"
	"control-panel/internal/domain/extension"
	"control-panel/internal/domain/provider"
	rundomain "control-panel/internal/domain/run"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// isolationFixture 装配一份覆盖矩阵所需全部表的内存库。
type isolationFixture struct {
	db        *gorm.DB
	registry  *ExtensionService
	lifecycle *ExtensionLifecycleService
	authz     *ExtensionAuthzService
	runs      *RunService
	agents    *repository.AgentRepository
	collab    *CollaborationService
	messages  *repository.AgentMessageRepository
}

func newIsolationFixture(t *testing.T) *isolationFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&extension.Extension{}, &extension.Version{}, &extension.Install{},
		&extension.Grant{}, &extension.AccessAudit{},
		&rundomain.Run{}, &rundomain.RunAgent{}, &rundomain.RunState{},
		&agent.AgentConfig{},
		&collaboration.Group{}, &collaboration.Channel{}, &collaboration.MemberAudit{},
		&agentrelation.AgentRelation{}, &agentrelation.AgentMessage{},
	))
	f := &isolationFixture{
		db:        db,
		registry:  NewExtensionService(db),
		lifecycle: NewExtensionLifecycleService(db),
		authz:     NewExtensionAuthzService(db),
		runs:      NewRunService(db),
		agents:    repository.NewAgentRepositoryWithDB(db),
		collab:    NewCollaborationService(db),
		messages:  repository.NewAgentMessageRepositoryWithDB(db),
	}
	f.lifecycle.SetAuthzService(f.authz)
	return f
}

// seedTenantA 在租户 A 落一套全量数据，返回关键 ID。
func (f *isolationFixture) seedTenantA(t *testing.T) (extID uint64, runID string, agentID uint64, groupID, channelID, messageID string) {
	t.Helper()
	// 扩展 + 安装 + 授权
	res, err := f.registry.Register("tenant-a", RegisterExtensionInput{
		Manifest: []byte(authzManifest("io.zerone.iso-a", "1.0.0",
			`[{"permission":"state","scope":"*","actions":["read"]}]`)),
		Source: extension.SourceUpload,
	})
	require.NoError(t, err)
	_, err = f.lifecycle.Install("tenant-a", res.Extension.ID, "1.0.0", "admin-a")
	require.NoError(t, err)
	extID = res.Extension.ID

	// run + run_state
	runID = uuid.NewString()
	require.NoError(t, f.db.Create(&rundomain.Run{
		ID: runID, TenantID: "tenant-a", Name: "run-a", Status: "running",
	}).Error)
	require.NoError(t, f.db.Create(&rundomain.RunState{
		TenantID: "tenant-a", RunID: runID, Namespace: "io.zerone.iso-a",
		SchemaName: "mood", SchemaVersion: "1.0.0", SchemaHash: "h",
		SubjectType: "agent", SubjectID: "1", Revision: 1,
		Data: map[string]any{"mood": "calm"},
	}).Error)

	// agent
	agentCfg := &agent.AgentConfig{
		TenantID: "tenant-a", Name: "agent-a",
		ContentHash: "c", SystemPrompt: "p",
	}
	require.NoError(t, f.agents.Create("tenant-a", agentCfg))
	agentID = agentCfg.ID

	// group + channel + relation + message
	group, err := f.collab.CreateGroup("tenant-a", "group-a", "d", "private", "admin-a")
	require.NoError(t, err)
	groupID = group.ID
	channel := collaboration.Channel{
		ID: uuid.NewString(), TenantID: "tenant-a", GroupID: groupID,
		Name: "chan-a", Visibility: "private", CreatedBy: "admin-a",
	}
	require.NoError(t, f.db.Create(&channel).Error)
	channelID = channel.ID
	relation := agentrelation.AgentRelation{
		TenantID: "tenant-a", Scope: "global",
		SourceAgentID: agentID, TargetAgentID: agentID + 1000,
		RelationType: "friend",
	}
	require.NoError(t, f.db.Create(&relation).Error)
	msg := agentrelation.AgentMessage{
		ID: uuid.NewString(), TenantID: "tenant-a", RelationID: relation.ID,
		ChannelID: channelID, ConversationID: uuid.NewString(),
		RootMessageID: uuid.NewString(),
		Scope:         "global", SourceAgentID: agentID, SourceAgent: "agent-a",
		TargetAgentID: agentID + 1000, TargetAgent: "agent-b",
		Action: "chat", DeliveryPolicy: "deliver", ContextPolicy: "full",
		Content: "hello", Status: "completed",
	}
	require.NoError(t, f.db.Create(&msg).Error)
	messageID = msg.ID
	return
}

func TestIsolationMatrixCrossTenant(t *testing.T) {
	f := newIsolationFixture(t)
	extID, runID, agentID, groupID, channelID, messageID := f.seedTenantA(t)

	// 1. 扩展 List：B 租户不可见 A 的扩展
	page, err := f.registry.List("tenant-b", ExtensionListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Zero(t, page.Total)
	require.Empty(t, page.Items)

	// 2. 扩展 Get：B 租户 404
	_, err = f.registry.Get("tenant-b", extID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 3. 扩展 GetVersion：B 租户 404
	_, err = f.registry.GetVersion("tenant-b", extID, "1.0.0")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// 4. 扩展 Install：B 租户 404
	_, err = f.lifecycle.Install("tenant-b", extID, "1.0.0", "admin-b")
	require.Error(t, err)
	code, ok := IsLifecycleError(err)
	require.True(t, ok)
	require.Equal(t, 404, code)

	// 5. 授权不跨租户：B 租户同名扩展没有 A 的授权
	require.False(t, f.authz.Enforce("tenant-b", "io.zerone.iso-a", "state", "read", "", ""))

	// 6. run Get：B 租户 not found（服务包装为 rundomain.ErrNotFound）
	_, err = f.runs.Get("tenant-b", runID)
	require.ErrorIs(t, err, rundomain.ErrNotFound)

	// 7. run List：B 租户空
	runs, err := f.runs.List("tenant-b", "", 50)
	require.NoError(t, err)
	require.Empty(t, runs)

	// 8. run_states：B 租户按 run 读 → 空（Run 隔离）；同租户另一 run 也不可见
	var states []rundomain.RunState
	require.NoError(t, f.db.Where("tenant_id=? AND run_id=?", "tenant-b", runID).Find(&states).Error)
	require.Empty(t, states)
	runB := uuid.NewString()
	require.NoError(t, f.db.Create(&rundomain.Run{
		ID: runB, TenantID: "tenant-a", Name: "run-b", Status: "running",
	}).Error)
	require.NoError(t, f.db.Where("tenant_id=? AND run_id=?", "tenant-a", runB).Find(&states).Error)
	require.Empty(t, states) // A 的 state 在 B run 内不可见

	// 9. agents：B 租户空 / not found
	agentList, err := f.agents.ListAll("tenant-b")
	require.NoError(t, err)
	require.Empty(t, agentList)
	_, err = f.agents.GetByID("tenant-b", agentID)
	require.Error(t, err)

	// 10. groups：B 租户空 / not found
	groups, err := f.collab.Groups("tenant-b")
	require.NoError(t, err)
	require.Empty(t, groups)
	_, err = f.collab.Group("tenant-b", groupID)
	require.Error(t, err)

	// 11. channels + messages：B 租户 not found / 空
	_, err = f.collab.Channel("tenant-b", channelID)
	require.Error(t, err)
	msgs, err := f.messages.ListForChannel("tenant-b", channelID, 100)
	require.NoError(t, err)
	require.Empty(t, msgs)
	// A 租户自身可读（对照组）
	msgsA, err := f.messages.ListForChannel("tenant-a", channelID, 100)
	require.NoError(t, err)
	require.Len(t, msgsA, 1)
	require.Equal(t, messageID, msgsA[0].ID)
}

// TestSensitiveFieldScanProviderDTO 敏感字段扫描：admin API 出参
// （ProviderDTO，List/Get/ListAdmin 共用）不得包含明文 apiSecret/token。
// 检查清单：
//   - lockedApiKey：必须掩码（首4+尾4，中间 ****），绝不出现明文；
//   - attributes：api_key / *token* / *secret* / *password* 键必须掩码；
//   - 掩码回传更新不得把掩码值写回数据库（保留明文）。
func TestSensitiveFieldScanProviderDTO(t *testing.T) {
	const plaintextKey = "sk-live-9f8e7d6c5b4a-secret-0123456789abcdef"

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&provider.ProviderSummary{}, &provider.ProviderAttribute{}, &provider.ProviderModel{}))
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	svc := NewProviderService(providerServiceTestEncryptionKey)
	created, err := svc.Create("tenant-a", &CreateProviderInput{
		Key:          "scan-target",
		Name:         "Scan Target",
		Protocol:     "openai",
		AuthStyle:    "api_key",
		LockedAPIKey: plaintextKey,
		Attributes: map[string]provider.AttrValue{
			"api_key":   {Type: "string", Value: "attr-token-abcdef0123456789"},
			"endpoint":  {Type: "string", Value: "https://api.example.com"},
			"authToken": {Type: "string", Value: "tok-abcdef0123456789"},
		},
	})
	require.NoError(t, err)

	// 1) Create 出参扫描
	raw, err := json.Marshal(created)
	require.NoError(t, err)
	body := string(raw)
	require.NotContains(t, body, plaintextKey)
	require.NotContains(t, body, "attr-token-abcdef0123456789")
	require.NotContains(t, body, "tok-abcdef0123456789")
	require.Contains(t, body, "https://api.example.com") // 非敏感属性原样返回
	require.Contains(t, body, "****")

	// 2) ToDTO（Get/List 出参）扫描
	providerObj, err := svc.GetByID("tenant-a", created.ID)
	require.NoError(t, err)
	dto, err := svc.ToDTO("tenant-a", providerObj)
	require.NoError(t, err)
	raw, err = json.Marshal(dto)
	require.NoError(t, err)
	body = string(raw)
	require.NotContains(t, body, plaintextKey)
	require.NotContains(t, body, "attr-token-abcdef0123456789")
	require.NotContains(t, body, "tok-abcdef0123456789")
	// 掩码形态校验：首4+****+尾4
	require.True(t, strings.Contains(body, "sk-l****cdef"), "lockedApiKey 应为掩码形态: %s", body)
	require.True(t, strings.Contains(body, "attr****6789"), "api_key 属性应为掩码形态: %s", body)

	// 3) 掩码回传更新：掩码值不得写回数据库
	_, err = svc.Update("tenant-a", created.ID, &UpdateProviderInput{
		Attributes: map[string]provider.AttrValue{
			"api_key":  {Type: "string", Value: "attr****6789"}, // 前端原样带回的掩码
			"endpoint": {Type: "string", Value: "https://api2.example.com"},
		},
	})
	require.NoError(t, err)
	var attrRow provider.ProviderAttribute
	require.NoError(t, db.Where("provider_id=? AND attr_key='api_key'", created.ID).First(&attrRow).Error)
	require.Equal(t, "attr-token-abcdef0123456789", attrRow.AttrValue, "掩码回传必须保留库存明文")
	var endpointRow provider.ProviderAttribute
	require.NoError(t, db.Where("provider_id=? AND attr_key='endpoint'", created.ID).First(&endpointRow).Error)
	require.Equal(t, "https://api2.example.com", endpointRow.AttrValue)
}
