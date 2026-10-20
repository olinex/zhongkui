// Package cmd 实现命令行入口与各子命令：构建根命令、注册 create / run / migrate 等子命令，
// 并负责参数解析、调用内部服务与终端输出。
//
// 本文是一篇从零开始的教程：假设你已拿到源码仓库，按顺序完成依赖准备、配置、初始化与启动，
// 最终在管理端看到一个可用的（内容为空的）风险列表，即表示采集、识别、存储与展示整条链路已经打通。
//
// 所有命令都经同一入口 cmd/zki/main.go 暴露。下文用 `go run cmd/zki/main.go {子命令}`
// 表示未安装二进制时的执行方式，它等价于 `make run ARGS="{子命令}"`；执行 `make build`
// 后即可改用生成的 `bin/zki` 直接替换该前缀。
//
// ## 1. 准备工具链
//
// 先确认本机 Go 版本与 go.mod 的 `go` 指令一致。随后安装项目脚本工具与抓包所需的系统依赖：
//
//   - `make dev-tools`：安装开发辅助工具（asynq、pkgsite、goconvey 等）。
//   - `make pcap-tools`：安装采集侧依赖（pcap 相关系统库）；只有需要本地跑 `run capture` 时才必须。
//
// ## 2. 启动依赖容器
//
// zhongkui 依赖 PostgreSQL（core 库）、ClickHouse（raw 库）、Redis、etcd 与 S3 兼容对象存储：
//
//   - `make pull-dev-deps`：拉取上述依赖镜像。
//   - `make run-dev-deps`：以 host 网络启动同名容器（PostgreSQL 口令为 `root_password`）。
//   - 调试结束后用 `make stop-dev-deps` 停止，`make remove-dev-deps` 一并删除容器。
//
// ## 3. 准备配置
//
// 程序通过 Viper 读取名为 `config` 的 TOML 文件，搜索路径依次为 `/etc/zhongkui`、当前目录与上级目录；
// 也可用全局 `--config` / `-c` 直接指定文件路径。关键在于各依赖段要与本地容器对齐：
//
//   - `[api]`：管理端 HTTP 服务的监听地址与端口。
//   - `[database.core]` 与 `[database.raw]`：分别指向 PostgreSQL 与 ClickHouse。
//   - `[redis.cache]`、`[redis.pipeline]`：缓存与任务队列，须指向容器暴露的 Redis。
//   - `[etcd]`：规则与配置变更广播。
//   - `[s3]`：抓包快照的对象存储。
//   - `[grpc.server.reporter]` 与 `[grpc.client.reporter]`：采集端与 Reporter 服务之间的通信。
//
// 接口语言由环境变量 `ZKI_LANG` 控制（`zh-Hans` 或 `en`，默认 `zh-Hans`）。
// 可用 `describe config` 打印全部配置项及其说明（说明即配置项的 `comment` tag），
// 用 `describe deps` 探测核心外部依赖的连通性，避免带着错误配置启动。
//
// ## 4. 初始化数据库与管理员
//
//   - `go run cmd/zki/main.go migrate up`：执行数据库迁移（默认 `--db all`，同时迁移 core 与 raw 两库）。
//   - `go run cmd/zki/main.go create user`：交互式创建管理员账号，可用 `-u/--username`、`-p/--password`、
//     `-e/--email`、`-n/--nicklename` 直接给定字段，并选择是否为超级管理员。
//
// 迁移与建号只需执行一次。
//
// ## 5. 启动服务
//
// 至少需要两个进程（各开一个终端）：
//
//  1. `go run cmd/zki/main.go run web`：启动管理端 HTTP 服务与健康检查端点。
//  2. `go run cmd/zki/main.go run worker`：启动后台任务消费者与各处理器（数据源重载、标签引擎、
//     流水线、选举与各类 asynq 处理器）。
//
// 若同时要采集真实流量，另开终端运行 `go run cmd/zki/main.go run capture`；
// 若需独立部署 Reporter gRPC 服务，可运行 `go run cmd/zki/main.go run grpc`。
//
// ## 6. 验证
//
// 打开管理端（`[api]` 配置的地址与端口），用上一步创建的管理员账号登录。
// 全新部署且尚未导入流量时，风险列表为空属于预期结果——这恰好证明服务已就绪：
// 数据库已迁移、后台任务已在消费、鉴权与接口均正常。随后即可导入流量或启用采集，
// 观察标签与风险内容逐步出现。
//
// 需要排查时可用 `go run cmd/zki/main.go version` 确认版本，用 `describe config` / `describe deps`
// 复核配置与依赖状态。
package cmd

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
	"zhongkuisec.com/zhongkui/internal/provider"
)

