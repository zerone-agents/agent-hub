package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/knowledge"
	repository "control-panel/internal/infrastructure/persistence"
)

const knowledgeUnavailableMessage = "知识库模块未配置：请设置 MULTIRAG_BASE_URL 和 MULTIRAG_API_KEY"

type KnowledgeService struct {
	engine      knowledge.KnowledgeEngine
	providerSvc *ProviderService
	agentRepo   *repository.AgentRepository
}

// NewKnowledgeService wires the MultiRAG knowledge engine and an optional
// ProviderService used to translate local model_ids into MultiRAG-format
// references before forwarding. providerSvc may be nil — in that case
// translation is skipped and refs pass through unchanged. agentRepo 为删除
// 保护反查（issue #122），内部构造对齐 NewToolService 先例。
func NewKnowledgeService(engine knowledge.KnowledgeEngine, providerSvc *ProviderService) *KnowledgeService {
	return &KnowledgeService{engine: engine, providerSvc: providerSvc, agentRepo: repository.NewAgentRepository()}
}

func (s *KnowledgeService) Health(ctx context.Context) (*knowledge.HealthStatus, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return &knowledge.HealthStatus{
			Configured: false,
			Connected:  false,
			Status:     "unavailable",
			Message:    err.Error(),
		}, err
	}
	return engine.Health(ctx)
}

func (s *KnowledgeService) ListDatasets(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	return engine.ListDatasets(ctx, req)
}

func (s *KnowledgeService) CreateDataset(ctx context.Context, tenantID string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	s.translateModelRefs(ctx, tenantID, req)
	return engine.CreateDataset(ctx, req)
}

func (s *KnowledgeService) GetDataset(ctx context.Context, id string) (*knowledge.Dataset, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, knowledge.NewBadRequestError("datasetId 不能为空")
	}
	return engine.GetDataset(ctx, id)
}

func (s *KnowledgeService) UpdateDataset(ctx context.Context, tenantID string, id string, req knowledge.DatasetMutationRequest) (*knowledge.Dataset, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, knowledge.NewBadRequestError("datasetId 不能为空")
	}
	s.translateModelRefs(ctx, tenantID, req)
	return engine.UpdateDataset(ctx, id, req)
}

func (s *KnowledgeService) DeleteDatasets(ctx context.Context, tenantID string, req knowledge.DeleteRequest) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	req.IDs = cleanIDs(req.IDs)
	if err := s.guardDatasetInUse(tenantID, req); err != nil {
		return err
	}
	return engine.DeleteDatasets(ctx, req)
}

