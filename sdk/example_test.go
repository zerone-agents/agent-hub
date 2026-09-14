package sdk_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	"control-panel/sdk"
)

// 示例 1：注册一个扩展并安装启用（happy path）。
func ExampleExtensionAdminClient_Register() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/extensions":
			fmt.Fprint(w, `{"success":true,"data":{"extension":{"name":"io.zerone.hello"},"version":{"version":"0.1.0"}}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/extensions/1/install":
			fmt.Fprint(w, `{"success":true,"data":{"install":{"status":"enabled"},"idempotent":false}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/extensions/1/enable":
			fmt.Fprint(w, `{"success":true,"data":{"status":"enabled"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := sdk.NewExtensionAdminClient(server.URL, "cli_xxx")
	manifest := json.RawMessage(`{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.hello","version":"0.1.0","displayName":"你好","description":"示例"}`)

	if _, err := client.Register(sdk.RegisterInput{Manifest: manifest, Source: "upload"}); err != nil {
		fmt.Println("注册失败：", err)
		return
	}
	if _, err := client.Install(1, "0.1.0"); err != nil {
		fmt.Println("安装失败：", err)
		return
	}
	if _, err := client.Enable(1); err != nil {
		fmt.Println("启用失败：", err)
		return
	}
	fmt.Println("注册→安装→启用 全部成功")

	// Output:
	// 注册→安装→启用 全部成功
}

// 示例 2：带 ed25519 签名的注册（发布者流程）。
func ExampleExtensionAdminClient_Register_signed() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var env struct {
			Signature string `json:"signature"`
			PublicKey string `json:"public_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&env)
		if env.Signature == "" || env.PublicKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"success":false,"error":"签名校验失败：缺少签名或公钥"}`)
			return
		}
		fmt.Fprint(w, `{"success":true,"data":{"version":{"signedBy":"aabbccddeeff00112233445566778899"}}}`)
	}))
	defer server.Close()

	client := sdk.NewExtensionAdminClient(server.URL, "cli_xxx")
	_, err := client.Register(sdk.RegisterInput{
		Manifest:  json.RawMessage(`{"apiVersion":"agenthub.extension/v1alpha1","name":"io.zerone.hello","version":"0.1.0","displayName":"你好","description":"示例"}`),
		Signature: "ZmFrZS1zaWduYXR1cmU=",
		PublicKey: "ZmFrZS1wdWJrZXk=",
	})
	fmt.Println(err == nil)

	// Output:
	// true
}

// 示例 3：错误分支——404 映射为 ErrNotFound，中文错误透传。
func ExampleExtensionAdminClient_Get() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"success":false,"error":"扩展不存在"}`)
	}))
	defer server.Close()

	client := sdk.NewExtensionAdminClient(server.URL, "cli_xxx")
	_, err := client.Get(42)
	var apiErr *sdk.APIError
	if errors.As(err, &apiErr) && errors.Is(err, sdk.ErrNotFound) {
		fmt.Println("未找到：", apiErr.Message)
	}

	// Output:
	// 未找到： 扩展不存在
}

// 示例 4：生命周期全流程——影响面检查→升级→必要时回滚→卸载。
func ExampleExtensionAdminClient_Impact() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/admin/extensions/7/impact":
			fmt.Fprint(w, `{"success":true,"data":{"dependents":[],"dependencies":[]}}`)
		case "/api/v1/admin/extensions/7/upgrade":
			fmt.Fprint(w, `{"success":true,"data":{"migrated":true}}`)
		case "/api/v1/admin/extensions/7/uninstall":
			fmt.Fprint(w, `{"success":true,"data":{"purged":false}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := sdk.NewExtensionAdminClient(server.URL, os.Getenv("ZERONE_TOKEN"))
	if _, err := client.Impact(7); err != nil {
		fmt.Println("影响面查询失败：", err)
		return
	}
	if _, err := client.Upgrade(7, "2.0.0"); err != nil {
		fmt.Println("升级失败：", err)
		return
	}
	if _, err := client.Uninstall(7, false, false); err != nil {
		fmt.Println("卸载失败：", err)
		return
	}
	fmt.Println("影响面→升级→卸载 全部成功")

	// Output:
	// 影响面→升级→卸载 全部成功
}
