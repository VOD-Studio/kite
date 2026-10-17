//! 对真实运行的 kite-server 的互操作测试。
//!
//! 需要环境变量 KITE_GRPC_ADDR(host:port)与 KITE_TOKEN,由 scripts/rust-interop.sh 启动服务并设置;
//! 因此标 #[ignore],脚本以 `--include-ignored` 运行。不访问上游网易云:
//! GetDailyRecommend 需要登录态,请求里没有 cookie 时服务端在本地就拒绝,不会发出任何外网请求。

use kite_interop::auth::auth_interceptor;
use kite_interop::classify::{Action, classify};
use kite_interop::pb::netease::music::v1::GetDailyRecommendRequest;
use kite_interop::pb::netease::music::v1::recommend_service_client::RecommendServiceClient;
use tonic::transport::Channel;
use tonic::{Code, Status};
use tonic_health::pb::HealthCheckRequest;
use tonic_health::pb::health_check_response::ServingStatus;
use tonic_health::pb::health_client::HealthClient;
use tonic_types::StatusExt;

fn env(key: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| panic!("缺少环境变量 {key},请通过 scripts/rust-interop.sh 运行"))
}

async fn channel() -> Channel {
    Channel::from_shared(format!("http://{}", env("KITE_GRPC_ADDR")))
        .expect("地址格式")
        .connect()
        .await
        .expect("连接 kite-server")
}

fn reason_of(status: &Status) -> (String, String) {
    let details = status.get_error_details();
    let info = details.error_info().expect("错误必须携带 google.rpc.ErrorInfo");
    (info.domain.clone(), info.reason.clone())
}

#[tokio::test]
#[ignore = "需要运行中的 kite-server,见 scripts/rust-interop.sh"]
async fn health_check_needs_no_token() {
    let resp = HealthClient::new(channel().await)
        .check(HealthCheckRequest { service: String::new() })
        .await
        .expect("健康检查应免鉴权")
        .into_inner();
    assert_eq!(resp.status(), ServingStatus::Serving);
}

#[tokio::test]
#[ignore = "需要运行中的 kite-server,见 scripts/rust-interop.sh"]
async fn missing_token_is_server_auth_failed() {
    let err = RecommendServiceClient::new(channel().await)
        .get_daily_recommend(GetDailyRecommendRequest {})
        .await
        .expect_err("无令牌必须被拒");

    assert_eq!(err.code(), Code::Unauthenticated);
    assert_eq!(reason_of(&err), ("kite.music".into(), "SERVER_AUTH_FAILED".into()));
    // 服务令牌错误是配置问题:不是「重新登录网易云」,也不该重试。
    assert_eq!(classify(&err), Action::GiveUp);
}

#[tokio::test]
#[ignore = "需要运行中的 kite-server,见 scripts/rust-interop.sh"]
async fn wrong_token_is_server_auth_failed() {
    let interceptor = auth_interceptor("definitely-wrong").unwrap();
    let err = RecommendServiceClient::with_interceptor(channel().await, interceptor)
        .get_daily_recommend(GetDailyRecommendRequest {})
        .await
        .expect_err("错误令牌必须被拒");

    assert_eq!(reason_of(&err).1, "SERVER_AUTH_FAILED");
}

// 令牌正确 → 请求到达业务层;没有网易云 cookie,所以是 LOGIN_REQUIRED。
// 它与 SERVER_AUTH_FAILED 同为 UNAUTHENTICATED,客户端只能靠 ErrorInfo.reason 区分,
// 这正是 reason 作为对外契约的意义。
#[tokio::test]
#[ignore = "需要运行中的 kite-server,见 scripts/rust-interop.sh"]
async fn valid_token_reaches_business_layer() {
    let interceptor = auth_interceptor(&env("KITE_TOKEN")).unwrap();
    let err = RecommendServiceClient::with_interceptor(channel().await, interceptor)
        .get_daily_recommend(GetDailyRecommendRequest {})
        .await
        .expect_err("没有网易云登录态应失败");

    assert_eq!(err.code(), Code::Unauthenticated);
    assert_eq!(reason_of(&err), ("kite.music".into(), "LOGIN_REQUIRED".into()));
    assert_eq!(classify(&err), Action::Relogin);
}
