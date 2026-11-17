package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/provider"

	"github.com/stretchr/testify/require"
)

// TestBuildProbeChatOptions 钉死三态装配（issue #263 评审重要 2）：
// 未触碰回落存量；显式清空（&” / provided+”）不复活——用户关掉开关
// 或清空 effort 后，probe 测的必须是表单终态而非存量。
func TestBuildProbeChatOptions(t *testing.T) {
	high := "high"
	empty := ""
	stored := &agent.AgentConfig{
		ModelID: "stored-model", Effort: "stored-effort",
		ThinkingType: "disabled", ThinkingBudgetTokens: intPtr(4096),
	}

	t.Run("未触碰回落存量（含 disabled 与 budget）", func(t *testing.T) {
		opts := buildProbeChatOptions(ProbeAgentInput{}, stored)
		require.Equal(t, "stored-model", opts.ModelID)
		require.Equal(t, "stored-effort", opts.Effort)
		require.Equal(t, "disabled", opts.ThinkingType)
		require.Equal(t, 4096, *opts.ThinkingBudget)
	})

	t.Run("显式清空 effort 不复活", func(t *testing.T) {
		opts := buildProbeChatOptions(ProbeAgentInput{Effort: &empty}, stored)
		require.Empty(t, opts.Effort)
	})

	t.Run("显式 effort 值优先", func(t *testing.T) {
		opts := buildProbeChatOptions(ProbeAgentInput{Effort: &high}, stored)
		require.Equal(t, "high", opts.Effort)
	})

	t.Run("触碰关开关 provided+'' 不复活存量 disabled（评审场景）", func(t *testing.T) {
		opts := buildProbeChatOptions(ProbeAgentInput{ThinkingProvided: true, ThinkingType: ""}, stored)
		require.Empty(t, opts.ThinkingType)
		require.Nil(t, opts.ThinkingBudget)
	})

	t.Run("请求 modelID 优先于存量", func(t *testing.T) {
		opts := buildProbeChatOptions(ProbeAgentInput{ModelID: "form-model"}, stored)
		require.Equal(t, "form-model", opts.ModelID)
	})
}

// TestBuildProbeChatBody 锁定 probe 请求体契约（issue #261）：互斥与下发
// 同源（disabled 不带 effort）、adaptive 不下发 thinking 字段、预算上限
// 留思考空间、modelID 走 json.Marshal（无注入面）。
func TestBuildProbeChatBody(t *testing.T) {
	decode := func(body string) map[string]any {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &m))
		return m
	}

	t.Run("nil opts = 旧连通性行为", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", false, nil))
		require.Equal(t, "glm-4", m["model"])
		require.Equal(t, float64(1), m["max_tokens"])
		require.NotContains(t, m, "enable_thinking")
		require.NotContains(t, m, "reasoning_effort")
	})

	t.Run("openai disabled 不带 effort（互斥）", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", false, &ProbeChatOptions{
			ThinkingType: "disabled", Effort: "xhigh",
		}))
		require.Equal(t, false, m["enable_thinking"])
		require.NotContains(t, m, "reasoning_effort", "disabled 时 effort 不下发（agent-sdk#170 互斥）")
		require.Equal(t, float64(1), m["max_tokens"])
	})

	t.Run("openai enabled → enable_thinking:true", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", false, &ProbeChatOptions{ThinkingType: "enabled"}))
		require.Equal(t, true, m["enable_thinking"])
	})

	t.Run("openai 仅 effort", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", false, &ProbeChatOptions{Effort: "xhigh"}))
		require.Equal(t, "xhigh", m["reasoning_effort"])
		require.NotContains(t, m, "enable_thinking")
	})

	t.Run("anthropic disabled+effort 互斥", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", true, &ProbeChatOptions{
			ThinkingType: "disabled", Effort: "xhigh",
		}))
		require.Equal(t, map[string]any{"type": "disabled"}, m["thinking"])
		require.NotContains(t, m, "output_config")
		require.Equal(t, float64(1), m["max_tokens"])
	})

	t.Run("anthropic effort 留思考空间", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", true, &ProbeChatOptions{Effort: "xhigh"}))
		require.Equal(t, map[string]any{"effort": "xhigh"}, m["output_config"])
		require.Equal(t, float64(probeThinkingMaxTokens), m["max_tokens"],
			"effort 隐式开思考，max_tokens:1 会 400/截断掩盖结论")
		require.NotContains(t, m, "thinking", "effort 路径不附带 thinking 对象")
	})

	t.Run("anthropic enabled+budget 下限", func(t *testing.T) {
		budget := 4096
		m := decode(mustBody(t, "glm-4", true, &ProbeChatOptions{
			ThinkingType: "enabled", ThinkingBudget: &budget,
		}))
		th, ok := m["thinking"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, "enabled", th["type"])
		require.Equal(t, float64(4096), th["budget_tokens"])
		require.Equal(t, float64(budget+64), m["max_tokens"], "max_tokens 必须大于 budget")
	})

	t.Run("anthropic adaptive 不下发 thinking", func(t *testing.T) {
		m := decode(mustBody(t, "glm-4", true, &ProbeChatOptions{ThinkingType: "adaptive"}))
		require.NotContains(t, m, "thinking")
	})

	t.Run("modelID 特殊字符无注入（json.Marshal）", func(t *testing.T) {
		evil := `evil","x":"y`
		m := decode(mustBody(t, evil, false, nil))
		require.Equal(t, evil, m["model"], "json.Marshal 对 modelID 原样编码，不破坏 body 结构")
	})
}

