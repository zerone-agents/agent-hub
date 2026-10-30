package chat

// H1 wire 契约回归：固定向量由独立参考工具（python rfc8785）生成并核对，
// 绝不由被测函数产生期望值（见 testdata/sync-v2-vectors.json generatedBy）。
// 覆盖：键序无关性、中文/emoji/转义、空消息、最大 version、delete 载荷、
// 以及重复 key / 未知字段 / version 与 digest 与 UUID 与 ordinal 的全部拒绝形态。

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/stretchr/testify/require"
)

type syncVectors struct {
	Accept []struct {
		Name         string `json:"name"`
		Raw          string `json:"raw"`
		Digest       string `json:"digest"`
		Version      string `json:"version"`
		DigestEquals string `json:"digestEquals"`
	} `json:"accept"`
	Reject []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
		Raw    string `json:"raw"`
	} `json:"reject"`
	RFC8785 []struct {
		Name   string `json:"name"`
		Input  string `json:"input"`
		SHA256 string `json:"sha256"`
	} `json:"rfc8785"`
}

func loadSyncVectors(t *testing.T) *syncVectors {
	t.Helper()
	b, err := os.ReadFile("testdata/sync-v2-vectors.json")
	require.NoError(t, err)
	var v syncVectors
	require.NoError(t, json.Unmarshal(b, &v))
	return &v
}

// TestSyncEnvelopeRFC8785Pins：独立工具产出的 RFC 8785 规范化向量（浮点
// ES6 序列化、Unicode/转义最小化）锚定 JCS 依赖行为，防依赖版本漂移导致
// 两端摘要分叉。
func TestSyncEnvelopeRFC8785Pins(t *testing.T) {
	for _, tc := range loadSyncVectors(t).RFC8785 {
		t.Run(tc.Name, func(t *testing.T) {
			canon, err := jsoncanonicalizer.Transform([]byte(tc.Input))
			require.NoError(t, err)
			sum := sha256.Sum256(canon)
			require.Equal(t, tc.SHA256, hex.EncodeToString(sum[:]),
				"canonical=%s", canon)
		})
	}
}

func TestSyncEnvelopeVectors(t *testing.T) {
	v := loadSyncVectors(t)
	digests := map[string]string{}

	for _, tc := range v.Accept {
		t.Run(tc.Name, func(t *testing.T) {
			e, err := ParseEnvelope([]byte(tc.Raw))
			require.NoError(t, err, "raw=%s", tc.Raw)

			got, err := DigestEnvelope(e)
			require.NoError(t, err)
			require.Equal(t, tc.Digest, got)
			require.Equal(t, tc.Version, strconv.FormatInt(e.Version, 10))
			require.Equal(t, ProtocolVersion, e.ProtocolVersion)
			require.Equal(t, ProjectionVersion, e.ProjectionVersion)

			digests[tc.Name] = got

			if tc.DigestEquals != "" {
				require.Equal(t, digests[tc.DigestEquals], got,
					"同逻辑内容键序不同，摘要必须一致")
			}
		})
	}

	for _, tc := range v.Reject {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := ParseEnvelope([]byte(tc.Raw))
			require.Error(t, err, "raw=%s", tc.Raw)
			require.Contains(t, err.Error(), tc.Reason,
				"错误信息须可诊断（含向量标注原因）")
		})
	}
}

// TestSyncEnvelopeBasicShape：除向量外，对解析出的业务结构做形状断言
// （消息序数、会话元数据、delete 载荷为空）。
func TestSyncEnvelopeBasicShape(t *testing.T) {
	v := loadSyncVectors(t)
	var basic string
	for _, tc := range v.Accept {
		if tc.Name == "replace-basic" {
			basic = tc.Raw
		}
	}
	require.NotEmpty(t, basic)

	e, err := ParseEnvelope([]byte(basic))
	require.NoError(t, err)
	require.Equal(t, KindReplace, e.Kind)
	require.Equal(t, "6f9619ff-8b86-d011-b42d-00cf4fc964ff", e.SourceID)
	require.Equal(t, "session-alpha-001", e.AppSessionID)
	require.Len(t, e.Payload.Messages, 3)
	for i, m := range e.Payload.Messages {
		require.Equal(t, int64(i), m.Ordinal)
		require.NotEmpty(t, m.ID)
		require.NotEmpty(t, m.CreatedAt)
	}
	require.Equal(t, "session-alpha-001", e.Payload.Session.ID)
	require.True(t, e.Payload.Session.IsUserBound)
	require.Equal(t, "glm-5-air", e.Payload.Session.ModelSelectionID)

	// delete 载荷：无 replace 业务结构
	for _, tc := range v.Accept {
		if tc.Name == "delete-basic" {
			d, err := ParseEnvelope([]byte(tc.Raw))
			require.NoError(t, err)
			require.Equal(t, KindDelete, d.Kind)
			require.Nil(t, d.Payload)
			require.Equal(t, "null", string(d.RawPayload))
		}
	}
}
