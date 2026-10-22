// Command kite-gen-service 重新生成 internal/service 的一行转发胶水。
//
// 由 proto 编译进 gen/ 的 descriptor 生成 <service>_gen.go(见 internal/servicegen)。
// 使用:make service-gen(在仓库根运行)。新鲜度由 internal/service 的测试守护,
// 与 gen/、docs/cmd/ 同一策略:生成物提交进版本控制,漂移即红。
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/VOD-Studio/kite/internal/servicegen"
)

func main() {
	files, err := servicegen.Generate()
	if err != nil {
		log.Fatalf("生成失败:%v\n新增 rpc 需在 internal/servicegen 的 spec 里登记转发目标或加入 Custom。", err)
	}
	const dir = "internal/service"
	for _, f := range files {
		path := filepath.Join(dir, f.Name)
		//nolint:gosec // 生成的 Go 源码文件,仓库里与其它源码同为 0644
		if err := os.WriteFile(path, f.Content, 0o644); err != nil {
			log.Fatalf("写入 %s 失败:%v", path, err)
		}
		fmt.Printf("已生成 %s\n", path)
	}
}
