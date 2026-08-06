package detectors

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	. "github.com/smartystreets/goconvey/convey"
	n "zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/huginn"
)

// fingerprintCase 描述一个 pcap 夹具的期望指纹结果（基线）。
// 期望值由当前 huginn 指纹库对夹具签名（testdata/fp/*.pcap）的实际识别结果固定；
// 当指纹库或识别逻辑改进时，更新期望值即可跟踪行为变化。
type fingerprintCase struct {
	// name 用例名称（测试子用例标识）
	name string
	// file testdata/fp 下的完整文件名（含后缀，如 .pcap / .pcapng）
	file string
	// 期望的 TCP 指纹（buildTcpFingerprint 输出）
	clientOS  string
	serverOS  string
	clientSig string // 期望客户端签名包含的子串
	mtu       uint16
	// 期望的 Http 指纹（buildHttpFingerprint 输出）
	browser        string
	server         string
	language       string
	deviceGuess    string
	browserQuality float64 // 0 表示不校验
	serverQuality  float64 // 0 表示不校验
}

// fingerprintCases 返回全部指纹验证用例（夹具 pcap 需先经 TestGenerateFingerprintPcapFixtures -update 生成）。
// 期望值 = 当前 huginn 指纹库对夹具签名（testdata/fp/*.pcap）的实际识别结果（基线）。
// 基线暴露了当前指纹库的识别缺口（浏览器/服务端/客户端 OS 误判或未识别），
// 这正是需要改进的方向；改进指纹库后更新期望值即可跟踪行为变化。
func fingerprintCases() []fingerprintCase {
	return []fingerprintCase{
		{
			// 场景：Linux 客户端 + Firefox 浏览器访问 nginx 服务端。
			// 基线：TCP 客户端 OS 识别正确；服务端 SYN-ACK 未识别 OS（server 侧签名缺失）；
			// Firefox 头（Host,User-Agent,...:Firefox/）经定制 p0f.fp 的现代 Firefox 签名正确识别；
			// nginx 响应头经上游 http:response 签名正确识别为 nginx 1.x（expsw 匹配）；
			// 语言识别正确（en-US → English），设备类别因浏览器命中为 desktop。
			name:           "fp_linux_firefox_nginx",
			file:           "fp_linux_firefox_nginx.pcap",
			clientOS:       "Linux 3.1-3.10",
			serverOS:       "",
			clientSig:      "4:64+0:0:1460:mss*10,6",
			mtu:            1500,
			browser:        "Firefox 115 or newer",
			browserQuality: 0.95,
			server:         "nginx 1.x",
			serverQuality:  0.9,
			language:       "English",
			deviceGuess:    "desktop",
		},
		{
			// 场景：Windows 10 客户端 + Chrome 浏览器访问 IIS 服务端。
			// 基线：Windows 风格 SYN（TTL 128 / mss,nop,ws,nop,nop,sok）识别为 Windows 10 or 11
			// （上游新库标签合并）；服务端 SYN-ACK 未识别 OS；Chrome 头（Host,Connection,...:Chrome/）
			// 经定制 p0f.fp 的现代 Chrome 签名正确识别；IIS 响应（含 Date 头）识别正确；
			// 语言识别正确（zh-CN → Chinese）。
			name:           "fp_windows_chrome_iis",
			file:           "fp_windows_chrome_iis.pcap",
			clientOS:       "Windows 10 or 11",
			serverOS:       "",
			clientSig:      "4:128+0:0:1460:65535,8",
			mtu:            1492,
			browser:        "Chrome 11 or newer",
			browserQuality: 1.0,
			server:         "IIS 7.x",
			serverQuality:  0.95,
			language:       "Chinese",
			deviceGuess:    "desktop",
		},
		{
			// 场景：macOS 客户端（本机 Chrome）访问 rustfs 控制台的真实 loopback 抓包（pcapng，多流）。
			// 基线：TCP 签名（mss,nop,ws,nop,nop,ts,sok,eol+1 / MSS 16344）经定制 p0f.fp 的
			// macOS loopback 变体（IP ID 恒 0，df:0）正确识别客户端/服务端为 Mac OS X 10.x
			// （loopback 上客户端与服务端为同一主机）；Chrome 请求头经现代 Chrome 签名识别为
			// Chrome 11 or newer；rustfs 响应头被误判为 Google Web Server（quality 0.8，
			// 上游 http.fp 签名误配）；设备类别为 desktop。
			name:           "rustfs",
			file:           "rustfs.pcapng",
			clientOS:       "Mac OS X 10.x",
			serverOS:       "Mac OS X 10.x",
			clientSig:      "4:64+0:0:16344:65535,6",
			mtu:            16388,
			browser:        "Chrome 11 or newer",
			browserQuality: 0.95,
			server:         "Google Web Server",
			serverQuality:  0.8,
			language:       "Chinese",
			deviceGuess:    "desktop",
		},
	}
}

