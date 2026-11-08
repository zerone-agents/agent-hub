package multirag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"control-panel/internal/domain/provider"
	"github.com/stretchr/testify/require"
)

func instancePage(w http.ResponseWriter, r *http.Request, items any, total int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": items, "total": total, "page": page, "page_size": 100}})
}

func TestInstanceTransportUsesAtomicCustomDeclarationsAndExactReferences(t *testing.T) {
	input := provider.InstanceInput{InstanceName: "hub-owned", InstanceCandidate: provider.InstanceCandidate{APIKey: "private-fixture", BaseURL: "https://provider.invalid/v1", ModelIDs: []string{}, CustomModels: []provider.ModelRegistration{{ModelName: "vision", ModelTypes: []string{"chat", "image2text"}, MaxTokens: 0, IsTools: false}}}}
	row := provider.RemoteInstance{ID: "bound-id", ProviderName: "OpenAI-API-Compatible", Name: "hub-owned", Status: "active", BaseURL: input.BaseURL, HasAPIKey: true}
	model := provider.RemoteModel{ID: json.Number("9007199254740993"), Reference: "9007199254740993", ProviderName: row.ProviderName, Name: "vision", InstanceName: row.Name, InstanceID: row.ID, Status: "active", EffectiveStatus: "active", ModelTypes: []string{"chat", "image2text"}}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer service-key", r.Header.Get("Authorization"))
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/providers":
			instancePage(w, r, []map[string]string{{"name": row.ProviderName}}, 1)
		case "GET /api/v1/providers/OpenAI-API-Compatible/instances":
			instancePage(w, r, []provider.RemoteInstance{row}, 1)
		case "POST /api/v1/providers/OpenAI-API-Compatible/instances":
			var actual provider.InstanceInput
			require.NoError(t, json.NewDecoder(r.Body).Decode(&actual))
			require.Equal(t, input, actual)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": row})
		case "POST /api/v1/providers/OpenAI-API-Compatible/connection":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"verified": true, "model_name": "vision", "model_type": "chat"}})
		case "GET /api/v1/providers/OpenAI-API-Compatible/instances/hub-owned/models":
			instancePage(w, r, []provider.RemoteModel{model}, 1)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "service-key")
	instances, err := c.ListInstances(context.Background(), row.ProviderName)
	require.NoError(t, err)
	require.Len(t, instances, 1)
	created, err := c.CreateInstance(context.Background(), row.ProviderName, input)
	require.NoError(t, err)
	require.Equal(t, row, *created)
	require.NoError(t, c.VerifyInstance(context.Background(), row.ProviderName, input.InstanceCandidate))
	models, err := c.ListInstanceModels(context.Background(), row.ProviderName, row.Name)
	require.NoError(t, err)
	require.Equal(t, "9007199254740993", models[0].Reference)
	require.Equal(t, 5, calls)
}

func TestInstanceTransportOnlySelectsLegacyOnReadRouteAbsence(t *testing.T) {
	for _, status := range []int{404, 405, 401, 403, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodGet, r.Method)
				w.WriteHeader(status)
			}))
			defer srv.Close()
			_, err := NewClient(srv.URL, "key").ListInstances(context.Background(), "OpenAI-API-Compatible")
			if status == 404 || status == 405 {
				require.ErrorIs(t, err, provider.ErrInstanceAPIUnavailable)
			} else {
				require.Error(t, err)
				require.NotErrorIs(t, err, provider.ErrInstanceAPIUnavailable)
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestInstanceTransportRequiresTypedVerificationAndMutationAcknowledgement(t *testing.T) {
	for _, body := range []string{`{"code":0,"data":false}`, `{"code":0,"data":{"verified":false}}`, `{"code":0,"data":{"verified":true,"model_name":"other","model_type":"chat"}}`, `{"code":0,"retcode":102,"data":true}`, `{"data":true}`, `{"code":0,"data":null}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			c := NewClient(srv.URL, "key")
			err := c.VerifyInstance(context.Background(), "OpenAI-API-Compatible", provider.InstanceCandidate{CustomModels: []provider.ModelRegistration{{ModelName: "one", ModelTypes: []string{"chat"}}}})
			require.Error(t, err)
			require.Equal(t, 1, calls)
			require.NotContains(t, err.Error(), body)
		})
	}
}

func TestInstanceInventoryRejectsPartialAndConflictingIdentity(t *testing.T) {
	for _, data := range []string{
		`{"items":[],"total":1,"page":1,"page_size":100}`,
		`{"items":null,"total":0,"page":1,"page_size":100}`,
		`{"items":[],"total":0,"page":2,"page_size":100}`,
		`{"items":[{"id":9007199254740993,"reference":"9007199254740992","name":"one","provider_name":"P","instance_name":"I","instance_id":"id","model_types":["embedding"],"effective_status":"active"}],"total":1,"page":1,"page_size":100}`,
	} {
		t.Run(data, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"code":0,"data":` + data + `}`)) }))
			defer srv.Close()
			rows, err := NewClient(srv.URL, "key").ListInstanceModels(context.Background(), "P", "I")
			require.Error(t, err)
			require.Nil(t, rows)
		})
	}
}

func TestInstanceMutationNeverFollowsRedirectOrReplays(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, "/other", 307) }))
	defer srv.Close()
	_, err := NewClient(srv.URL, "key").CreateInstance(context.Background(), "P", provider.InstanceInput{})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}