// guardDatasetInUse 拒绝删除仍被 Agent 绑定的知识库（issue #122）：反查
// 绑定表，命中即返回 DatasetInUseError（handler 映射 409），不触 multirag。
// 防护跨租户（own ∪ foreign 任一命中即挡，防误删他租户在用的库）；载荷按
// 租户切分（review P1）——Agents 只含请求租户名单，他租户仅以 Foreign
// 中性事实出现、绝不透名。显式 IDs 求交集；delete_all 只要存在任一绑定
// 即挡（含僵尸——恢复路径恰好是前端 ghost 项）。
func (s *KnowledgeService) guardDatasetInUse(tenantID string, req knowledge.DeleteRequest) error {
	own, foreign, err := s.agentRepo.GetDatasetBindingsScoped(tenantID)
	if err != nil {
		return fmt.Errorf("query dataset agent bindings failed: %w", err)
	}
	blocked := func(id string) bool {
		if _, ok := own[id]; ok {
			return true
		}
		_, ok := foreign[id]
		return ok
	}
	var ids []string
	if req.DeleteAll {
		for id := range own {
			ids = append(ids, id)
		}
		for id := range foreign {
			if _, ok := own[id]; !ok {
				ids = append(ids, id)
			}
		}
	} else {
		for _, id := range req.IDs {
			if blocked(id) {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	items := make([]agent.DatasetInUseItem, 0, len(ids))
	for _, id := range ids {
		_, isForeign := foreign[id]
		items = append(items, agent.DatasetInUseItem{ID: id, Agents: own[id], Foreign: isForeign})
	}
	return &agent.DatasetInUseError{Datasets: items}
}

func (s *KnowledgeService) ListDocuments(ctx context.Context, datasetID string, req knowledge.DocumentListRequest) (*knowledge.DocumentListResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	return engine.ListDocuments(ctx, datasetID, req)
}

func (s *KnowledgeService) UploadDocuments(ctx context.Context, datasetID string, upload knowledge.UploadRequest) ([]knowledge.Document, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	if upload.Body == nil {
		return nil, knowledge.NewBadRequestError("上传请求体不能为空")
	}
	return engine.UploadDocuments(ctx, datasetID, upload)
}

func (s *KnowledgeService) DownloadDocument(ctx context.Context, datasetID string, documentID string) (*knowledge.StreamResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return nil, err
	}
	return engine.DownloadDocument(ctx, datasetID, documentID)
}

func (s *KnowledgeService) GetImage(ctx context.Context, datasetID string, imageID string) (*knowledge.StreamResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	if _, err := requireID("datasetId", datasetID); err != nil {
		return nil, err
	}
	imageID, err = requireID("imageId", imageID)
	if err != nil {
		return nil, err
	}
	return engine.GetImage(ctx, imageID)
}

func (s *KnowledgeService) UpdateDocument(ctx context.Context, datasetID string, documentID string, req knowledge.DocumentUpdateRequest) (*knowledge.Document, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return nil, err
	}
	return engine.UpdateDocument(ctx, datasetID, documentID, req)
}

func (s *KnowledgeService) DeleteDocuments(ctx context.Context, datasetID string, req knowledge.DeleteRequest) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return err
	}
	req.IDs = cleanIDs(req.IDs)
	return engine.DeleteDocuments(ctx, datasetID, req)
}

func (s *KnowledgeService) ParseDocuments(ctx context.Context, datasetID string, ids []string) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return err
	}
	ids = cleanIDs(ids)
	if len(ids) == 0 {
		return knowledge.NewBadRequestError("document_ids 不能为空")
	}
	return engine.ParseDocuments(ctx, datasetID, ids)
}

func (s *KnowledgeService) StopParsingDocuments(ctx context.Context, datasetID string, ids []string) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return err
	}
	ids = cleanIDs(ids)
	if len(ids) == 0 {
		return knowledge.NewBadRequestError("document_ids 不能为空")
	}
	return engine.StopParsingDocuments(ctx, datasetID, ids)
}

func (s *KnowledgeService) ListChunks(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkListRequest) (*knowledge.ChunkListResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return nil, err
	}
	return engine.ListChunks(ctx, datasetID, documentID, req)
}

func (s *KnowledgeService) CreateChunk(ctx context.Context, datasetID string, documentID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return nil, err
	}
	return engine.CreateChunk(ctx, datasetID, documentID, req)
}

func (s *KnowledgeService) UpdateChunk(ctx context.Context, datasetID string, documentID string, chunkID string, req knowledge.ChunkMutationRequest) (*knowledge.Chunk, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return nil, err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return nil, err
	}
	chunkID, err = requireID("chunkId", chunkID)
	if err != nil {
		return nil, err
	}
	return engine.UpdateChunk(ctx, datasetID, documentID, chunkID, req)
}

func (s *KnowledgeService) DeleteChunks(ctx context.Context, datasetID string, documentID string, req knowledge.DeleteChunksRequest) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return err
	}
	req.ChunkIDs = cleanIDs(req.ChunkIDs)
	return engine.DeleteChunks(ctx, datasetID, documentID, req)
}

