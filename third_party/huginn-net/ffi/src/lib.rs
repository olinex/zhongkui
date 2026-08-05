//! huginn_ffi：Huginn Net 被动指纹识别库的 C ABI 封装 crate。
//!
//! 以 Rust → C → Go 的 FFI 形式，将 `huginn-net-tcp`（p0f 风格 TCP/OS 指纹）、
//! `huginn-net-http`（p0f 风格 HTTP 指纹）与 `huginn-net-db`（p0f 签名库）集成到
//! Go 侧调用方（通过 cgo 链接本 crate 产出的 cdylib / staticlib）。
//!
//! 约定：
//! - 所有导出函数返回的 C 字符串（`*mut c_char`）均由本库通过 `CString::into_raw`
//!   分配，调用方必须使用 [`huginn_free`] 释放，避免内存泄漏。
//! - 所有入参指针先做 null / 长度校验，空输入一律返回 NULL。
//! - 分析失败（无法解析、未命中握手包等）一律返回 NULL。

use huginn_net_db::{Database, SharedHttpSignatureMatcher, SharedTcpSignatureMatcher};
use huginn_net_http::matcher_api::HttpMatcher;
use huginn_net_http::{Http1Processor, HttpProcessor};
use huginn_net_tcp::output::{MatchQuality, OperativeSystem, TcpAnalysisResult};
use huginn_net_tcp::{process_ipv4_packet, process_ipv6_packet, ConnectionTracker};
use libc::{c_char, c_void};
use pnet::packet::ipv4::Ipv4Packet;
use pnet::packet::ipv6::Ipv6Packet;
use std::ffi::CString;
use std::ptr::null_mut;
use std::slice;
use std::sync::{LazyLock, Mutex, OnceLock};

/// FFI 版本号常量
const FFI_VERSION: &str = "huginn-ffi 0.1.0";

/// 全局连接追踪器容量上限（并发连接数上界）
const MAX_TRACKED_CONNECTIONS: usize = 1024;

/// MTU 缺省值：未提取到 MTU 时的兜底值
const DEFAULT_MTU: u16 = 1500;

// ---------------------------------------------------------------------------
// 全局状态：签名库与匹配器只加载一次（OnceLock 懒加载），
// 连接追踪器为跨线程共享的可变状态（Mutex 保护）。
// ---------------------------------------------------------------------------

/// 全局 p0f 签名库（懒加载；`Err` 表示内置签名库解析失败，此后调用方返回 NULL）
static DB: OnceLock<Result<Database, ()>> = OnceLock::new();

/// 全局 TCP 签名匹配器（懒加载；`Err` 表示签名库不可用）
static TCP_MATCHER: OnceLock<Result<SharedTcpSignatureMatcher, ()>> = OnceLock::new();

/// 全局 HTTP 签名匹配器（懒加载；`Err` 表示签名库不可用）
static HTTP_MATCHER: OnceLock<Result<SharedHttpSignatureMatcher, ()>> = OnceLock::new();

/// 全局连接追踪器（Mutex 保护，供 SYN / SYN-ACK 状态追踪使用）
static CONN_TRACKER: LazyLock<Mutex<ConnectionTracker>> =
    LazyLock::new(|| Mutex::new(ConnectionTracker::new(MAX_TRACKED_CONNECTIONS)));

/// 获取全局签名库；内置 p0f.fp 解析失败时返回 `None`（调用方应返回 NULL）
fn db() -> Option<&'static Database> {
    DB.get_or_init(|| Database::load_default().map_err(|_| ())).as_ref().ok()
}

/// 获取全局 TCP 匹配器；签名库不可用时返回 `None`
fn tcp_matcher() -> Option<&'static SharedTcpSignatureMatcher> {
    TCP_MATCHER
        .get_or_init(|| {
            let database = db().ok_or(())?;
            Ok::<_, ()>(SharedTcpSignatureMatcher::from_database(database))
        })
        .as_ref()
        .ok()
}

/// 获取全局 HTTP 匹配器；签名库不可用时返回 `None`
fn http_matcher() -> Option<&'static SharedHttpSignatureMatcher> {
    HTTP_MATCHER
        .get_or_init(|| {
            let database = db().ok_or(())?;
            Ok::<_, ()>(SharedHttpSignatureMatcher::from_database(database))
        })
        .as_ref()
        .ok()
}

