package detectors

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

// updateFingerprintFixtures 由 -update 标志控制：为 true 时重新生成 testdata/fp 下的静态 pcap 夹具。
// 运行方式: go test ./internal/network/detectors -run TestGenerateFingerprintPcapFixtures -update
var updateFingerprintFixtures = flag.Bool("update", false, "regenerate fingerprint pcap fixtures under testdata/fp")

// testdataFpDir 返回指纹 pcap 夹具目录（相对当前包）。
func testdataFpDir() string {
	return filepath.Join("testdata", "fp")
}

// fingerprintFixtureSpec 描述一个待生成的 TCP 会话 pcap 夹具（单条 HTTP 事务）。
type fingerprintFixtureSpec struct {
	// name 用例名称（与 fingerprintCases 的 name 对应）
	name string
	// file 输出文件名（含后缀，如 .pcap）
	file string
	// steps 按发送顺序排列的 TCP 包步骤
	steps []tcpStep
}

// tcpStep 描述 TCP 会话中的一个数据包。
type tcpStep struct {
	// c2s true 表示客户端→服务端方向，false 表示服务端→客户端方向
	c2s bool
	// syn / ack / fin TCP 控制标志
	syn, ack, fin bool
	// ttl IP 生存时间（p0f 签名的重要维度）
	ttl uint8
	// window 通告窗口大小
	window uint16
	// options TCP 选项列表（决定 p0f 的 olayout 维度）
	options []layers.TCPOption
	// payload 应用层载荷（HTTP 请求/响应头字节）
	payload []byte
	// seq / ack TCP 序号与确认号
	seq, ackNum uint32
}

// 夹具两侧端点固定，保证同一会话内方向判断与端口稳定。
var (
	fpClientIP   = net.ParseIP("192.168.1.10")
	fpServerIP   = net.ParseIP("93.184.216.34")
	fpClientPort = layers.TCPPort(51234)
	fpServerPort = layers.TCPPort(80)
)

// tcpOpt 构造一个 TCP 选项。
func tcpOpt(kind layers.TCPOptionKind, data []byte) layers.TCPOption {
	return layers.TCPOption{OptionType: kind, OptionLength: uint8(len(data) + 2), OptionData: data}
}

// mssOpt 构造 MSS 选项。
func mssOpt(mss uint16) layers.TCPOption {
	return tcpOpt(layers.TCPOptionKindMSS, []byte{byte(mss >> 8), byte(mss)})
}

// 已知可被 p0f 签名库命中的 TCP 选项组合（与 huginn-net-ffi 单测 / p0f.fp 签名对应）：
//   - linuxClientOptions: 对应 *:64:0:*:mss*10,6:mss,sok,ts,nop,ws:df,id+:0 → Linux 3.1-3.10
//   - windowsClientOptions: 对应 *:128:0:*:65535,8:mss,nop,ws,nop,nop,sok:df,id+:0 → Windows 10
var (
	linuxClientOptions = []layers.TCPOption{
		mssOpt(1460),
		tcpOpt(layers.TCPOptionKindSACKPermitted, nil),
		tcpOpt(layers.TCPOptionKindTimestamps, []byte{0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0}),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindWindowScale, []byte{6}),
	}
	linuxServerOptions = []layers.TCPOption{
		mssOpt(1460),
		tcpOpt(layers.TCPOptionKindSACKPermitted, nil),
		tcpOpt(layers.TCPOptionKindTimestamps, []byte{0x0A, 0x0B, 0x0C, 0x0D, 0, 0, 0, 0}),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindWindowScale, []byte{7}),
	}
	// 真实 Windows 10 的 SYN 选项序为 mss,nop,ws,nop,nop,sok（对应 p0f.fp s:win:Windows:10）
	windowsClientOptions = []layers.TCPOption{
		mssOpt(1460),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindWindowScale, []byte{8}),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindSACKPermitted, nil),
	}
	windowsServerOptions = []layers.TCPOption{
		mssOpt(1460),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindSACKPermitted, nil),
		tcpOpt(layers.TCPOptionKindTimestamps, []byte{0x1A, 0x1B, 0x1C, 0x1D, 0, 0, 0, 0}),
		tcpOpt(layers.TCPOptionKindNop, nil),
		tcpOpt(layers.TCPOptionKindWindowScale, []byte{8}),
	}
)

// 夹具内的 HTTP 请求/响应头字节（指纹识别只消费头部，与 body 无关）。
const (
	firefoxRequest = "GET /index.html HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0\r\n" +
		"Accept-Language: en-US,en;q=0.5\r\n" +
		"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\n" +
		"Connection: keep-alive\r\n\r\n"
	nginxResponse = "HTTP/1.1 200 OK\r\n" +
		"Server: nginx/1.18.0\r\n" +
		"Content-Type: text/html\r\n" +
		"Content-Length: 10\r\n\r\n" +
		"hello world"
	chromeRequest = "GET / HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Connection: keep-alive\r\n" +
		"Upgrade-Insecure-Requests: 1\r\n" +
		"User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36\r\n" +
		"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8\r\n" +
		"Accept-Encoding: gzip, deflate, br\r\n" +
		"Accept-Language: zh-CN,zh;q=0.9,en;q=0.8\r\n\r\n"
	iisResponse = "HTTP/1.1 200 OK\r\n" +
		"Server: Microsoft-IIS/7.5\r\n" +
		"Date: Mon, 01 Jan 2024 00:00:00 GMT\r\n" +
		"Content-Type: text/html\r\n" +
		"Content-Length: 10\r\n\r\n" +
		"hello world"
)