func (s *KnowledgeService) SwitchChunks(ctx context.Context, datasetID string, documentID string, ids []string, available bool) error {
	engine, err := s.requireEngine()
	if err != nil {
		return err
	}
	datasetID, err = requireID("datasetId", datasetID)
	if err != nil {
		return err
	}
	documentID, err = requireID("documentId", documentID)
	if err != nil {
		return err
	}
	ids = cleanIDs(ids)
	if len(ids) == 0 {
		return knowledge.NewBadRequestError("chunk_ids 不能为空")
	}
	return engine.SwitchChunks(ctx, datasetID, documentID, ids, available)
}

func (s *KnowledgeService) Retrieval(ctx context.Context, req knowledge.RetrievalRequest) (*knowledge.RetrievalResult, error) {
	engine, err := s.requireEngine()
	if err != nil {
		return nil, err
	}
	normalized := normalizeRetrievalRequest(req)
	return engine.Retrieval(ctx, normalized)
}

func (s *KnowledgeService) requireEngine() (knowledge.KnowledgeEngine, error) {
	if s == nil || s.engine == nil {
		return nil, knowledge.NewUnavailableError(knowledgeUnavailableMessage)
	}
	return s.engine, nil
}

// translateModelRefs rewrites local model_ids on a DatasetMutationRequest
// to their MultiRAG-format equivalents in place. It is a no-op when the
// service has no ProviderService. Both embd_id (top-level) and
// parser_config.layout_recognize (nested) are translated.
func (s *KnowledgeService) translateModelRefs(ctx context.Context, tenantID string, req knowledge.DatasetMutationRequest) {
	if s == nil || s.providerSvc == nil {
		return
	}
	if embd, ok := req["embd_id"].(string); ok {
		if translated := s.translateLocalModelRef(ctx, tenantID, embd, "embedding"); translated != "" {
			req["embd_id"] = translated
		}
	}
	if cfg, ok := req["parser_config"].(map[string]any); ok {
		if lr, ok := cfg["layout_recognize"].(string); ok {
			if translated := s.translateLocalModelRef(ctx, tenantID, lr, "layout"); translated != "" {
				cfg["layout_recognize"] = translated
			}
		}
	}
}

// translateLocalModelRef checks if the given ref matches a local
// provider_models.model_id. If so, returns the MultiRAG-format reference:
//   - mode "embedding" → "<modelId>@<factory>" (the full MultiRAG id)
//   - mode "layout"    → "<factory>" (MultiRAG's layout_recognize uses
//     the factory name only, e.g. "MinerU", not "mineru@MinerU")
//
// Otherwise returns the original ref unchanged. Empty refs, nil
// ProviderService, providers without a MultiRAG factory mapping, and
// unknown model_ids all pass through unchanged.
func (s *KnowledgeService) translateLocalModelRef(ctx context.Context, tenantID, ref, mode string) string {
	if ref == "" || s == nil || s.providerSvc == nil {
		return ref
	}
	p, err := s.providerSvc.FindProviderByModelID(tenantID, ref)
	if err != nil {
		return ref
	}
	factory := p.MultiRAGFactoryName()
	if factory == "" {
		return ref
	}
	switch mode {
	case "embedding":
		return ref + "@" + factory
	case "layout":
		return factory
	}
	return ref
}

func requireID(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", knowledge.NewBadRequestError(label + " 不能为空")
	}
	return value, nil
}

func cleanIDs(ids []string) []string {
	cleaned := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			cleaned = append(cleaned, id)
		}
	}
	return cleaned
}

func normalizeRetrievalRequest(req knowledge.RetrievalRequest) knowledge.RetrievalRequest {
	normalized := knowledge.CloneObject(map[string]any(req))
	if _, ok := normalized["dataset_ids"]; !ok {
		if kbIDs, exists := normalized["kb_ids"]; exists {
			normalized["dataset_ids"] = kbIDs
		}
	}
	delete(normalized, "kb_ids")

	if _, ok := normalized["document_ids"]; !ok {
		if docIDs, exists := normalized["doc_ids"]; exists {
			normalized["document_ids"] = docIDs
		}
	}
	delete(normalized, "doc_ids")
	return knowledge.RetrievalRequest(normalized)
}
