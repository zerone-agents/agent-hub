package chat

// SessionStore 云同步 v2 wire 契约（issue #232，契约 v0.1 / 实施计划 H1）。
//
// 设计要点：
//   - App → Hub 单向；envelope 顶层字段 camelCase，业务载荷（session/messages）
//     沿用 v1 snake_case（表示收敛，见 issue「实施时先解决的表示细节」）。
//   - version 为 wire 十进制字符串（防 JS 2^53 精度丢失），范围 1..MaxInt64；
//     claim 的 0 只出现在 SyncState，不允许出现在 envelope。
//   - digest = SHA-256(JCS(RFC 8785) 规范化的 envelope 业务字段)，digest 字段
//     自身不参与；Hub 端解析后重算并核对。
//   - 严格解析：未知字段、任意层重复对象键、非有限数字（NaN/Infinity）、
//     非法 UTF-8、UUID/digest/version 形态、ordinal 连续性全部拒绝——
//     任何静默容错都会让两端对同一文档算出不同摘要。

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

const (
	ProtocolVersion   int64 = 2
	ProjectionVersion int64 = 1
)

// Owner：同步键的归属（target + 认证 tenant/user）。不来自 body——
// 账号或 Hub 切换必须建立独立绑定（契约 §3）。
type Owner struct {
	TenantID string
	UserID   string
}

// syncTimeFormats 与 v1（services/chat_service.go timeFormats）保持一致：
// v2 载荷沿用 v1 的时间字符串形态，两端解析域必须相同。
var syncTimeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999Z07:00",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05.999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05",
}

// ParseSyncTime 解析 v2 载荷时间字段；全部格式不匹配时返回错误。
func ParseSyncTime(s string) (time.Time, error) {
	for _, f := range syncTimeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %q", s)
}

// Kind 是快照种类；delete 携带永久 tombstone（见契约 §8）。
type Kind string

const (
	KindReplace Kind = "replace"
	KindDelete  Kind = "delete"
)

// SyncSessionPayload：v2 会话元数据。字段集 = v1 SessionInput 去掉 messages /
// user_name / org（身份只取认证上下文，不信任 body）。agent_id 属 v1 元数据
// （v1 push 同步该字段），必须携带——否则 replace 会清空 agent 绑定、破坏
// agent chat 的归属校验。
type SyncSessionPayload struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	Model             string `json:"model"`
	ModelSelectionID  string `json:"model_selection_id"`
	SystemPrompt      string `json:"system_prompt"`
	Status            string `json:"status"`
	Mode              string `json:"mode"`
	ProviderID        string `json:"provider_id"`
	AgentID           string `json:"agent_id"`
	PermissionProfile string `json:"permission_profile"`
	Hidden            bool   `json:"hidden"`
	ExtraDirectories  string `json:"extra_directories"`
	IsUserBound       bool   `json:"is_user_bound"`
}

// SyncMessagePayload：v1 MessageInput + ordinal（从 0 连续递增；展示排序
// 权威，同 timestamp 不换序——契约 §6）。
type SyncMessagePayload struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
	Hidden     bool   `json:"hidden"`
	TokenUsage string `json:"token_usage"`
	Feedback   string `json:"feedback"`
	Ordinal    int64  `json:"ordinal"`
}

// ReplacePayload 是 kind=replace 的业务载荷（delete 为 null）。
type ReplacePayload struct {
	Session  SyncSessionPayload   `json:"session"`
	Messages []SyncMessagePayload `json:"messages"`
}

// Envelope：一次冻结的全量快照。RawPayload 保留原始载荷字节——摘要规范化
// 以它为输入，保证与 App 端逐字节一致（解析后的结构化字段不参与重序列化，
// 避免数字字面量/转义形式漂移）。
type Envelope struct {
	ProtocolVersion   int64
	ProjectionVersion int64
	SourceID          string
	AppSessionID      string
	Version           int64
	SnapshotID        string
	Kind              Kind
	Payload           *ReplacePayload // kind=delete 时为 nil
	RawPayload        json.RawMessage // delete 时为字面量 "null"
	Digest            string
}

