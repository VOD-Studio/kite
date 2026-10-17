//! 不需要服务的单元测试:文档示例本身的行为。

use kite_interop::auth::auth_interceptor;
use kite_interop::classify::{Action, classify};
use tonic::{Code, Request, Status};
use tonic_types::{ErrorDetails, StatusExt};

fn status_with_reason(code: Code, reason: &str) -> Status {
    Status::with_error_details(code, "msg", ErrorDetails::with_error_info(reason, "kite.music", std::collections::HashMap::<String, String>::new()))
}

#[test]
fn classify_by_reason() {
    assert_eq!(classify(&status_with_reason(Code::Unauthenticated, "LOGIN_REQUIRED")), Action::Relogin);
    assert_eq!(classify(&status_with_reason(Code::ResourceExhausted, "RATE_LIMITED")), Action::RetryLater);
    assert_eq!(classify(&status_with_reason(Code::Unavailable, "CIRCUIT_OPEN")), Action::RetryLater);
    assert_eq!(classify(&status_with_reason(Code::Unauthenticated, "SERVER_AUTH_FAILED")), Action::GiveUp);
    assert_eq!(classify(&status_with_reason(Code::NotFound, "RESOURCE_NOT_FOUND")), Action::GiveUp);
}

#[test]
fn classify_without_error_info_falls_back_to_code() {
    assert_eq!(classify(&Status::unavailable("连接断开")), Action::RetryLater);
    assert_eq!(classify(&Status::internal("x")), Action::GiveUp);
}

#[test]
fn interceptor_sets_bearer_header() {
    let mut f = auth_interceptor("s3cret").unwrap();
    let req = f(Request::new(())).unwrap();
    assert_eq!(req.metadata().get("authorization").unwrap(), "Bearer s3cret");
}

#[test]
fn interceptor_rejects_invalid_token_without_panicking() {
    assert!(auth_interceptor("bad\ntoken").is_err());
}
