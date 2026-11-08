package services

import (
	"context"
	"encoding/json"
	"strings"

	"control-panel/internal/domain/knowledge"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DatasetGuard applies to API, MCP and deployment paths, including after
// MEMORY_ENABLED is turned off. A database error closes the general entry.
type DatasetGuard struct{ db *gorm.DB }

func NewDatasetGuard(db *gorm.DB) *DatasetGuard { return &DatasetGuard{db: db} }

func (g *DatasetGuard) Reserve(ctx context.Context, datasetID, tenantID, appID string) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var bindings int64
		if err := tx.Table("agent_knowledge_datasets").Where("dataset_id = ?", datasetID).Count(&bindings).Error; err != nil {
			return err
		}
		if bindings != 0 {
			return knowledge.NewBadRequestError("记忆专用库不能是现有 Agent 挂载的知识库")
		}
		row := knowledge.ReservedDataset{DatasetID: datasetID, TenantID: tenantID, ApplicationID: appID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		var saved knowledge.ReservedDataset
		if err := tx.First(&saved, "dataset_id = ?", datasetID).Error; err != nil {
			return err
		}
		if saved.TenantID != tenantID || saved.ApplicationID != appID {
			return knowledge.NewBadRequestError("记忆专用库已保留给其他应用")
		}
		return nil
	})
}

func (g *DatasetGuard) IDs(ctx context.Context) (map[string]bool, error) {
	ids := []string{}
	if err := g.db.WithContext(ctx).Model(&knowledge.ReservedDataset{}).Pluck("dataset_id", &ids).Error; err != nil {
		return nil, knowledge.NewUnavailableError("知识库隔离状态暂不可用")
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[strings.ToLower(id)] = true
	}
	return out, nil
}

func (g *DatasetGuard) Check(ctx context.Context, ids ...string) error {
	reserved, err := g.IDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if reserved[strings.ToLower(strings.TrimSpace(id))] {
			return knowledge.NewNotFoundError("知识库不存在或不可通过此入口访问")
		}
	}
	return nil
}

func (s *KnowledgeService) SetDatasetGuard(g *DatasetGuard) { s.datasetGuard = g }

func (s *KnowledgeService) guardDatasets(ctx context.Context, ids ...string) error {
	if s.datasetGuard == nil {
		return nil
	}
	return s.datasetGuard.Check(ctx, ids...)
}

// Dataset tagging can read another dataset without going through Retrieval.
// Inspect these references as well as the dataset being modified.
func (s *KnowledgeService) guardDatasetReferences(ctx context.Context, req knowledge.DatasetMutationRequest) error {
	if s.datasetGuard == nil {
		return nil
	}
	b, err := json.Marshal(req)
	if err != nil {
		return knowledge.NewBadRequestError("知识库配置无效")
	}
	var value any
	if json.Unmarshal(b, &value) != nil {
		return knowledge.NewBadRequestError("知识库配置无效")
	}
	var inspect func(any) error
	inspect = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				if key == "tag_kb_ids" {
					ids := []string{}
					switch refs := child.(type) {
					case nil:
					case string:
						ids = strings.Split(refs, ",")
					case []any:
						for _, raw := range refs {
							id, ok := raw.(string)
							if !ok {
								return knowledge.NewBadRequestError("tag_kb_ids 必须包含知识库标识")
							}
							ids = append(ids, id)
						}
					default:
						return knowledge.NewBadRequestError("tag_kb_ids 格式无效")
					}
					if err := s.guardDatasets(ctx, ids...); err != nil {
						return err
					}
				}
				if err := inspect(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range v {
				if err := inspect(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return inspect(value)
}

func (s *KnowledgeService) guardedDatasetList(ctx context.Context, req knowledge.DatasetListRequest) (*knowledge.DatasetListResult, error) {
	reserved, err := s.datasetGuard.IDs(ctx)
	if err != nil {
		return nil, err
	}
	if reserved[strings.ToLower(strings.TrimSpace(req.ID))] {
		return nil, knowledge.NewNotFoundError("知识库不存在或不可通过此入口访问")
	}
	if len(reserved) == 0 {
		return s.engine.ListDatasets(ctx, req)
	}
	// The upstream list cannot express exclusions. Bound the scan and paginate
	// after filtering, so names, counts and page boundaries never expose memory.
	page, size := req.Page, req.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 30
	}
	all := []knowledge.Dataset{}
	for p := 1; p <= 100; p++ {
		r := req
		r.Page, r.PageSize = p, 100
		result, err := s.engine.ListDatasets(ctx, r)
		if err != nil {
			return nil, err
		}
		for _, ds := range result.Datasets {
			id, ok := ds["id"].(string)
			if !ok || id == "" {
				return nil, knowledge.NewUpstreamError("知识库列表缺少标识", nil)
			}
			if !reserved[strings.ToLower(id)] {
				all = append(all, ds)
			}
		}
		if len(result.Datasets) < 100 || p*100 >= result.Total {
			start := (page - 1) * size
			if start > len(all) {
				start = len(all)
			}
			end := start + size
			if end > len(all) {
				end = len(all)
			}
			return &knowledge.DatasetListResult{Total: len(all), Datasets: all[start:end]}, nil
		}
	}
	return nil, knowledge.NewUnavailableError("知识库列表超过隔离扫描上限，请指定知识库查询")
}