/// 获取全局连接追踪器的互斥锁守卫（自动恢复锁中毒状态）
fn tracker() -> std::sync::MutexGuard<'static, ConnectionTracker> {
    CONN_TRACKER.lock().unwrap_or_else(|poisoned| poisoned.into_inner())
}

// ---------------------------------------------------------------------------
// 内部辅助函数
// ---------------------------------------------------------------------------

/// 将外部指针安全地转换为字节切片；空指针或零长度返回 `None`
///
/// # Safety
/// 调用方必须保证 `data` 指向至少 `len` 字节的有效内存。
unsafe fn to_bytes<'a>(data: *const c_void, len: usize) -> Option<&'a [u8]> {
    if data.is_null() || len == 0 {
        return None;
    }
    Some(slice::from_raw_parts(data.cast::<u8>(), len))
}

/// 将字符串封装为 C 字符串返回（调用方用 [`huginn_free`] 释放）；构造失败返回 NULL
fn to_cstring(text: String) -> *mut c_char {
    match CString::new(text) {
        Ok(cs) => cs.into_raw(),
        Err(_) => null_mut(),
    }
}

/// 提取匹配质量分数；未匹配或匹配被禁用时返回 0.0
fn quality_score(quality: &MatchQuality) -> f32 {
    match quality {
        MatchQuality::Matched(score) => *score,
        MatchQuality::NotMatched | MatchQuality::Disabled => 0.0,
    }
}

/// 格式化猜测串：优先 `name`，其次 `name variant`，兜底 `family`
fn format_guess(name: &str, family: &Option<String>, variant: &Option<String>) -> String {
    let mut guess = String::new();
    if !name.is_empty() {
        guess.push_str(name);
    }
    if let Some(v) = variant.as_deref() {
        if !v.is_empty() {
            if !guess.is_empty() {
                guess.push(' ');
            }
            guess.push_str(v);
        }
    }
    if guess.is_empty() {
        if let Some(f) = family.as_deref() {
            guess.push_str(f);
        }
    }
    guess
}

/// 将 TCP 匹配结果渲染为固定 JSON 契约：
/// `{"os":"<OS 猜测串>","os_quality":0.0,"sig":"<原始签名串>","mtu":1500}`
fn render_tcp_json(
    os: Option<&OperativeSystem>,
    quality: &MatchQuality,
    sig: &dyn std::fmt::Display,
    mtu: u16,
) -> String {
    let os_guess = os
        .map(|os| format_guess(&os.name, &os.family, &os.variant))
        .unwrap_or_default();
    let sig_str = sig.to_string();
    serde_json::json!({
        "os": os_guess,
        "os_quality": quality_score(quality),
        "sig": sig_str,
        "mtu": mtu,
    })
    .to_string()
}

/// 从 TCP 分析结果提取 JSON：优先 SYN（客户端）结果，其次 SYN-ACK（服务端）结果；
/// 两者均不存在（如非握手包）时返回 `None`
fn tcp_result_to_json(result: &TcpAnalysisResult) -> Option<String> {
    let mtu = result.mtu.as_ref().map_or(DEFAULT_MTU, |mtu| mtu.mtu);
    if let Some(syn) = &result.syn {
        return Some(render_tcp_json(
            syn.os_matched.os.as_ref(),
            &syn.os_matched.quality,
            &syn.sig,
            mtu,
        ));
    }
    if let Some(syn_ack) = &result.syn_ack {
        return Some(render_tcp_json(
            syn_ack.os_matched.os.as_ref(),
            &syn_ack.os_matched.quality,
            &syn_ack.sig,
            mtu,
        ));
    }
    None
}

// ---------------------------------------------------------------------------
// FFI 导出函数
// ---------------------------------------------------------------------------

