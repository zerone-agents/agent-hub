package services

// ChatSyncService：v2 同步协议的应用服务（issue #232 H4）。
//
// 职责：能力宣告（协议/投影版本、摘要算法、大小上限、serverIdentity）
// 与认证上下文 → Owner 的收敛；状态机本体在 ChatSyncRepository。

import (
	"context"

	"control-panel/internal/domain/chat"
	repository "control-panel/internal/infrastructure/persistence"
	"control-panel/pkg/database"
)

const (
	// SyncMaxRequestBytes：单请求 8 MiB 上限（完整 UTF-8 请求体；超限 413，
	// 不分片、不部分替换——issue #232 计划实现选择，App 以能力响应为准）。
	SyncMaxRequestBytes int64 = 8388608
	// SyncDigestAlgorithm：摘要算法标识（JCS 规范化 + SHA-256，小写 hex）。
	SyncDigestAlgorithm = "sha256-jcs-rfc8785"
)

// ChatSyncCapabilities：能力查询响应。serverIdentity 为 hub 对外可达
// base URL（复用 ChatPushPublicURL 配置——部署身份语义一致；未配置为空串，
// App 以实际请求 origin 兜底）。
type ChatSyncCapabilities struct {
	ProtocolVersions   []int64 `json:"protocolVersions"`
	ProjectionVersions []int64 `json:"projectionVersions"`
	DigestAlgorithm    string  `json:"digestAlgorithm"`
	MaxRequestBytes    int64   `json:"maxRequestBytes"`
	Tombstone          bool    `json:"tombstone"`
	ServerIdentity     string  `json:"serverIdentity"`
}

type ChatSyncService struct {
	repo           *repository.ChatSyncRepository
	serverIdentity string
}

func NewChatSyncService(serverIdentity string) *ChatSyncService {
	return &ChatSyncService{
		repo:           repository.NewChatSyncRepository(database.GetDB()),
		serverIdentity: serverIdentity,
	}
}

func (s *ChatSyncService) Capabilities() ChatSyncCapabilities {
	return ChatSyncCapabilities{
		ProtocolVersions:   []int64{chat.ProtocolVersion},
		ProjectionVersions: []int64{chat.ProjectionVersion},
		DigestAlgorithm:    SyncDigestAlgorithm,
		MaxRequestBytes:    SyncMaxRequestBytes,
		Tombstone:          true,
		ServerIdentity:     s.serverIdentity,
	}
}

func (s *ChatSyncService) Claim(ctx context.Context, tenantID, userID, sessionID, sourceID, origin string) (chat.SyncState, error) {
	return s.repo.Claim(ctx, chat.Owner{TenantID: tenantID, UserID: userID}, sessionID, sourceID, origin)
}

func (s *ChatSyncService) Apply(ctx context.Context, tenantID, userID string, e chat.Envelope) (chat.Reply, error) {
	return s.repo.Apply(ctx, chat.Owner{TenantID: tenantID, UserID: userID}, e)
}

func (s *ChatSyncService) GetState(ctx context.Context, tenantID, userID, sessionID string) (chat.SyncState, error) {
	return s.repo.GetState(ctx, chat.Owner{TenantID: tenantID, UserID: userID}, sessionID)
}