// SyncState：同步键的权威状态（H2/H4 消费）。version wire 为十进制字符串；
// claim 阶段（claimed-empty / claimed-legacy）仅 version=0 且缺快照身份三件。
type SyncState struct {
	SourceID   string `json:"sourceId"`
	Version    string `json:"version"`
	SnapshotID string `json:"snapshotId,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status"` // claimed-empty | claimed-legacy | active | deleted
}

// Reply 是 PUT/claim 的精确确认：status ∈ applied|duplicate|superseded|
// conflict|deleted（+ claim 专用状态）；HTTP 200 不能替代逐项回显。
type Reply struct {
	Status string    `json:"status"`
	State  SyncState `json:"state"`
}

// 状态常量（Reply.Status / SyncState.Status）。Status* 是 apply 的精确确认；
// State* 是同步键的阶段状态；"deleted" 在两侧同名同值（tombstone 既是确认
// 结果也是键的终态）。
const (
	StatusApplied      = "applied"
	StatusDuplicate    = "duplicate"
	StatusSuperseded   = "superseded"
	StatusConflict     = "conflict"
	StatusDeleted      = "deleted"
	StateClaimedEmpty  = "claimed-empty"
	StateClaimedLegacy = "claimed-legacy"
	StateActive        = "active"
	StateDeleted       = "deleted"
)

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// IsValidSyncUUID：小写规范 UUID（sourceId / snapshotId 的公共形态约束，
// claim 入口复用于 sourceId 校验）。
func IsValidSyncUUID(s string) bool { return uuidPattern.MatchString(s) }

// wireEnvelope 是传输层形状（version 为字符串）；仅用于解码与摘要重建。
type wireEnvelope struct {
	ProtocolVersion   int64           `json:"protocolVersion"`
	ProjectionVersion int64           `json:"projectionVersion"`
	SourceID          string          `json:"sourceId"`
	AppSessionID      string          `json:"appSessionId"`
	Version           string          `json:"version"`
	SnapshotID        string          `json:"snapshotId"`
	Kind              string          `json:"kind"`
	Payload           json.RawMessage `json:"payload"`
	Digest            string          `json:"digest"`
}