// pcapFingerprintResult 一次 pcap 分析的完整指纹结果，供断言与调试输出。
type pcapFingerprintResult struct {
	tcpFP  n.TcpFingerprint
	httpFP n.HttpFingerprint
	// 原始分析结果，便于失败时输出定位
	clientFP *huginn.TCPFingerprint
	serverFP *huginn.TCPFingerprint
	reqFP    *huginn.HTTPFingerprint
	rspFP    *huginn.HTTPFingerprint
}

// pcapPacketReader 是经典 pcap（pcapgo.Reader）与 pcapng（pcapgo.NgReader）的公共读取接口。
type pcapPacketReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

// openPcapReader 按文件 magic 打开经典 pcap 或 pcapng，返回读取器与关闭函数。
func openPcapReader(t *testing.T, path string) (pcapPacketReader, func()) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	br := bufio.NewReader(f)
	magic, err := br.Peek(4)
	if err != nil {
		f.Close()
		t.Fatalf("peek magic %s: %v", path, err)
	}
	// pcapng 的 block magic 为 0x0a0d0d0a；经典 pcap 以 0xa1b2c3d4 系列 magic 开头
	ng := binary.LittleEndian.Uint32(magic) == 0x0a0d0d0a || binary.BigEndian.Uint32(magic) == 0x0a0d0d0a
	var reader pcapPacketReader
	if ng {
		reader, err = pcapgo.NewNgReader(br, pcapgo.DefaultNgReaderOptions)
	} else {
		reader, err = pcapgo.NewReader(br)
	}
	if err != nil {
		f.Close()
		t.Fatalf("pcap reader %s: %v", path, err)
	}
	return reader, func() { f.Close() }
}

// readFirstFlow 扫描全部包，锁定首个 SYN（无 ACK）包所属连接为待分析主流，
// 返回该流的客户端/服务端端口、SYN / SYN-ACK 原始握手包字节与双向拼接载荷；
// 其余并发连接一律忽略。
func readFirstFlow(reader pcapPacketReader) (clientPort, serverPort layers.TCPPort,
	clientRaw, serverRaw, c2sPayload, s2cPayload []byte) {
	for {
		data, _, err := reader.ReadPacketData()
		if err != nil {
			break // io.EOF 表示读完
		}
		packet := gopacket.NewPacket(data, reader.LinkType(), gopacket.Default)
		netLayer := packet.NetworkLayer()
		tcpLayer, ok := packet.TransportLayer().(*layers.TCP)
		if !ok || netLayer == nil {
			continue
		}
		// 锁定主流：首个 SYN（无 ACK）包为客户端发起方，其四元组即待分析连接
		if clientPort == 0 && tcpLayer.SYN && !tcpLayer.ACK {
			clientPort, serverPort = tcpLayer.SrcPort, tcpLayer.DstPort
		}
		if clientPort == 0 {
			continue
		}
		// 仅处理主流相关包，忽略其它并发连接
		belong := (tcpLayer.SrcPort == clientPort && tcpLayer.DstPort == serverPort) ||
			(tcpLayer.SrcPort == serverPort && tcpLayer.DstPort == clientPort)
		if !belong {
			continue
		}
		// 握手包：SYN（无 ACK）为客户端指纹，SYN-ACK 为服务端指纹
		if tcpLayer.SYN {
			raw := append(append([]byte{}, netLayer.LayerContents()...), tcpLayer.LayerContents()...)
			if tcpLayer.ACK {
				serverRaw = raw
			} else {
				clientRaw = raw
			}
		}
		// 载荷：按方向拼接，供 HTTP 头指纹识别
		if len(tcpLayer.Payload) > 0 {
			if tcpLayer.SrcPort == clientPort {
				c2sPayload = append(c2sPayload, tcpLayer.Payload...)
			} else {
				s2cPayload = append(s2cPayload, tcpLayer.Payload...)
			}
		}
	}
	return
}

