// zerone 是 Agent Hub H7.6 扩展开发者 CLI。
//
// 用法：
//
//	zerone [--server URL] [--token TOKEN] [--tenant ID] [--output DIR] extension <子命令>
//
// 子命令：create / validate / dev / pack / migrate-manifest。
// 本目录同时存在 Bun/TypeScript 版 zhub CLI（package.json、src/），二者互不干扰：
// Go 工具链只编译 .go 文件，产物二进制名 zerone。
package main

import (
	"fmt"
	"os"
)

const defaultServer = "http://localhost:8080"

// globalFlags 是各子命令共享的全局连接参数。
type globalFlags struct {
	server string
	token  string
	tenant string
	output string
}

func usage() {
	fmt.Fprint(os.Stderr, `zerone — Agent Hub 扩展开发者 CLI（H7.6）

用法：
  zerone [全局参数] extension <子命令> [参数]

全局参数（须写在 extension 子命令之前）：
  --server  Agent Hub 服务地址（默认 `+defaultServer+`，可用 ZERONE_SERVER 覆盖）
  --token   管理员 Bearer token（默认读取 ZERONE_TOKEN 环境变量）
  --tenant  租户 ID（默认 default；经 X-Tenant-Id 头透传）
  --output  产物输出目录（pack/create 使用，默认当前目录）

子命令：
  create <name> [--dir DIR]           生成扩展目录骨架（extension.yaml + README + 示例）
  validate <dir|file>                 本地严格校验 manifest，输出中文错误
  dev <dir>                           开发模式：监听目录变化，防抖 1s 重新校验并 POST 到注册端点
  pack <dir> [--sign --key 私钥文件]  打包 tar.gz，计算 sha256，可选 ed25519 签名
  migrate-manifest <old.yaml> [--out 新文件]  把 v1alpha1 旧宽松 manifest 升级为严格 v1 格式

示例：
  zerone extension create io.zerone.hello --dir ./my-extensions
  zerone extension validate ./my-extensions/io.zerone.hello
  zerone --server http://localhost:8080 --token cli_xxx extension dev ./my-extensions/io.zerone.hello
  zerone extension pack ./my-extensions/io.zerone.hello --sign --key ./ed25519.key
`)
}

func main() {
	g := globalFlags{server: os.Getenv("ZERONE_SERVER")}
	if g.server == "" {
		g.server = defaultServer
	}
	g.token = os.Getenv("ZERONE_TOKEN")
	var args []string
	// 手工解析：全局 flag 只能出现在 extension 子命令之前（若在子命令后
	// 继续吞 flag，会误伤 pack/create 自己的 --output/--dir/--key 参数）
	seenCmd := false
	for i := 1; i < len(os.Args); i++ {
		a := os.Args[i]
		if seenCmd || a == "" || a[0] != '-' {
			seenCmd = true
			args = append(args, a)
			continue
		}
		switch {
		case a == "--server" && i+1 < len(os.Args):
			i++
			g.server = os.Args[i]
		case a == "--token" && i+1 < len(os.Args):
			i++
			g.token = os.Args[i]
		case a == "--tenant" && i+1 < len(os.Args):
			i++
			g.tenant = os.Args[i]
		case a == "--output" && i+1 < len(os.Args):
			i++
			g.output = os.Args[i]
		case a == "-h" || a == "--help":
			usage()
			return
		default:
			args = append(args, a)
		}
	}
	if len(args) < 2 || args[0] != "extension" {
		usage()
		os.Exit(2)
	}
	cmd, rest := args[1], args[2:]
	var err error
	switch cmd {
	case "create":
		err = cmdCreate(g, rest)
	case "validate":
		err = cmdValidate(rest)
	case "dev":
		err = cmdDev(g, rest)
	case "pack":
		err = cmdPack(g, rest)
	case "migrate-manifest":
		err = cmdMigrateManifest(rest)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令 %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		os.Exit(1)
	}
}
