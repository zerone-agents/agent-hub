package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelProjectionExpandsCapabilitiesAndPreservesNumericIdentity(t *testing.T) {
	raw := json.RawMessage(`{"Provider":{"llm":[{"id":9007199254740993,"reference":"9007199254740993","name":"same@one","instance_name":"one","type":"chat","model_types":["chat","image2text","rerank"],"status":"1","is_tools":false},{"id":9007199254740994,"reference":"9007199254740994","name":"same@two","instance_name":"two","type":"chat","model_types":["chat","image2text"],"status":"0"}]}}`)
	rows, err := projectMultiRAGModels(raw, "image2text")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "image2text", rows[0].Type)
	require.Equal(t, "9007199254740993", rows[0].FullID)
	require.Equal(t, rows[0].FullID, rows[0].Reference)
	require.Equal(t, "one", rows[0].InstanceName)
	require.NotNil(t, rows[0].IsTools)
	require.False(t, *rows[0].IsTools)
	h := &KnowledgeMcpHandler{modelsSource: &stubMultiRAGMyLLMs{data: raw}}
	models, err := h.availableMcpRerankModels(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"9007199254740993"}, models)
}

func TestModelProjectionRejectsConflictingOrMalformedReferences(t *testing.T) {
	for _, model := range []string{
		`{"id":9007199254740993,"reference":"9007199254740992","name":"one","type":"chat","model_types":["chat"]}`,
		`{"id":1,"reference":"01","name":"one","type":"chat","model_types":["chat"]}`,
		`{"id":1,"reference":1,"name":"one","type":"chat","model_types":["chat"]}`,
		`{"id":1,"reference":"1","name":"one","type":"chat","model_types":[]}`,
		`{"id":1,"reference":null,"name":"one","type":"chat"}`,
		`{"id":1,"reference":"1","name":"one","type":"chat","model_types":null}`,
		`{"id":1,"reference":"1","name":"one","type":"chat","model_types":["rerank","rerank"]}`,
		`{"id":1,"reference":"1","name":"one","type":"chat","model_types":["invented"]}`,
	} {
		t.Run(model, func(t *testing.T) {
			raw := json.RawMessage(`{"P":{"llm":[` + model + `]}}`)
			rows, err := projectMultiRAGModels(raw, "rerank")
			require.Error(t, err)
			require.Nil(t, rows)
		})
	}
}