// fingerprintFixtureSpecs 定义全部指纹 pcap 夹具。
func fingerprintFixtureSpecs() []fingerprintFixtureSpec {
	return []fingerprintFixtureSpec{
		{
			// Linux 客户端（Firefox）+ nginx 服务端：期望识别浏览器且设备类别为 desktop
			name: "fp_linux_firefox_nginx",
			file: "fp_linux_firefox_nginx.pcap",
			steps: []tcpStep{
				{c2s: true, syn: true, ttl: 64, window: 14600, options: linuxClientOptions, seq: 1000},
				{c2s: false, syn: true, ack: true, ttl: 64, window: 29200, options: linuxServerOptions, seq: 2000, ackNum: 1001},
				{c2s: true, ack: true, ttl: 64, window: 14600, options: linuxClientOptions, seq: 1001, ackNum: 2001},
				{c2s: true, ack: true, ttl: 64, window: 14600, options: linuxClientOptions, seq: 1001, ackNum: 2001, payload: []byte(firefoxRequest)},
				{c2s: false, ack: true, ttl: 64, window: 29200, options: linuxServerOptions, seq: 2001, ackNum: 1001 + uint32(len(firefoxRequest)), payload: []byte(nginxResponse)},
				{c2s: false, fin: true, ack: true, ttl: 64, window: 29200, options: linuxServerOptions, seq: 2001 + uint32(len(nginxResponse)), ackNum: 1001 + uint32(len(firefoxRequest))},
				{c2s: true, fin: true, ack: true, ttl: 64, window: 14600, options: linuxClientOptions, seq: 1001 + uint32(len(firefoxRequest)), ackNum: 2002 + uint32(len(nginxResponse))},
			},
		},
		{
			// Windows 客户端（Chrome）+ IIS 服务端：复现用户报告中浏览器识别缺失的场景
			name: "fp_windows_chrome_iis",
			file: "fp_windows_chrome_iis.pcap",
			steps: []tcpStep{
				{c2s: true, syn: true, ttl: 128, window: 65535, options: windowsClientOptions, seq: 4000},
				{c2s: false, syn: true, ack: true, ttl: 128, window: 65535, options: windowsServerOptions, seq: 5000, ackNum: 4001},
				{c2s: true, ack: true, ttl: 128, window: 65535, options: windowsClientOptions, seq: 4001, ackNum: 5001},
				{c2s: true, ack: true, ttl: 128, window: 65535, options: windowsClientOptions, seq: 4001, ackNum: 5001, payload: []byte(chromeRequest)},
				{c2s: false, ack: true, ttl: 128, window: 65535, options: windowsServerOptions, seq: 5001, ackNum: 4001 + uint32(len(chromeRequest)), payload: []byte(iisResponse)},
			},
		},
	}
}

// buildTCPPacket 序列化一个以太网 + IPv4 + TCP 数据包字节序列。
// IP 头固定 DF 置位、标识 0x1234（对应 p0f df,id+ 特征），TTL 与 TCP 特征由 step 决定。
func buildTCPPacket(step tcpStep) []byte {
	srcIP, dstIP := fpClientIP, fpServerIP
	srcPort, dstPort := fpClientPort, fpServerPort
	if !step.c2s {
		srcIP, dstIP = fpServerIP, fpClientIP
		srcPort, dstPort = fpServerPort, fpClientPort
	}
	eth := layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      step.ttl,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    srcIP,
		DstIP:    dstIP,
		Id:       0x1234,
		Flags:    layers.IPv4DontFragment,
	}
	tcp := layers.TCP{
		SrcPort: srcPort,
		DstPort: dstPort,
		Seq:     step.seq,
		Ack:     step.ackNum,
		SYN:     step.syn,
		ACK:     step.ack,
		FIN:     step.fin,
		Window:  step.window,
		Options: step.options,
	}
	if err := tcp.SetNetworkLayerForChecksum(&ip); err != nil {
		panic(err)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, &eth, &ip, &tcp, gopacket.Payload(step.payload)); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// TestGenerateFingerprintPcapFixtures 重新生成 testdata/fp 下的静态 pcap 夹具。
// 夹具为合成流量，保证 TCP 选项与 HTTP 头可被 huginn 指纹库命中；
// 也可将真实浏览器抓包放入同一目录并加入 fingerprintCases 期望清单来扩充覆盖。
// 运行方式: go test ./internal/network/detectors -run TestGenerateFingerprintPcapFixtures -update
func TestGenerateFingerprintPcapFixtures(t *testing.T) {
	if !*updateFingerprintFixtures {
		t.Skip("use -update to regenerate fingerprint pcap fixtures")
	}
	dir := testdataFpDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Truncate(time.Second)
	for _, spec := range fingerprintFixtureSpecs() {
		path := filepath.Join(dir, spec.file)
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		writer := pcapgo.NewWriter(f)
		if err := writer.WriteFileHeader(65535, layers.LinkTypeEthernet); err != nil {
			f.Close()
			t.Fatalf("write header %s: %v", path, err)
		}
		for i, step := range spec.steps {
			if err := writer.WritePacket(gopacket.CaptureInfo{
				Timestamp:     ts.Add(time.Duration(i) * time.Millisecond),
				CaptureLength: len(buildTCPPacket(step)),
				Length:        len(buildTCPPacket(step)),
			}, buildTCPPacket(step)); err != nil {
				f.Close()
				t.Fatalf("write packet %d of %s: %v", i, path, err)
			}
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
		t.Logf("generated %s (%d packets)", path, len(spec.steps))
	}
}
