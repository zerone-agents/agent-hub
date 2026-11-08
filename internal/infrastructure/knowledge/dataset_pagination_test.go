package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	domain "control-panel/internal/domain/knowledge"
	"github.com/stretchr/testify/require"
)

func TestDisplayNameFilterReadsCompleteRESTPages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "100", r.URL.Query().Get("page_size"))
		require.Empty(t, r.URL.Query().Get("keywords"))
		require.Equal(t, "table", r.URL.Query().Get("parser_id"))
		require.Equal(t, []string{"owner"}, r.URL.Query()["owner_ids"])
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		rows := []map[string]any{}
		for i := (page - 1) * 100; i < min(page*100, 1101); i++ {
			name := "ordinary"
			if i == 101 || i == 1100 {
				name = "needle"
			}
			rows = append(rows, map[string]any{"id": fmt.Sprint(i), "name": fmt.Sprint("physical_", i), "parser_config": map[string]any{"control_panel": map[string]any{"display_name": name}}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "total": 1101, "data": rows})
	}))
	defer server.Close()
	client := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour)
	result, err := client.ListDatasets(context.Background(), domain.DatasetListRequest{Keywords: "needle", Page: 2, PageSize: 1, ParserID: "table", OwnerIDs: []string{"owner"}})
	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
	require.Len(t, result.Datasets, 1)
	require.Equal(t, "1100", result.Datasets[0]["id"])
	require.EqualValues(t, 12, calls.Load())
}

func TestDisplayNameFilterRejectsIncompleteOrUnstableInventory(t *testing.T) {
	for _, mode := range []string{"short", "duplicate", "changed-total", "missing-id", "over-limit", "http-error"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				if mode == "http-error" && page == 2 {
					w.WriteHeader(503)
					return
				}
				total := 150
				if mode == "over-limit" {
					total = maxDatasetFilterRows + 1
				}
				if mode == "changed-total" && page == 2 {
					total = 151
				}
				count := 100
				if page == 2 || mode == "short" {
					count = 50
				}
				rows := []map[string]any{}
				for i := 0; i < count; i++ {
					id := fmt.Sprintf("%d-%d", page, i)
					if mode == "duplicate" && page == 2 {
						id = "1-0"
					}
					if mode == "missing-id" {
						id = ""
					}
					rows = append(rows, map[string]any{"id": id, "name": "physical"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "total": total, "data": rows})
			}))
			defer server.Close()
			result, err := NewRemoteMultiragEngine(server.URL, "key", time.Second, time.Hour).ListDatasets(context.Background(), domain.DatasetListRequest{Keywords: "needle", Page: 1000000, PageSize: 100})
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
