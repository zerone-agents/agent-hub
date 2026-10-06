package knowledge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocumentIngestStrictContract(t *testing.T) {
	for _, run := range []string{`0`, `1`, `2`, `"0"`, `"1"`, `"2"`} {
		var req DocumentIngestRequest
		require.NoError(t, json.Unmarshal([]byte(`{"doc_ids":["d"],"run":`+run+`}`), &req))
	}
	for _, payload := range []string{
		`null`, `{}`, `{"doc_ids":[],"run":1}`, `{"doc_ids":null,"run":1}`,
		`{"doc_ids":[null],"run":1}`, `{"doc_ids":[1],"run":1}`, `{"doc_ids":[" "],"run":1}`,
		`{"doc_ids":["d"],"run":true}`, `{"doc_ids":["d"],"run":1.0}`, `{"doc_ids":["d"],"run":" 1"}`,
		`{"doc_ids":["d"],"run":3}`, `{"doc_ids":["d"],"run":0,"apply_kb":true}`,
		`{"doc_ids":["d"],"run":2,"apply_kb":true}`, `{"doc_ids":["d"],"run":1,"delete":"true"}`,
		`{"doc_ids":["d"],"run":1,"apply_kb":null}`, `{"doc_ids":["d"],"run":1,"dataset_id":"kb"}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var req DocumentIngestRequest
			require.Error(t, json.Unmarshal([]byte(payload), &req))
		})
	}
}
