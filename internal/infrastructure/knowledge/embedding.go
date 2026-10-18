package knowledge

import (
	"bytes"
	"context"
	domain "control-panel/internal/domain/knowledge"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
)

func (c *RemoteMultiragEngine) CheckEmbedding(ctx context.Context, datasetID string, check domain.EmbeddingCheckRequest) (*domain.EmbeddingCheckResult, error) {
	count := 5
	if check.CheckNum != nil {
		count = *check.CheckNum
	}
	if strings.TrimSpace(check.EmbeddingID) == "" || count < 1 || count > 100 {
		return nil, domain.NewBadRequestError("embd_id 不能为空，check_num 必须为 1 到 100 的整数")
	}
	body, _ := json.Marshal(map[string]any{"embd_id": check.EmbeddingID, "check_num": count})
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/datasets/"+url.PathEscape(datasetID)+"/embedding/check", nil, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, domain.NewUpstreamError("嵌入模型兼容检查未完成，请重试", nil)
	}
	defer resp.Body.Close()
	var env envelope
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&env) != nil {
		return nil, domain.NewUpstreamError("嵌入模型兼容检查响应异常", nil)
	}
	code, ok := env.statusCode()
	if !ok || (env.Code != nil && env.RetCode != nil && *env.Code != *env.RetCode) || (code != 0 && code != 10) {
		return nil, domain.NewUpstreamError("嵌入模型兼容检查失败，请核对模型配置及知识库权限", nil)
	}
	var result domain.EmbeddingCheckResult
	if json.Unmarshal(env.Data, &result) != nil || !validEmbeddingEvidence(result, datasetID, check.EmbeddingID, count) {
		return nil, domain.NewUpstreamError("嵌入模型兼容检查缺少有效样本证据", nil)
	}
	average := result.Summary["avg_cos_sim"].(float64)
	// Scores are rounded upstream. A rounded 0.9 can be either outcome.
	if (code == 0 && average < .9) || (code == 10 && average > .9) {
		return nil, domain.NewUpstreamError("嵌入模型兼容检查结果与样本证据不一致", nil)
	}
	result.Code, result.Compatible = code, code == 0
	return &result, nil
}

func validEmbeddingEvidence(result domain.EmbeddingCheckResult, datasetID, model string, count int) bool {
	s := result.Summary
	if s["kb_id"] != datasetID || s["model"] != model || result.Results == nil {
		return false
	}
	sampled, a := s["sampled"].(float64)
	valid, b := s["valid"].(float64)
	if !a || !b || sampled < 1 || sampled > float64(count) || math.Trunc(sampled) != sampled || valid < 1 || valid > sampled || math.Trunc(valid) != valid || len(result.Results) != int(sampled) {
		return false
	}
	for _, key := range []string{"avg_cos_sim", "min_cos_sim", "max_cos_sim"} {
		value, ok := s[key].(float64)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
			return false
		}
	}
	if s["min_cos_sim"].(float64) > s["avg_cos_sim"].(float64) || s["avg_cos_sim"].(float64) > s["max_cos_sim"].(float64) {
		return false
	}
	mode, _ := s["match_mode"].(string)
	return mode == "content_only" || mode == "title+content" || mode == "mixed"
}

// Inspect the persisted model/count, never a browser's chunk count. The same
// resolved model is checked and sent to PUT. No write follows an unknown result.
func (c *RemoteMultiragEngine) guardEmbeddingSwitch(ctx context.Context, id string, body map[string]any) error {
	raw, changed := body["embedding_model"]
	if !changed {
		return nil
	}
	model, ok := raw.(string)
	if !ok || strings.TrimSpace(model) == "" {
		return domain.NewBadRequestError("embedding_model 必须是非空模型引用")
	}
	before, err := c.GetDataset(ctx, id)
	if err != nil {
		return err
	}
	if (*before)["id"] != id {
		return domain.NewUpstreamError("知识库模型回读归属不一致", nil)
	}
	if model == (*before)["embd_id"] {
		return nil
	}
	count, ok := (*before)["chunk_num"].(float64)
	if !ok || count < 0 || math.IsInf(count, 0) || math.IsNaN(count) || math.Trunc(count) != count {
		return domain.NewUpstreamError("无法确认已有切片数量，请刷新后再更换嵌入模型", nil)
	}
	if count == 0 {
		return nil
	}
	result, err := c.CheckEmbedding(ctx, id, domain.EmbeddingCheckRequest{EmbeddingID: model})
	if err != nil {
		return err
	}
	if !result.Compatible {
		return &domain.EmbeddingIncompatibleError{Result: result}
	}
	return nil
}