// TransactionFixture 表示 TCP 流中的一个 HTTP 事务（请求-响应对）。
// 对于 HTTP/1.1 keep-alive 连接，一个 TCP 流可能包含多个事务。
type TransactionFixture struct {
	// StreamID 所属 TCP 流 ID
	StreamID int `json:"stream_id"`
	// TransactionID 流内事务序号，从 0 开始
	TransactionID int `json:"transaction_id"`
	// ClientAddr 客户端 IP 地址
	ClientAddr string `json:"client_addr"`
	// ClientPort 客户端端口号
	ClientPort int `json:"client_port"`
	// ServerAddr 服务端 IP 地址
	ServerAddr string `json:"server_addr"`
	// ServerPort 服务端端口号
	ServerPort int `json:"server_port"`

	// ExpectedRequestHex 期望的 HTTP 请求原始字节（十六进制编码）
	ExpectedRequestHex string `json:"expected_request_hex"`

	// ExpectedResponseHex 期望的 HTTP 响应原始字节（十六进制编码）
	ExpectedResponseHex string `json:"expected_response_hex"`
}

// PacketRecord 表示单个 TCP 包的记录。
type PacketRecord struct {
	// Frame 报文在 pcap 中的帧号
	Frame int `json:"frame"`
	// Direction 报文方向，"c2s" 或 "s2c"
	Direction string `json:"direction"` // "c2s" 或 "s2c"
	// SeqRaw 序列号原始值（未做相对化处理）
	SeqRaw uint32 `json:"seq_raw"`
	// AckRaw 确认号原始值（未做相对化处理）
	AckRaw uint32 `json:"ack_raw"`
	// Flags TCP 标志位列表，如 SYN、ACK、FIN、RST
	Flags []string `json:"flags"`
	// PayloadHex 载荷的十六进制编码
	PayloadHex string `json:"payload_hex"`
}

// streamMeta 保存 TCP 流的元数据。
type streamMeta struct {
	ClientAddr string
	ClientPort int
	ServerAddr string
	ServerPort int
}

var generateTestcaseCmd = &cobra.Command{
	Use:   "testcase",
	Short: "cmd.create.testcase.short",
	Long:  "cmd.create.testcase.long",
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)

		pcapPath := v.GetString("pcap")
		outputDir := v.GetString("output")

		if err := generateTestcase(pcapPath, outputDir); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Println(translate.T("cli.testcase.generated", map[string]any{"Dir": outputDir}))
	},
}

func init() {
	flags := generateTestcaseCmd.Flags()
	flags.StringP("pcap", "p", "", "flag.create.testcase.pcap")
	flags.StringP("output", "o", "./testcases", "flag.create.testcase.output")
	generateTestcaseCmd.MarkFlagRequired("pcap")
}

// generateTestcase 使用 tshark 从 pcap 文件中提取 HTTP 数据。
func generateTestcase(pcapPath, outputDir string) error {
	// 确认 tshark 可用
	if _, err := exec.LookPath("tshark"); err != nil {
		return fmt.Errorf("tshark not found in PATH: %w", err)
	}

	// 确认 pcap 文件存在
	if _, err := os.Stat(pcapPath); os.IsNotExist(err) {
		return fmt.Errorf("pcap file not found: %s", pcapPath)
	}

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 列出所有含 HTTP 流量的 TCP 流
	streams, err := listHTTPStreams(pcapPath)
	if err != nil {
		return fmt.Errorf("failed to list HTTP streams: %w", err)
	}
	if len(streams) == 0 {
		pterm.Warning.Println(translate.T("cli.testcase.no-http", nil))
		return nil
	}
	pterm.Info.Println(translate.T("cli.testcase.found-streams", map[string]any{"Count": len(streams)}))

	totalTransactions := 0
	for _, streamID := range streams {
		pterm.Info.Println(translate.T("cli.testcase.processing-stream", map[string]any{"Stream": streamID}))
		transactions, err := extractStreamTransactions(pcapPath, streamID)
		if err != nil {
			pterm.Warning.Println(translate.T("cli.testcase.process-failed", map[string]any{"Stream": streamID, "Err": err}))
			continue
		}

		for _, tx := range transactions {
			outputPath := filepath.Join(outputDir,
				fmt.Sprintf("stream_%d_tx_%d.json", streamID, tx.TransactionID))
			if err := writeTransactionFixture(&tx, outputPath); err != nil {
				return fmt.Errorf("failed to write test case file: %w", err)
			}
			pterm.Success.Println(translate.T("cli.testcase.generated-file", map[string]any{"Path": outputPath}))
			totalTransactions++
		}
	}

	pterm.Info.Println(translate.T("cli.testcase.total", map[string]any{"Tx": totalTransactions, "Streams": len(streams)}))
	return nil
}

