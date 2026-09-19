// zerone extension dev —— 本地开发模式。
//
// 轮询扩展目录 mtime（400ms 间隔，零依赖 watch），内容变化后防抖 1s：
// 先本地严格校验，通过则把 manifest 以签名信封形态 POST 到
// {--server}/api/v1/admin/extensions（Authorization: Bearer <token>，
// X-Tenant-Id 透传 --tenant）；失败打印中文错误，不重试风暴。
// Ctrl-C 退出。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"control-panel/internal/extensionmanifest"
)

// cmdDev 实现 zerone extension dev <dir>。
func cmdDev(g globalFlags, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：zerone extension dev <dir>")
	}
	if g.token == "" {
		return fmt.Errorf("开发模式需要管理员 token：--token 或 ZERONE_TOKEN")
	}
	root, err := extensionRootDir(args[0])
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}

	push := func() {
		m, err := loadManifestYAML(filepath.Join(root, "extension.yaml"))
		if err != nil {
			fmt.Printf("[dev] 校验失败：%v\n", err)
			return
		}
		canonical, err := manifestToCanonicalJSON(m)
		if err != nil {
			fmt.Printf("[dev] 校验失败：%v\n", err)
			return
		}
		manifest, errs := extensionmanifest.ValidateExtensionManifest(canonical)
		if len(errs) > 0 {
			fmt.Printf("[dev] 校验失败：%s\n", joinErrs(errs))
			return
		}
		body, _ := json.Marshal(map[string]json.RawMessage{
			"manifest": json.RawMessage(canonical),
		})
		req, err := http.NewRequest(http.MethodPost, g.server+"/api/v1/admin/extensions", bytes.NewReader(body))
		if err != nil {
			fmt.Printf("[dev] 请求构造失败：%v\n", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+g.token)
		if g.tenant != "" {
			req.Header.Set("X-Tenant-Id", g.tenant)
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("[dev] 推送失败（网络错误，不重试）：%v\n", err)
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
			fmt.Printf("[dev] %s@%s 已推送到 %s（HTTP %d）\n", manifest.Name, manifest.Version, g.server, resp.StatusCode)
			return
		}
		var env struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &env)
		if env.Error == "" {
			env.Error = string(respBody)
		}
		fmt.Printf("[dev] 推送被拒（HTTP %d）：%s\n", resp.StatusCode, env.Error)
	}

	// 启动即推一次
	push()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	var lastChange time.Time
	var pending bool
	last := dirFingerprint(root)
	fmt.Printf("[dev] 正在监听 %s（Ctrl-C 退出）\n", root)
	for {
		select {
		case <-sig:
			fmt.Println("[dev] 已退出")
			return nil
		case <-ticker.C:
			fp := dirFingerprint(root)
			if fp != last {
				last = fp
				lastChange = time.Now()
				pending = true
				continue
			}
			// 防抖 1s：变化停止后 1s 才推送
			if pending && time.Since(lastChange) >= time.Second {
				pending = false
				push()
			}
		}
	}
}

// dirFingerprint 计算目录下全部文件的（路径+mtime+size）拼接串，
// 作为内容变化的廉价判定。
func dirFingerprint(root string) string {
	type entry struct {
		path string
		mt   time.Time
		sz   int64
	}
	var entries []entry
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		entries = append(entries, entry{path, info.ModTime(), info.Size()})
		return nil
	})
	// 排序保证稳定
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].path < entries[j-1].path; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	out := ""
	for _, e := range entries {
		out += fmt.Sprintf("%s:%d:%d;", e.path, e.mt.UnixNano(), e.sz)
	}
	return out
}
