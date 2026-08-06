;
; p0f.local.fp - 本项目本地定制指纹签名（overlay）
; -------------------------------------------------
;
; 该文件与上游 p0f.fp（third_party/huginn-net/ffi/p0f.fp）拼接后由 huginn_ffi
; 加载。上游 p0f.fp 可通过 `make p0f-update` 从社区维护的 p0f3 签名库刷新，
; 本文件中的本地定制签名不会被上游覆盖。
;
; 格式遵循 p0f3 *.fp：`[module:direction]` 头切换当前 section，label/sys/sig
; 行追加到当前 section。huginn 解析器逐行处理，重复的 section 头安全，
; 因此拼接加载与单个文件语义一致。
;
; 版权说明：p0f.fp 系列源自 Michal Zalewski 的 p0f3，按 GNU LGPL 分发；
; 本文件是针对现代浏览器与 macOS 回环抓包场景的本地补充签名。

[tcp:request]

; macOS loopback：lo0 抓包 IP ID 恒为 0（id-），上游签名均带 df,id+ 无法命中
label = s:unix:Mac OS X:10.x
sig   = *:64:0:*:65535,6:mss,nop,ws,nop,nop,ts,sok,eol+1:df:0

[tcp:response]

; Modern macOS (Ventura+): window scale 6（上游签名全部为 65535,0，WS=6 无法识别）
label = s:unix:Mac OS X:10.x
sig   = *:64:0:*:65535,6:mss,nop,ws,nop,nop,ts,sok,eol+1:df:0
sig   = *:64:0:*:65535,6:mss,nop,ws,nop,nop,ts,sok,eol+1:df,id+:0

[http:request]

; Modern Firefox (Quantum+): 头序 Host, UA, Accept-Language, Accept[, Connection]
label = s:!:Firefox:115 or newer
sys   = Windows,@unix
sig   = 1:Host,User-Agent,?Accept-Language,Accept,?Connection:Accept-Charset:Firefox/

; Modern Chrome (Chromium 90+, desktop): Client Hints（sec-ch-ua）与 Sec-Fetch-* 头
label = s:!:Chrome:11 or newer
sys   = Windows,@unix
sig   = 1:Host,Connection,?Cache-Control,sec-ch-ua,?sec-ch-ua-mobile,?sec-ch-ua-platform,?Upgrade-Insecure-Requests,User-Agent,Accept,?Sec-Fetch-Site,?Sec-Fetch-Mode,?Sec-Fetch-User,?Sec-Fetch-Dest,?Accept-Encoding,?Accept-Language,?Cookie:Accept-Charset: Chrome/
; Modern Chrome (desktop, 无 Client Hints): 头序 Host, Connection, UA, Accept...
sig   = 1:Host,Connection,?Upgrade-Insecure-Requests,User-Agent,Accept,?Accept-Encoding,?Accept-Language:Accept-Charset: Chrome/
