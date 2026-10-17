package server

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// kiteServerEnv 提取 Dockerfile 里设置的 KITE_SERVER_* 环境变量(含续行)。
func kiteServerEnv(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	text := strings.ReplaceAll(string(b), "\\\n", " ")

	got := map[string]string{}
	for _, m := range regexp.MustCompile(`(KITE_SERVER_\w+)=(\S*)`).FindAllStringSubmatch(text, -1) {
		got[m[1]] = m[2]
	}
	return got
}

// 两个 Dockerfile(本地构建 / 发布镜像)必须设置相同的监听地址:容器内要监听全部网卡端口映射才通。
// 发布镜像没有冒烟测试覆盖,靠这条守护防止两处漂移。
func TestDockerfiles_ServerEnvInSync(t *testing.T) {
	dev := kiteServerEnv(t, "../../Dockerfile")
	release := kiteServerEnv(t, "../../Dockerfile.release")

	require.Equal(t, "0.0.0.0:3722", dev["KITE_SERVER_GRPC_ADDR"])
	require.Equal(t, "0.0.0.0:3721", dev["KITE_SERVER_HTTP_ADDR"])
	require.Equal(t, dev, release, "Dockerfile 与 Dockerfile.release 的 KITE_SERVER_* 必须一致")
	// 镜像里不能烤进令牌。
	require.NotContains(t, dev, "KITE_SERVER_AUTH_TOKEN")
}