// listHTTPStreams 返回所有含 HTTP 流量的 TCP 流 ID。
func listHTTPStreams(pcapPath string) ([]int, error) {
	cmd := exec.Command("tshark",
		"-r", pcapPath,
		"-Y", "http",
		"-T", "fields",
		"-e", "tcp.stream",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tshark execution failed: %w", err)
	}

	seen := make(map[int]bool)
	var streams []int
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			continue
		}
		if !seen[id] {
			seen[id] = true
			streams = append(streams, id)
		}
	}
	sort.Ints(streams)
	return streams, nil
}

// extractStreamTransactions 从一条 TCP 流中提取所有 HTTP 事务。
func extractStreamTransactions(pcapPath string, streamID int) ([]TransactionFixture, error) {
	// 提取流元数据（客户端/服务端 IP 与端口）
	meta, err := extractStreamMeta(pcapPath, streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract stream metadata: %w", err)
	}

	// 提取该流的全部 TCP 包
	packets, err := extractPacketRecords(pcapPath, streamID, meta)
	if err != nil {
		return nil, fmt.Errorf("failed to extract TCP packets: %w", err)
	}

	// 拼接同方向载荷并切分为 HTTP 事务
	return splitHTTPTransactions(streamID, meta, packets)
}

// extractStreamMeta 从首个 HTTP 请求包中提取客户端/服务端地址信息。
func extractStreamMeta(pcapPath string, streamID int) (*streamMeta, error) {
	streamStr := fmt.Sprintf("tcp.stream==%d and http.request", streamID)
	cmd := exec.Command("tshark",
		"-r", pcapPath,
		"-Y", streamStr,
		"-T", "fields",
		"-e", "ip.src",
		"-e", "ip.dst",
		"-e", "tcp.srcport",
		"-e", "tcp.dstport",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tshark execution failed: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("unable to get metadata for stream %d", streamID)
	}

	fields := strings.Split(strings.TrimSpace(lines[0]), "\t")
	if len(fields) < 4 {
		return nil, fmt.Errorf("insufficient metadata fields: %v", fields)
	}

	meta := &streamMeta{
		ClientAddr: fields[0],
		ServerAddr: fields[1],
	}
	meta.ClientPort, _ = strconv.Atoi(fields[2])
	meta.ServerPort, _ = strconv.Atoi(fields[3])

	return meta, nil
}

// extractPacketRecords 提取指定流的全部 TCP 包信息。
func extractPacketRecords(pcapPath string, streamID int, meta *streamMeta) ([]PacketRecord, error) {
	streamStr := fmt.Sprintf("tcp.stream==%d", streamID)
	cmd := exec.Command("tshark",
		"-r", pcapPath,
		"-Y", streamStr,
		"-T", "fields",
		"-e", "frame.number",
		"-e", "tcp.srcport",
		"-e", "tcp.seq_raw",
		"-e", "tcp.ack_raw",
		"-e", "tcp.flags.syn",
		"-e", "tcp.flags.ack",
		"-e", "tcp.flags.fin",
		"-e", "tcp.flags.reset",
		"-e", "tcp.payload",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tshark execution failed: %w", err)
	}

	clientPortStr := strconv.Itoa(meta.ClientPort)
	var packets []PacketRecord

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")

		packet := PacketRecord{}
		if len(fields) > 0 {
			packet.Frame, _ = strconv.Atoi(fields[0])
		}
		// 用源端口判定方向——对客户端与服务端共用同一 IP 的 localhost 流量同样正确
		if len(fields) > 1 {
			if fields[1] == clientPortStr {
				packet.Direction = "c2s"
			} else {
				packet.Direction = "s2c"
			}
		}
		if len(fields) > 2 {
			v, _ := strconv.ParseUint(fields[2], 10, 32)
			packet.SeqRaw = uint32(v)
		}
		if len(fields) > 3 {
			v, _ := strconv.ParseUint(fields[3], 10, 32)
			packet.AckRaw = uint32(v)
		}

		// 解析标志位
		packet.Flags = make([]string, 0)
		if len(fields) > 4 && fields[4] == "1" {
			packet.Flags = append(packet.Flags, "SYN")
		}
		if len(fields) > 5 && fields[5] == "1" {
			packet.Flags = append(packet.Flags, "ACK")
		}
		if len(fields) > 6 && fields[6] == "1" {
			packet.Flags = append(packet.Flags, "FIN")
		}
		if len(fields) > 7 && fields[7] == "1" {
			packet.Flags = append(packet.Flags, "RST")
		}

		if len(fields) > 8 {
			packet.PayloadHex = fields[8]
		}

		packets = append(packets, packet)
	}

	return packets, nil
}