/// 分析一个完整的 IPv4 数据包（SYN / SYN-ACK），返回 TCP 指纹 JSON 契约字符串。
///
/// 返回的字符串由 [`huginn_free`] 释放；入参为空、无法解析或未识别出握手包时返回 NULL。
///
/// # Safety
/// `data` 必须指向长度为 `len` 的有效原始 IP 数据包内存。
#[no_mangle]
pub unsafe extern "C" fn huginn_tcp_analyze_ipv4(
    data: *const c_void,
    len: usize,
) -> *mut c_char {
    let Some(bytes) = to_bytes(data, len) else {
        return null_mut();
    };
    let Some(ipv4) = Ipv4Packet::new(bytes) else {
        return null_mut();
    };
    let Some(matcher) = tcp_matcher() else {
        return null_mut();
    };
    let mut tracker = tracker();
    match process_ipv4_packet(&ipv4, &mut tracker, Some(matcher)) {
        Ok(result) => match tcp_result_to_json(&result) {
            Some(json) => to_cstring(json),
            None => null_mut(),
        },
        Err(_) => null_mut(),
    }
}

/// 分析一个完整的 IPv6 数据包（SYN / SYN-ACK），返回 TCP 指纹 JSON 契约字符串。
///
/// 返回的字符串由 [`huginn_free`] 释放；入参为空、无法解析或未识别出握手包时返回 NULL。
///
/// # Safety
/// `data` 必须指向长度为 `len` 的有效原始 IP 数据包内存。
#[no_mangle]
pub unsafe extern "C" fn huginn_tcp_analyze_ipv6(
    data: *const c_void,
    len: usize,
) -> *mut c_char {
    let Some(bytes) = to_bytes(data, len) else {
        return null_mut();
    };
    let Some(ipv6) = Ipv6Packet::new(bytes) else {
        return null_mut();
    };
    let Some(matcher) = tcp_matcher() else {
        return null_mut();
    };
    let mut tracker = tracker();
    match process_ipv6_packet(&ipv6, &mut tracker, Some(matcher)) {
        Ok(result) => match tcp_result_to_json(&result) {
            Some(json) => to_cstring(json),
            None => null_mut(),
        },
        Err(_) => null_mut(),
    }
}

/// 分析一段完整的 HTTP/1.x 请求数据，返回 JSON 契约：
/// `{"browser":"<浏览器猜测>","language":"<语言偏好>","quality":0.0}`
///
/// 返回的字符串由 [`huginn_free`] 释放；入参为空或无法解析时返回 NULL。
///
/// # Safety
/// `data` 必须指向长度为 `len` 的有效原始 HTTP 数据内存。
#[no_mangle]
pub unsafe extern "C" fn huginn_http_analyze_request(
    data: *const c_void,
    len: usize,
) -> *mut c_char {
    let Some(bytes) = to_bytes(data, len) else {
        return null_mut();
    };
    let processor = Http1Processor::new();
    let Ok(Some(observable)) = processor.process_request(bytes) else {
        return null_mut();
    };
    let Some(matcher) = http_matcher() else {
        return null_mut();
    };
    let matched = matcher.match_http_request(&observable.matching);
    let browser = matched
        .as_ref()
        .map(|m| format_guess(&m.browser.name, &m.browser.family, &m.browser.variant))
        .unwrap_or_default();
    let quality = matched.as_ref().map_or(0.0, |m| m.quality);
    let language = observable.lang.unwrap_or_default();
    to_cstring(
        serde_json::json!({
            "browser": browser,
            "language": language,
            "quality": quality,
        })
        .to_string(),
    )
}

/// 分析一段完整的 HTTP/1.x 响应数据，返回 JSON 契约：
/// `{"server":"<服务器软件猜测>","quality":0.0}`
///
/// 返回的字符串由 [`huginn_free`] 释放；入参为空或无法解析时返回 NULL。
///
/// # Safety
/// `data` 必须指向长度为 `len` 的有效原始 HTTP 数据内存。
#[no_mangle]
pub unsafe extern "C" fn huginn_http_analyze_response(
    data: *const c_void,
    len: usize,
) -> *mut c_char {
    let Some(bytes) = to_bytes(data, len) else {
        return null_mut();
    };
    let processor = Http1Processor::new();
    let Ok(Some(observable)) = processor.process_response(bytes) else {
        return null_mut();
    };
    let Some(matcher) = http_matcher() else {
        return null_mut();
    };
    let matched = matcher.match_http_response(&observable.matching);
    let server = matched
        .as_ref()
        .map(|m| format_guess(&m.web_server.name, &m.web_server.family, &m.web_server.variant))
        .unwrap_or_default();
    let quality = matched.as_ref().map_or(0.0, |m| m.quality);
    to_cstring(
        serde_json::json!({
            "server": server,
            "quality": quality,
        })
        .to_string(),
    )
}

