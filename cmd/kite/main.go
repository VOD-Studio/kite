// Package main 是 mimo-music 接口调试与实用 CLI 工具 kite。
//
// 命令实现按领域分包在 internal/cli/{auth,song,album,...},这里只做装配入口。
//
// 用法:
//
//	go run cmd/kite/main.go <command> [flags]
//
// 登录态接口需先登录(扫码或手机号验证码),cookie 持久化到本地配置目录的 session.json
// (macOS ~/Library/Application Support/kite/、Linux ~/.config/kite/、Windows %AppData%\kite\):
//
//	go run cmd/kite/main.go login
//	go run cmd/kite/main.go song like --id 347230 --on
//	go run cmd/kite/main.go logout
package main

import "github.com/VOD-Studio/kite/internal/cli"

func main() {
	cli.Execute()
}
