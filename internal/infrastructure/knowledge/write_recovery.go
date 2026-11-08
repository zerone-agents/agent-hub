package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	domain "control-panel/internal/domain/knowledge"
)

func recoveryFailure(err error, businessCode int) *knowledgeUpstreamFailure {
	var failure *knowledgeUpstreamFailure
	if errors.As(err, &failure) && failure.httpStatus == http.StatusOK && failure.businessCode == businessCode {
		return failure
	}
	return nil
}

func partialUploadFailure(err error, datasetID string) *domain.UploadPartialError {
	failure := recoveryFailure(err, 500) // RetCode.SERVER_ERROR in partial document uploads.
	if failure == nil {
		return nil
	}
	var data struct {
		Uploaded []map[string]any `json:"uploaded"`
	}
	if json.Unmarshal(failure.data, &data) != nil || len(data.Uploaded) == 0 || len(data.Uploaded) > 1000 {
		return nil
	}
	seen := map[string]bool{}
	confirmed := make([]domain.Document, 0, len(data.Uploaded))
	for _, row := range data.Uploaded {
		id, idOK := row["id"].(string)
		name, nameOK := row["name"].(string)
		if !idOK || id == "" || strings.TrimSpace(id) != id || !nameOK || name == "" || row["dataset_id"] != datasetID || seen[id] {
			return nil
		}
		seen[id] = true
		confirmed = append(confirmed, domain.Document{"id": id, "name": name, "dataset_id": datasetID, "run": "0"})
	}
	return &domain.UploadPartialError{Uploaded: confirmed}
}

func (c *RemoteMultiragEngine) confirmPartialUpload(ctx context.Context, datasetID string, partial *domain.UploadPartialError) bool {
	for start := 0; start < len(partial.Uploaded); start += 100 {
		ids := []string{}
		wanted := map[string]bool{}
		for _, row := range partial.Uploaded[start:min(start+100, len(partial.Uploaded))] {
			id := row["id"].(string)
			ids = append(ids, id)
			wanted[id] = true
		}
		result, err := c.ListDocuments(ctx, datasetID, domain.DocumentListRequest{IDs: ids, Page: 1, PageSize: 100})
		if err != nil || result == nil || result.Total != len(ids) || len(result.Documents) != len(ids) {
			return false
		}
		for _, row := range result.Documents {
			id, ok := row["id"].(string)
			if !ok || !wanted[id] || row["dataset_id"] != datasetID {
				return false
			}
			delete(wanted, id)
		}
	}
	return true
}

var createdChunkImageKey = regexp.MustCompile(`^[a-f0-9]{16}$`)

func chunkImageFailure(err error, datasetID, chunkID string) *domain.ChunkImageWriteError {
	failure := recoveryFailure(err, 100) // RetCode.EXCEPTION_ERROR in image writes.
	if failure == nil {
		return nil
	}
	var data struct {
		domain.ChunkImageRecovery
		RetrySafe *bool `json:"retry_safe"`
	}
	if json.Unmarshal(failure.data, &data) != nil || data.RetrySafe == nil || *data.RetrySafe {
		return nil
	}
	member := func(value string, choices ...string) bool {
		for _, choice := range choices {
			if value == choice {
				return true
			}
		}
		return false
	}
	if !member(data.Outcome, "unchanged", "unknown", "partial") || !member(data.Stage, "image_read", "index_write", "image_write") || !member(data.Index, "not_attempted", "unknown", "acknowledged") || !member(data.Image, "not_attempted", "unchanged", "changed", "unknown", "acknowledged") {
		return nil
	}
	if chunkID != "" {
		if data.ImageID != datasetID+"-"+chunkID {
			return nil
		}
	} else if key, found := strings.CutPrefix(data.ImageID, datasetID+"-"); !found || !createdChunkImageKey.MatchString(key) {
		return nil
	}
	return &domain.ChunkImageWriteError{Recovery: data.ChunkImageRecovery}
}