/// 释放由本库分配的 C 字符串内存；传入 NULL 时为安全无操作。
///
/// # Safety
/// `ptr` 必须是本库先前通过 `CString::into_raw` 返回的指针，且只能释放一次。
#[no_mangle]
pub unsafe extern "C" fn huginn_free(ptr: *mut c_char) {
    if ptr.is_null() {
        return;
    }
    drop(CString::from_raw(ptr));
}

/// 返回版本字符串 `"huginn-ffi 0.1.0"`（调用方用 [`huginn_free`] 释放）。
#[no_mangle]
pub unsafe extern "C" fn huginn_version() -> *mut c_char {
    to_cstring(FFI_VERSION.to_string())
}

// ---------------------------------------------------------------------------
// 单元测试
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use pnet::packet::ip::IpNextHeaderProtocols;
    use pnet::packet::ipv4::{Ipv4Flags, MutableIpv4Packet};
    use pnet::packet::tcp::{MutableTcpPacket, TcpFlags, TcpOption};
    use std::ffi::CStr;
    use std::net::Ipv4Addr;

    /// 构造典型的 Linux 3.1-3.10 IPv4 SYN 数据包：
    /// TTL=64、MSS=1460、窗口=MSS*10（14600）、WS=6、选项 mss,sok,ts,nop,ws、
    /// DF 置位且 IP 标识非零（id+）、无载荷。
    /// 对应 p0f.fp 签名 `*:64:0:*:mss*10,6:mss,sok,ts,nop,ws:df,id+:0`
    /// → 标签 `s:unix:Linux:3.1-3.10`
    fn build_linux_syn_packet() -> Vec<u8> {
        // TCP 头 + 20 字节选项（4+2+10+1+3=20，恰好 4 字节对齐，无需填充）
        let mut tcp_buf = vec![0u8; 40];
        let mut tcp = MutableTcpPacket::new(&mut tcp_buf).expect("TCP 缓冲区容量足够");
        tcp.set_source(12345);
        tcp.set_destination(80);
        tcp.set_sequence(0x0102_0304);
        tcp.set_acknowledgement(0);
        tcp.set_flags(TcpFlags::SYN);
        tcp.set_window(14600);
        tcp.set_urgent_ptr(0);
        // 必须先设置 data_offset（40/4=10 字），pnet 生成的 set_options 依此计算选项区长度
        tcp.set_data_offset(10);
        tcp.set_options(&[
            TcpOption::mss(1460),
            TcpOption::sack_perm(),
            TcpOption::timestamp(0x0102_0304, 0),
            TcpOption::nop(),
            TcpOption::wscale(6),
        ]);

        // IPv4 头 + TCP 载荷
        let mut ip_buf = vec![0u8; 20 + tcp_buf.len()];
        let mut ipv4 = MutableIpv4Packet::new(&mut ip_buf).expect("IPv4 缓冲区容量足够");
        ipv4.set_version(4);
        ipv4.set_header_length(5);
        ipv4.set_total_length((20 + tcp_buf.len()) as u16);
        ipv4.set_identification(0x1234);
        ipv4.set_flags(Ipv4Flags::DontFragment);
        ipv4.set_ttl(64);
        ipv4.set_next_level_protocol(IpNextHeaderProtocols::Tcp);
        ipv4.set_source(Ipv4Addr::new(192, 168, 1, 1));
        ipv4.set_destination(Ipv4Addr::new(10, 0, 0, 1));
        ipv4.set_payload(&tcp_buf);
        ip_buf
    }

    /// 将 FFI 返回的 C 字符串读取为 Rust String 并释放（断言非 NULL）
    unsafe fn take_cstring(ptr: *mut c_char) -> String {
        assert!(!ptr.is_null(), "FFI 返回了 NULL，预期非空结果");
        let s = CStr::from_ptr(ptr).to_string_lossy().into_owned();
        huginn_free(ptr);
        s
    }

    /// 测试 1：已知 p0f 签名 SYN 包应匹配出操作系统，且 os_quality 在 0.0~1.0
    #[test]
    fn tcp_syn_linux_packet_matches() {
        let packet = build_linux_syn_packet();
        let json = unsafe {
            take_cstring(huginn_tcp_analyze_ipv4(packet.as_ptr().cast(), packet.len()))
        };
        let value: serde_json::Value =
            serde_json::from_str(&json).expect("返回内容应为合法 JSON");
        // 便于调试：--nocapture 时输出真实匹配结果
        println!("SYN 匹配结果: {json}");
        let os = value.get("os").and_then(|v| v.as_str()).unwrap_or("");
        let os_quality = value.get("os_quality").and_then(|v| v.as_f64()).unwrap_or(-1.0);
        assert!(
            os.starts_with("Linux"),
            "已知 p0f 签名 SYN 包应匹配出 Linux，实际 os: {os}，完整 JSON: {json}"
        );
        assert!(
            (0.0..=1.0).contains(&os_quality),
            "os_quality 应在 0.0~1.0 之间，实际: {os_quality}"
        );
        assert!(value.get("sig").and_then(|v| v.as_str()).is_some(), "sig 字段缺失: {json}");
        assert_eq!(
            value.get("mtu").and_then(|v| v.as_u64()),
            Some(1500),
            "MTU 应为缺省 1500，实际 JSON: {json}"
        );
    }

    /// 测试 2：版本号非空
    #[test]
    fn version_is_non_empty() {
        let version = unsafe { take_cstring(huginn_version()) };
        assert!(!version.is_empty(), "版本号不应为空");
        assert!(version.starts_with("huginn-ffi"), "版本号前缀不符: {version}");
    }

    /// 测试 3：空指针 / 零长度调用返回 NULL 且不 panic；释放 NULL 指针安全
    #[test]
    fn null_and_empty_inputs_return_null() {
        unsafe {
            assert!(huginn_tcp_analyze_ipv4(std::ptr::null(), 0).is_null());
            assert!(huginn_tcp_analyze_ipv4(std::ptr::null(), 32).is_null());
            assert!(huginn_tcp_analyze_ipv6(std::ptr::null(), 0).is_null());
            assert!(huginn_tcp_analyze_ipv6(std::ptr::null(), 64).is_null());
            assert!(huginn_http_analyze_request(std::ptr::null(), 0).is_null());
            assert!(huginn_http_analyze_response(std::ptr::null(), 0).is_null());
            // 释放 NULL 指针必须是安全无操作
            huginn_free(std::ptr::null_mut());
        }
    }

    /// 补充：HTTP 请求 / 响应可解析为固定 JSON 契约
    #[test]
    fn http_request_and_response_parse() {
        let request = b"GET /index.html HTTP/1.1\r\nHost: example.com\r\nUser-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0\r\nAccept-Language: en-US,en;q=0.5\r\nAccept: text/html\r\n\r\n";
        let json = unsafe {
            take_cstring(huginn_http_analyze_request(request.as_ptr().cast(), request.len()))
        };
        let value: serde_json::Value =
            serde_json::from_str(&json).expect("请求分析结果应为合法 JSON");
        assert!(value.get("language").and_then(|v| v.as_str()).is_some(), "language 字段缺失: {json}");
        assert!(value.get("browser").and_then(|v| v.as_str()).is_some(), "browser 字段缺失: {json}");
        assert!(value.get("quality").and_then(|v| v.as_f64()).is_some(), "quality 字段缺失: {json}");

        let response = b"HTTP/1.1 200 OK\r\nServer: nginx/1.18.0\r\nContent-Type: text/html\r\nContent-Length: 10\r\n\r\nhello world";
        let json = unsafe {
            take_cstring(huginn_http_analyze_response(response.as_ptr().cast(), response.len()))
        };
        let value: serde_json::Value =
            serde_json::from_str(&json).expect("响应分析结果应为合法 JSON");
        assert!(value.get("server").and_then(|v| v.as_str()).is_some(), "server 字段缺失: {json}");
        assert!(value.get("quality").and_then(|v| v.as_f64()).is_some(), "quality 字段缺失: {json}");
    }
}
