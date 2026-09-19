// SDK 全方法 happy path + 错误路径测试（httptest 假 server）。
package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeServer 返回一个按路由表响应的假 server；hit 记录收到的请求。
func fakeServer(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) (*ExtensionAdminClient, *[]string) {
	t.Helper()
	hits := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.Method+" "+r.URL.String())
		w.Header().Set("Content-Type", "application/json")
		key := r.Method + " " + r.URL.Path
		if h, ok := routes[key]; ok {
			h(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"success":false,"error":"扩展不存在"}`)
	}))
	t.Cleanup(srv.Close)
	c := NewExtensionAdminClient(srv.URL, "cli_test")
	c.Tenant = "acme"
	return c, hits
}

func TestAllMethodsHappyPath(t *testing.T) {
	ok := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer cli_test", r.Header.Get("Authorization"))
		require.Equal(t, "acme", r.Header.Get("X-Tenant-Id"))
		fmt.Fprint(w, `{"success":true,"data":{"ok":true}}`)
	}
	client, hits := fakeServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v1/admin/extensions":                 ok,
		"GET /api/v1/admin/extensions":                  ok,
		"GET /api/v1/admin/extensions/1":                ok,
		"POST /api/v1/admin/extensions/1/install":       ok,
		"POST /api/v1/admin/extensions/1/enable":        ok,
		"POST /api/v1/admin/extensions/1/disable":       ok,
		"POST /api/v1/admin/extensions/1/upgrade":       ok,
		"POST /api/v1/admin/extensions/1/rollback":      ok,
		"DELETE /api/v1/admin/extensions/1/uninstall":   ok,
		"GET /api/v1/admin/extensions/1/impact":         ok,
		"GET /api/v1/admin/extensions/1/versions/1.0.0": ok,
	})

	manifest := json.RawMessage(`{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.t","version":"1.0.0","displayName":"t","description":"t"}`)
	steps := []func() (json.RawMessage, error){
		func() (json.RawMessage, error) {
			return client.Register(RegisterInput{Manifest: manifest, Source: "upload"})
		},
		func() (json.RawMessage, error) { return client.List("", "", 1, 20) },
		func() (json.RawMessage, error) { return client.Get(1) },
		func() (json.RawMessage, error) { return client.Install(1, "1.0.0") },
		func() (json.RawMessage, error) { return client.Enable(1) },
		func() (json.RawMessage, error) { return client.Disable(1) },
		func() (json.RawMessage, error) { return client.Upgrade(1, "2.0.0") },
		func() (json.RawMessage, error) { return client.Rollback(1, "") },
		func() (json.RawMessage, error) { return client.Uninstall(1, true, false) },
		func() (json.RawMessage, error) { return client.Impact(1) },
		func() (json.RawMessage, error) { return client.GetVersion(1, "1.0.0") },
	}
	for i, step := range steps {
		data, err := step()
		require.NoError(t, err, "step %d", i)
		require.Contains(t, string(data), `"ok":true`)
	}
	require.Len(t, *hits, len(steps))
}

func TestRegisterSendsSignatureEnvelope(t *testing.T) {
	client, _ := fakeServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v1/admin/extensions": func(w http.ResponseWriter, r *http.Request) {
			var env struct {
				Manifest  map[string]any `json:"manifest"`
				Signature string         `json:"signature"`
				PublicKey string         `json:"public_key"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&env))
			require.Equal(t, "sig-b64", env.Signature)
			require.Equal(t, "pub-b64", env.PublicKey)
			require.Equal(t, "io.zerone.t", env.Manifest["name"])
			fmt.Fprint(w, `{"success":true,"data":{}}`)
		},
	})
	_, err := client.Register(RegisterInput{
		Manifest:  json.RawMessage(`{"name":"io.zerone.t"}`),
		Signature: "sig-b64",
		PublicKey: "pub-b64",
	})
	require.NoError(t, err)
}

func TestErrorPaths(t *testing.T) {
	client, _ := fakeServer(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v1/admin/extensions/9": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"success":false,"error":"已安装其他版本 1.0.0，请先升级"}`)
		},
	})
	_, err := client.Get(9)
	require.Error(t, err)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	require.True(t, errors.Is(err, ErrConflict))
	require.Equal(t, 409, apiErr.StatusCode)
	require.Equal(t, "已安装其他版本 1.0.0，请先升级", apiErr.Message) // 中文透传
}

func TestNotFoundDefaultRoute(t *testing.T) {
	client, _ := fakeServer(t, map[string]func(http.ResponseWriter, *http.Request){})
	_, err := client.Disable(404)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	require.True(t, errors.Is(err, ErrNotFound))
	require.Equal(t, "扩展不存在", apiErr.Message)
}
