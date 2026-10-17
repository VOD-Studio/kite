// 从 buf 产出的 FileDescriptorSet 生成 tonic 客户端,不需要本机安装 protoc。
// 描述符由 scripts/rust-interop.sh 用 `buf build` 生成(包含 google.api 等依赖)。
use prost::Message;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let path = std::env::var("KITE_PROTO_IMAGE").unwrap_or_else(|_| "kite.binpb".to_string());
    println!("cargo:rerun-if-env-changed=KITE_PROTO_IMAGE");
    println!("cargo:rerun-if-changed={path}");

    let bytes = std::fs::read(&path).map_err(|e| {
        format!("读取 proto 描述符 {path} 失败: {e}\n先运行 scripts/rust-interop.sh,或 `buf build proto -o interop/rust/kite.binpb`")
    })?;
    let fds = prost_types::FileDescriptorSet::decode(bytes.as_slice())?;

    tonic_prost_build::configure().build_server(false).compile_fds(fds)?;
    Ok(())
}
