use tonic::{Code, Status};
use tonic_types::StatusExt;

#[derive(Debug, PartialEq)]
pub enum Action {
    Relogin,
    RetryLater,
    GiveUp,
}

/// 按 kite 的 ErrorInfo.reason 决定动作。
pub fn classify(status: &Status) -> Action {
    let details = status.get_error_details();
    match details.error_info() {
        Some(info) if info.domain == "kite.music" => match info.reason.as_str() {
            "LOGIN_REQUIRED" => Action::Relogin,
            "RATE_LIMITED" | "UPSTREAM_UNAVAILABLE" | "CIRCUIT_OPEN" => Action::RetryLater,
            _ => Action::GiveUp,
        },
        // 没有 ErrorInfo(例如网络层错误)时退回 gRPC code。
        _ => match status.code() {
            Code::Unavailable => Action::RetryLater,
            _ => Action::GiveUp,
        },
    }
}
