package knowledge

import (
	"context"
	"strings"

	domain "control-panel/internal/domain/knowledge"
)

const maxDatasetFilterRows = 20000

// Display names live in parser_config, so remote name filters cannot be used.
// A capped first page cannot establish the complete local result or its total.
func (c *RemoteMultiragEngine) listDatasetsByDisplayName(ctx context.Context, req domain.DatasetListRequest) (*domain.DatasetListResult, error) {
	fail := func() (*domain.DatasetListResult, error) {
		return nil, domain.NewUpstreamError("知识库目录未完整读取或已变化，请刷新后重试", nil)
	}
	scan := req
	scan.Name, scan.Keywords, scan.PageSize = "", "", 100
	all := []domain.Dataset{}
	seen := map[string]bool{}
	total := -1
	for scan.Page = 1; scan.Page <= maxDatasetFilterRows/100; scan.Page++ {
		page, err := c.ListDatasets(ctx, scan)
		if err != nil {
			return nil, err
		}
		if page == nil || page.Total < 0 || page.Total > maxDatasetFilterRows || len(page.Datasets) > 100 || (total >= 0 && page.Total != total) {
			return fail()
		}
		total = page.Total
		for _, row := range page.Datasets {
			id, ok := row["id"].(string)
			if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || seen[id] {
				return fail()
			}
			seen[id] = true
			all = append(all, row)
		}
		if len(all) > total {
			return fail()
		}
		if len(all) == total {
			filtered := filterDatasetsByDisplayName(all, req.Name, req.Keywords)
			return &domain.DatasetListResult{Total: len(filtered), Datasets: paginateDatasets(filtered, req.Page, min(req.PageSize, 100))}, nil
		}
		if len(page.Datasets) != 100 {
			return fail()
		}
	}
	return fail()
}
