use tonic::{metadata::MetadataValue, Request, Status};

// `use<>`:返回的拦截器不借用 `token`,否则传入临时字符串(如从环境变量读取)时无法编译。
pub fn auth_interceptor(
    token: &str,
) -> Result<impl FnMut(Request<()>) -> Result<Request<()>, Status> + Clone + use<>, tonic::metadata::errors::InvalidMetadataValue>
{
    let value: MetadataValue<_> = format!("Bearer {token}").parse()?;
    Ok(move |mut req: Request<()>| {
        req.metadata_mut().insert("authorization", value.clone());
        Ok(req)
    })
}

// let client = SongServiceClient::with_interceptor(channel, auth_interceptor(&token)?);