func mustBody(t *testing.T, modelID string, anthropic bool, opts *ProbeChatOptions) string {
	t.Helper()
	body, err := buildProbeChatBody(modelID, anthropic, opts)
	require.NoError(t, err)
	return body
}

// TestDoProbe_ModelIDOverride 钉死 opts.ModelID 强制 POST ping（#261 与 #262
// 协同点）：目录外自定义模型也走真实 ping，不退化为 GET /models。
func TestDoProbe_ModelIDOverride(t *testing.T) {
	cases := []struct {
		name      string
		protocol  string
		wantPath  string
		wantModel string
		opts      *ProbeChatOptions
	}{
		{"openai 自定义模型强制 POST", string(provider.ProtocolOpenAI), "/chat/completions", "custom-snapshot", &ProbeChatOptions{ModelID: "custom-snapshot"}},
		{"anthropic opts 模型覆盖默认", "anthropic", "/v1/messages", "qwen3.8-flash", &ProbeChatOptions{ModelID: "qwen3.8-flash"}},
		{"openai 无 catalog 无 opts 退化 GET /models", string(provider.ProtocolOpenAI), "/models", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			svc := &ProviderService{}
			res := svc.doProbe(srv.URL, "k", tc.protocol, string(provider.AuthStyleAPIKey), nil, tc.opts)
			require.True(t, res.Success, "probe should succeed, got: %+v", res)
			require.Equal(t, tc.wantPath, gotPath)
			if tc.wantModel != "" {
				var m map[string]any
				require.NoError(t, json.Unmarshal(gotBody, &m))
				require.Equal(t, tc.wantModel, m["model"])
			}
			if tc.opts == nil {
				require.Empty(t, gotBody, "GET 连通性检查不带 body")
			}
		})
	}
}

// TestDoProbe_DisabledEffortMutualExclusion 端到端形态钉死：disabled+effort
// 组合的出站 body 恰为「enable_thinking:false 且无 reasoning_effort」——
// 阿里云 OpenAI 兼容端点互斥约束（agent-sdk#170）在探测侧同规则。
func TestDoProbe_DisabledEffortMutualExclusion(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	svc := &ProviderService{}
	res := svc.doProbe(srv.URL, "k", string(provider.ProtocolOpenAI), string(provider.AuthStyleAPIKey), nil, &ProbeChatOptions{
		ModelID: "qwen3.8-flash", ThinkingType: "disabled", Effort: "xhigh",
	})
	require.True(t, res.Success)
	require.NotContains(t, string(body), "reasoning_effort")
	require.True(t, strings.Contains(string(body), `"enable_thinking":false`))
}
