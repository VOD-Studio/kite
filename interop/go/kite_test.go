package kiteclient

import (
	"context"
	"errors"
	"os"
	"testing"

	"buf.build/gen/go/vod-studio/kite/grpc/go/netease/music/v1/musicv1grpc"
	musicv1 "buf.build/gen/go/vod-studio/kite/protocolbuffers/go/netease/music/v1"
)

// 对真实运行的 kite-server 验证文档里的示例(scripts/go-interop.sh 启动服务并设置环境变量)。
// 只用服务端本地就能判定的场景,不访问上游网易云。
func TestAgainstServer(t *testing.T) {
	addr, token := os.Getenv("KITE_GRPC_ADDR"), os.Getenv("KITE_TOKEN")
	if addr == "" {
		t.Skip("未设置 KITE_GRPC_ADDR(由 scripts/go-interop.sh 启动服务后运行)")
	}
	ctx := context.Background()

	wrong, err := Dial(addr, "wrong-token")
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.Close()
	_, err = musicv1grpc.NewRecommendServiceClient(wrong).GetDailyRecommend(ctx, &musicv1.GetDailyRecommendRequest{})
	if got := Reason(err); got != "SERVER_AUTH_FAILED" {
		t.Fatalf("错误令牌:原因应为 SERVER_AUTH_FAILED,实际 %q(err=%v)", got, err)
	}

	conn, err := Dial(addr, token)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := DailySongs(ctx, conn, ""); !errors.Is(err, ErrReLogin) {
		t.Fatalf("没有网易云 cookie:应返回 ErrReLogin,实际 %v", err)
	}
}

func TestReasonOfNonKiteError(t *testing.T) {
	if got := Reason(errors.New("plain")); got != "" {
		t.Fatalf("非 kite 错误应返回空串,实际 %q", got)
	}
	// 成功路径:调用成功时 err 为 nil,Reason 必须返回空串而不是 panic(DailySongs 先调 Reason 再判 err)。
	if got := Reason(nil); got != "" {
		t.Fatalf("nil 错误应返回空串,实际 %q", got)
	}
}
