//! kite-server 的 Rust 客户端互操作冒烟。
//!
//! `classify` 与 `auth` 两个模块的内容**逐字等于**文档里的 Rust 代码块
//! (`docs/grpc-errors.md`、`docs/server-security.md`),由 Go 测试守护一致——
//! 文档里的示例因此是被编译、被运行过的,不会腐烂。要改示例,先改文档,再同步到这里。

pub mod auth;
pub mod classify;

/// 由 proto 描述符生成的 tonic 客户端(见 build.rs)。
pub mod pb {
    #![allow(clippy::all, clippy::pedantic)]
    pub mod netease {
        pub mod music {
            pub mod v1 {
                tonic::include_proto!("netease.music.v1");
            }
        }
    }
}