// splitHTTPTransactions 拼接同方向的 TCP 载荷，并用 Go 的 net/http 解析器
// 将其切分为一个个 HTTP 请求-响应对。
func splitHTTPTransactions(streamID int, meta *streamMeta, packets []PacketRecord) ([]TransactionFixture, error) {
	// 分别拼接全部 c2s 与 s2c 载荷
	var reqPayloads, rspPayloads []string
	for _, pkt := range packets {
		if pkt.PayloadHex == "" {
			continue
		}
		if pkt.Direction == "c2s" {
			reqPayloads = append(reqPayloads, pkt.PayloadHex)
		} else {
			rspPayloads = append(rspPayloads, pkt.PayloadHex)
		}
	}

	fullClientData, err := hexDecode(joinHexPayloads(reqPayloads))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	fullServerData, err := hexDecode(joinHexPayloads(rspPayloads))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if len(fullClientData) == 0 {
		return nil, nil
	}

	clientReader := bufio.NewReader(bytes.NewReader(fullClientData))
	serverReader := bufio.NewReader(bytes.NewReader(fullServerData))

	var transactions []TransactionFixture
	txID := 0

	for {
		// 尝试从客户端数据流读取一个 HTTP 请求
		req, reqRaw, err := readHTTPRequest(clientReader)
		if err == io.EOF {
			break
		}
		if err != nil {
			// 仍有数据却无法解析出请求时，记录告警并停止
			pterm.Warning.Println(translate.T("cli.testcase.parse-request-failed", map[string]any{"Stream": streamID, "Tx": txID, "Err": err}))
			break
		}

		// 尝试读取对应的 HTTP 响应
		_, respRaw, err := readHTTPResponse(serverReader, req)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// 响应不完整或缺失——仍单独记录该请求
			pterm.Warning.Println(translate.T("cli.testcase.response-missing", map[string]any{"Stream": streamID, "Tx": txID, "Err": err}))
		}
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			pterm.Warning.Println(translate.T("cli.testcase.parse-response-failed", map[string]any{"Stream": streamID, "Tx": txID, "Err": err}))
		}

		tx := TransactionFixture{
			StreamID:            streamID,
			TransactionID:       txID,
			ClientAddr:          meta.ClientAddr,
			ClientPort:          meta.ClientPort,
			ServerAddr:          meta.ServerAddr,
			ServerPort:          meta.ServerPort,
			ExpectedRequestHex:  hexEncode(reqRaw),
			ExpectedResponseHex: hexEncode(respRaw),
		}
		transactions = append(transactions, tx)
		txID++
	}

	return transactions, nil
}

// readHTTPRequest 从 reader 读取一个 HTTP 请求，返回解析后的请求
// 及其原始线格式字节。
func readHTTPRequest(r *bufio.Reader) (*http.Request, []byte, error) {
	// 预先窥探是否有数据
	if _, err := r.Peek(1); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// 使用 net/http 读取请求
	req, err := http.ReadRequest(r)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// 经 Request.Write 重建原始线格式字节
	var buf bytes.Buffer
	if err := req.Write(&buf); err != nil {
		return req, nil, errors.WithStack(err)
	}

	return req, buf.Bytes(), nil
}

// readHTTPResponse 以给定请求为上下文，从 reader 读取一个 HTTP 响应。
func readHTTPResponse(r *bufio.Reader, req *http.Request) (*http.Response, []byte, error) {
	if _, err := r.Peek(1); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	var buf bytes.Buffer
	if err := resp.Write(&buf); err != nil {
		return resp, nil, errors.WithStack(err)
	}

	return resp, buf.Bytes(), nil
}

// joinHexPayloads 把多个十六进制编码的载荷拼接为单个十六进制字符串。
func joinHexPayloads(payloads []string) string {
	var builder strings.Builder
	for _, p := range payloads {
		// tshark 输出的十六进制可能带冒号分隔符（如 "aa:bb:cc"）
		p = strings.ReplaceAll(p, ":", "")
		builder.WriteString(p)
	}
	return builder.String()
}

// hexDecode 把十六进制字符串解码为原始字节。
func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// hexEncode 把原始字节编码为十六进制字符串。
func hexEncode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// writeTransactionFixture 把 TransactionFixture 序列化为 JSON 文件。
func writeTransactionFixture(fixture *TransactionFixture, path string) error {
	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON serialization failed: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

// HexDecode 把十六进制字符串解码为原始字节（供测试代码使用的便捷函数）。
func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