// ParseEnvelope 严格解析并校验 wire envelope，最后核对摘要。
func ParseEnvelope(raw []byte) (Envelope, error) {
	if !utf8.Valid(raw) {
		return Envelope{}, errors.New("invalid envelope: not valid UTF-8")
	}
	if err := checkJSONShape(raw); err != nil {
		return Envelope{}, err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var w wireEnvelope
	if err := dec.Decode(&w); err != nil {
		return Envelope{}, fmt.Errorf("invalid envelope: %w", err)
	}

	if w.ProtocolVersion != ProtocolVersion {
		return Envelope{}, fmt.Errorf("unsupported protocolVersion %d (want %d)", w.ProtocolVersion, ProtocolVersion)
	}
	if w.ProjectionVersion != ProjectionVersion {
		return Envelope{}, fmt.Errorf("unsupported projectionVersion %d (want %d)", w.ProjectionVersion, ProjectionVersion)
	}
	if !uuidPattern.MatchString(w.SourceID) {
		return Envelope{}, fmt.Errorf("invalid sourceId %q: want lowercase canonical UUID", w.SourceID)
	}
	if w.AppSessionID == "" || len(w.AppSessionID) > 255 {
		return Envelope{}, fmt.Errorf("invalid appSessionId %q: want 1..255 chars", w.AppSessionID)
	}
	if !uuidPattern.MatchString(w.SnapshotID) {
		return Envelope{}, fmt.Errorf("invalid snapshotId %q: want lowercase canonical UUID", w.SnapshotID)
	}
	if !versionPattern.MatchString(w.Version) {
		return Envelope{}, fmt.Errorf("invalid version %q: decimal digits without leading zeros/sign/exponent", w.Version)
	}
	v, err := strconv.ParseInt(w.Version, 10, 64)
	if err != nil {
		return Envelope{}, fmt.Errorf("invalid version %q: out of 64-bit range", w.Version)
	}
	if v < 1 {
		return Envelope{}, fmt.Errorf("invalid version %q: envelope version must be >= 1 (0 is claim-only)", w.Version)
	}
	if w.Kind != string(KindReplace) && w.Kind != string(KindDelete) {
		return Envelope{}, fmt.Errorf("invalid kind %q: want replace|delete", w.Kind)
	}
	if !digestPattern.MatchString(w.Digest) {
		return Envelope{}, fmt.Errorf("invalid digest %q: want 64 lowercase hex chars", w.Digest)
	}
	if w.Payload == nil {
		return Envelope{}, errors.New("invalid envelope: payload field is required")
	}

	e := Envelope{
		ProtocolVersion:   w.ProtocolVersion,
		ProjectionVersion: w.ProjectionVersion,
		SourceID:          w.SourceID,
		AppSessionID:      w.AppSessionID,
		Version:           v,
		SnapshotID:        w.SnapshotID,
		Kind:              Kind(w.Kind),
		RawPayload:        w.Payload,
		Digest:            w.Digest,
	}

	switch e.Kind {
	case KindDelete:
		if string(w.Payload) != "null" {
			return Envelope{}, errors.New("invalid payload: delete payload must be null")
		}
	case KindReplace:
		rp, err := parseReplacePayload(w.Payload)
		if err != nil {
			return Envelope{}, err
		}
		if rp.Session.ID != e.AppSessionID {
			return Envelope{}, fmt.Errorf("session id mismatch: payload session.id=%q appSessionId=%q", rp.Session.ID, e.AppSessionID)
		}
		e.Payload = rp
	}

	got, err := DigestEnvelope(e)
	if err != nil {
		return Envelope{}, err
	}
	if got != e.Digest {
		return Envelope{}, fmt.Errorf("digest mismatch: envelope digest %s, computed %s", e.Digest, got)
	}
	return e, nil
}

// parseReplacePayload：载荷形状（恰好 session+messages 两键）+ 严格字段解码 +
// 必填/ordinal 连续性校验。
func parseReplacePayload(payload json.RawMessage) (*ReplacePayload, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(payload, &keys); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if _, ok := keys["session"]; !ok {
		return nil, errors.New("invalid payload: replace payload missing session")
	}
	if _, ok := keys["messages"]; !ok {
		return nil, errors.New("invalid payload: replace payload missing messages")
	}
	for k := range keys {
		if k != "session" && k != "messages" {
			return nil, fmt.Errorf("invalid payload: unknown key %q (want session,messages)", k)
		}
	}

	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var rp ReplacePayload
	if err := dec.Decode(&rp); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if rp.Session.ID == "" {
		return nil, errors.New("invalid payload: session id is required")
	}
	if rp.Session.CreatedAt == "" {
		return nil, errors.New("invalid payload: session created_at is required")
	}
	if rp.Session.UpdatedAt == "" {
		return nil, errors.New("invalid payload: session updated_at is required")
	}
	// 评审修订 S3：时间格式解析期校验（fail-fast 400）——apply 期解析保留为
	// 防御性兜底。
	if _, err := ParseSyncTime(rp.Session.CreatedAt); err != nil {
		return nil, fmt.Errorf("invalid payload: session created_at %q: %w", rp.Session.CreatedAt, err)
	}
	if _, err := ParseSyncTime(rp.Session.UpdatedAt); err != nil {
		return nil, fmt.Errorf("invalid payload: session updated_at %q: %w", rp.Session.UpdatedAt, err)
	}
	for i := range rp.Messages {
		if _, err := ParseSyncTime(rp.Messages[i].CreatedAt); err != nil {
			return nil, fmt.Errorf("invalid payload: messages[%d].created_at %q: %w", i, rp.Messages[i].CreatedAt, err)
		}
	}
	for i := range rp.Messages {
		m := &rp.Messages[i]
		if m.ID == "" {
			return nil, fmt.Errorf("invalid payload: messages[%d].id is required", i)
		}
		if m.CreatedAt == "" {
			return nil, fmt.Errorf("invalid payload: messages[%d].created_at is required", i)
		}
		if m.Ordinal != int64(i) {
			if i == 0 {
				return nil, fmt.Errorf("invalid payload: message ordinal must start at 0, got %d", m.Ordinal)
			}
			return nil, fmt.Errorf("invalid payload: message ordinal gap: expected %d, got %d", i, m.Ordinal)
		}
	}
	return &rp, nil
}

// DigestEnvelope 计算 SHA-256(JCS(业务 envelope))，hex 小写；digest 字段
// 自身不参与（契约 §5）。
func DigestEnvelope(e Envelope) (string, error) {
	payload := e.RawPayload
	if payload == nil {
		payload = json.RawMessage("null")
	}
	wire := struct {
		ProtocolVersion   int64           `json:"protocolVersion"`
		ProjectionVersion int64           `json:"projectionVersion"`
		SourceID          string          `json:"sourceId"`
		AppSessionID      string          `json:"appSessionId"`
		Version           string          `json:"version"`
		SnapshotID        string          `json:"snapshotId"`
		Kind              Kind            `json:"kind"`
		Payload           json.RawMessage `json:"payload"`
	}{
		ProtocolVersion:   e.ProtocolVersion,
		ProjectionVersion: e.ProjectionVersion,
		SourceID:          e.SourceID,
		AppSessionID:      e.AppSessionID,
		Version:           strconv.FormatInt(e.Version, 10),
		SnapshotID:        e.SnapshotID,
		Kind:              e.Kind,
		Payload:           payload,
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("digest marshal: %w", err)
	}
	canon, err := jsoncanonicalizer.Transform(b)
	if err != nil {
		return "", fmt.Errorf("canonicalize: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// checkJSONShape 逐 token 校验文档形状：顶层必须是单个 JSON 对象、语法错误
// （含 NaN/Infinity 字面量，encoding/json 原生拒绝）与任意层重复对象键
// （encoding/json 静默取后者，会造成两端摘要分叉，必须显式拒绝）。
func checkJSONShape(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	type frame struct {
		isObject  bool
		expectKey bool
		keys      map[string]struct{}
	}
	var stack []frame
	sawTopObject := false // 顶层对象完成后，任何后续 token 均为尾随内容（评审探针 P5）
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		if len(stack) == 0 {
			if !sawTopObject {
				if d, ok := tok.(json.Delim); ok && d == '{' {
					sawTopObject = true
					stack = append(stack, frame{isObject: true, expectKey: true, keys: map[string]struct{}{}})
					continue
				}
				return errors.New("invalid JSON: envelope must be a single object")
			}
			return errors.New("invalid JSON: trailing content after envelope")
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{':
				stack = append(stack, frame{isObject: true, expectKey: true, keys: map[string]struct{}{}})
			case '[':
				stack = append(stack, frame{})
			case '}', ']':
				stack = stack[:len(stack)-1]
				if len(stack) > 0 && stack[len(stack)-1].isObject {
					stack[len(stack)-1].expectKey = true
				}
			}
		case string:
			top := &stack[len(stack)-1]
			if top.isObject && top.expectKey {
				if _, dup := top.keys[t]; dup {
					return fmt.Errorf("duplicate key %q", t)
				}
				top.keys[t] = struct{}{}
				top.expectKey = false
			} else if top.isObject {
				top.expectKey = true
			}
		default: // 数字 / bool / null 标量
			top := &stack[len(stack)-1]
			if top.isObject {
				top.expectKey = true
			}
		}
	}
	if len(stack) != 0 {
		return errors.New("invalid JSON: unexpected end of input")
	}
	return nil
}