// analyzeFingerprintPcap 读取静态 pcap（经典 pcap 或 pcapng），沿生产链路计算指纹：
//  1. 握手包（SYN / SYN-ACK）→ huginn.AnalyzeTCPIPv4 识别客户端/服务端 OS；
//  2. 同方向 TCP 载荷拼接，截取头部 → huginn.AnalyzeHTTPRequest/Response 识别浏览器/服务端；
//  3. 组装 StreamMeta 与携带原始头字节的 *http.Request，调用 buildTcpFingerprint /
//     buildHttpFingerprint（与 NewHttpTransport 生产代码一致）。
//
// 支持真实抓包：多流 pcap 仅分析首条完整流（由首个 SYN 包锁定客户端/服务端端口），
// 客户端端口不再依赖合成夹具的固定值；其余流被忽略。
func analyzeFingerprintPcap(t *testing.T, path string) pcapFingerprintResult {
	t.Helper()
	reader, closeReader := openPcapReader(t, path)
	defer closeReader()
	clientPort, serverPort, clientRaw, serverRaw, c2sPayload, s2cPayload := readFirstFlow(reader)
	if len(clientRaw) == 0 || len(serverRaw) == 0 {
		t.Fatalf("%s: missing SYN/SYN-ACK packets", path)
	}

	result := pcapFingerprintResult{}
	var err1 error
	result.clientFP, err1 = huginn.AnalyzeTCPIPv4(clientRaw)
	if err1 != nil {
		t.Fatalf("AnalyzeTCPIPv4(client) %s: %v", path, err1)
	}
	result.serverFP, err1 = huginn.AnalyzeTCPIPv4(serverRaw)
	if err1 != nil {
		t.Fatalf("AnalyzeTCPIPv4(server) %s: %v", path, err1)
	}

	// 截取头部字节（首个空行前），与 Extract 阶段 captureBuffered 的行为一致
	reqHeaders := cutHeaders(c2sPayload)
	rspHeaders := cutHeaders(s2cPayload)
	if len(reqHeaders) == 0 || len(rspHeaders) == 0 {
		t.Fatalf("%s: missing HTTP request/response headers", path)
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(reqHeaders)))
	if err != nil {
		t.Fatalf("read request %s: %v", path, err)
	}
	req = req.WithContext(context.WithValue(req.Context(), rawHTTPKey{}, &rawHTTP{
		request:  reqHeaders,
		response: rspHeaders,
	}))
	result.reqFP, err1 = huginn.AnalyzeHTTPRequest(reqHeaders)
	if err1 != nil {
		t.Fatalf("AnalyzeHTTPRequest %s: %v", path, err1)
	}
	result.rspFP, err1 = huginn.AnalyzeHTTPResponse(rspHeaders)
	if err1 != nil {
		t.Fatalf("AnalyzeHTTPResponse %s: %v", path, err1)
	}

	streamMeta := &StreamMeta{
		ClientAddr: fpClientIP,
		ServerAddr: fpServerIP,
		ClientPort: clientPort,
		ServerPort: serverPort,
	}
	if result.clientFP != nil {
		streamMeta.ClientFingerprint = &n.PacketTCPFingerprint{
			OS: result.clientFP.OS, OSQuality: result.clientFP.OSQuality,
			Sig: result.clientFP.Sig, MTU: result.clientFP.MTU,
		}
	}
	if result.serverFP != nil {
		streamMeta.ServerFingerprint = &n.PacketTCPFingerprint{
			OS: result.serverFP.OS, OSQuality: result.serverFP.OSQuality,
			Sig: result.serverFP.Sig, MTU: result.serverFP.MTU,
		}
	}
	result.tcpFP = buildTcpFingerprint(streamMeta)
	result.httpFP = buildHttpFingerprint(streamMeta, req)
	return result
}

// cutHeaders 截取 HTTP 数据中的头部字节（首个 \r\n\r\n 前，含空行）。
func cutHeaders(data []byte) []byte {
	if i := bytes.Index(data, []byte("\r\n\r\n")); i >= 0 {
		return data[:i+4]
	}
	return data
}

// TestFingerprintFromPcap 通过静态 pcap 夹具验证设备指纹能力：
// 每个夹具覆盖一条完整的 TCP 会话（握手 + HTTP 事务），断言 TCP 指纹、Http 指纹与设备类别。
// 指纹识别由 huginn cgo 绑定提供（构建期强制链接，无降级路径）；夹具缺失时需先运行夹具生成器。
func TestFingerprintFromPcap(t *testing.T) {
	Convey("Test device fingerprint from static pcap", t, func() {
		for _, tc := range fingerprintCases() {
			Convey(tc.name, func(c C) {
				path := filepath.Join(testdataFpDir(), tc.file)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					c.SoMsg("夹具缺失，请先运行 go test ./internal/network/detectors -run TestGenerateFingerprintPcapFixtures -update",
						err, ShouldBeNil)
					return
				}
				got := analyzeFingerprintPcap(t, path)

				// TCP 指纹
				c.SoMsg("client OS 不匹配", got.tcpFP.ClientOS, ShouldEqual, tc.clientOS)
				c.SoMsg("server OS 不匹配", got.tcpFP.ServerOS, ShouldEqual, tc.serverOS)
				if tc.clientSig != "" {
					c.SoMsg("client sig 不包含期望签名", got.tcpFP.ClientSig, ShouldContainSubstring, tc.clientSig)
				}
				if tc.mtu != 0 {
					c.SoMsg("MTU 不匹配", got.tcpFP.MTU, ShouldEqual, tc.mtu)
				}

				// Http 指纹
				c.SoMsg("browser 不匹配", got.httpFP.Browser, ShouldEqual, tc.browser)
				if tc.browserQuality > 0 {
					c.SoMsg("browser quality 不匹配", got.httpFP.BrowserQuality, ShouldAlmostEqual, tc.browserQuality, 0.01)
				}
				c.SoMsg("http server 不匹配", got.httpFP.Server, ShouldEqual, tc.server)
				if tc.serverQuality > 0 {
					c.SoMsg("http server quality 不匹配", got.httpFP.ServerQuality, ShouldAlmostEqual, tc.serverQuality, 0.01)
				}
				c.SoMsg("language 不匹配", got.httpFP.Language, ShouldEqual, tc.language)
				c.SoMsg("device guess 不匹配", got.httpFP.DeviceGuess, ShouldEqual, tc.deviceGuess)

				t.Logf("[%s] tcpFP=%+v httpFP=%+v\n", tc.name, got.tcpFP, got.httpFP)
			})
		}
	})
}
