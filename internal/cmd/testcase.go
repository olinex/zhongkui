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
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/provider"
)

// TransactionFixture 表示 TCP 流中的一个 HTTP 事务（请求-响应对）。
// 对于 HTTP/1.1 keep-alive 连接，一个 TCP 流可能包含多个事务。
type TransactionFixture struct {
	StreamID      int    `json:"stream_id"`
	TransactionID int    `json:"transaction_id"`
	ClientAddr    string `json:"client_addr"`
	ClientPort    int    `json:"client_port"`
	ServerAddr    string `json:"server_addr"`
	ServerPort    int    `json:"server_port"`

	// ExpectedRequestHex 期望的 HTTP 请求原始字节（十六进制编码）
	ExpectedRequestHex string `json:"expected_request_hex"`

	// ExpectedResponseHex 期望的 HTTP 响应原始字节（十六进制编码）
	ExpectedResponseHex string `json:"expected_response_hex"`
}

// PacketRecord 表示单个 TCP 包的记录。
type PacketRecord struct {
	Frame      int      `json:"frame"`
	Direction  string   `json:"direction"` // "c2s" 或 "s2c"
	SeqRaw     uint32   `json:"seq_raw"`
	AckRaw     uint32   `json:"ack_raw"`
	Flags      []string `json:"flags"`
	PayloadHex string   `json:"payload_hex"`
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
	Short: "从 pcap 文件中提取 HTTP 测试用例",
	Long: `使用 tshark 从 pcap 文件中提取 HTTP 请求/响应数据，
并生成 JSON 测试用例文件，用于验证 TCP 重组和 HTTP 协议识别逻辑。

每个 HTTP 事务（请求-响应对）生成一个 JSON 文件。
支持 HTTP/1.1 keep-alive 连接中每个 TCP 流包含多个事务的情况。`,
	Run: func(cmd *cobra.Command, args []string) {
		_, i := cmdToolBox(cmd)
		v := provider.MustInvokeViper(i)

		pcapPath := v.GetString("pcap")
		outputDir := v.GetString("output")

		if err := generateTestcase(pcapPath, outputDir); err != nil {
			pterm.Error.Println(err.Error())
			return
		}
		pterm.Success.Printf("Test cases generated at %s\n", outputDir)
	},
}

func init() {
	flags := generateTestcaseCmd.Flags()
	flags.StringP("pcap", "p", "", "pcap 文件路径（必填）")
	flags.StringP("output", "o", "./testcases", "测试用例文件的输出目录")
	generateTestcaseCmd.MarkFlagRequired("pcap")
}

// generateTestcase uses tshark to extract HTTP data from a pcap file.
func generateTestcase(pcapPath, outputDir string) error {
	// Verify tshark is available
	if _, err := exec.LookPath("tshark"); err != nil {
		return fmt.Errorf("tshark not found in PATH: %w", err)
	}

	// Verify pcap file exists
	if _, err := os.Stat(pcapPath); os.IsNotExist(err) {
		return fmt.Errorf("pcap file not found: %s", pcapPath)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// List all TCP streams containing HTTP traffic
	streams, err := listHTTPStreams(pcapPath)
	if err != nil {
		return fmt.Errorf("failed to list HTTP streams: %w", err)
	}
	if len(streams) == 0 {
		pterm.Warning.Println("No HTTP traffic found in pcap file")
		return nil
	}
	pterm.Info.Printf("Found %d HTTP stream(s)\n", len(streams))

	totalTransactions := 0
	for _, streamID := range streams {
		pterm.Info.Printf("Processing TCP stream %d...\n", streamID)
		transactions, err := extractStreamTransactions(pcapPath, streamID)
		if err != nil {
			pterm.Warning.Printf("Failed to process stream %d: %v, skipping\n", streamID, err)
			continue
		}

		for _, tx := range transactions {
			outputPath := filepath.Join(outputDir,
				fmt.Sprintf("stream_%d_tx_%d.json", streamID, tx.TransactionID))
			if err := writeTransactionFixture(&tx, outputPath); err != nil {
				return fmt.Errorf("failed to write test case file: %w", err)
			}
			pterm.Success.Printf("  Generated %s\n", outputPath)
			totalTransactions++
		}
	}

	pterm.Info.Printf("Total: %d HTTP transaction(s) across %d stream(s)\n", totalTransactions, len(streams))
	return nil
}

// listHTTPStreams returns all TCP stream IDs that contain HTTP traffic.
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

// extractStreamTransactions extracts all HTTP transactions from a TCP stream.
func extractStreamTransactions(pcapPath string, streamID int) ([]TransactionFixture, error) {
	// Extract stream metadata (client/server IPs and ports)
	meta, err := extractStreamMeta(pcapPath, streamID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract stream metadata: %w", err)
	}

	// Extract all TCP packets for the stream
	packets, err := extractPacketRecords(pcapPath, streamID, meta)
	if err != nil {
		return nil, fmt.Errorf("failed to extract TCP packets: %w", err)
	}

	// Concatenate same-direction payloads and split into HTTP transactions
	return splitHTTPTransactions(streamID, meta, packets)
}

// extractStreamMeta extracts client/server address info from the first HTTP request packet.
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

// extractPacketRecords extracts all TCP packet info for a given stream.
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
		// Use source port to determine direction — works correctly for localhost traffic
		// where client and server share the same IP
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

		// Parse flags
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

// splitHTTPTransactions concatenates same-direction TCP payloads and splits them
// into individual HTTP request-response pairs using Go's net/http parser.
func splitHTTPTransactions(streamID int, meta *streamMeta, packets []PacketRecord) ([]TransactionFixture, error) {
	// Concatenate all c2s and s2c payloads separately
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
		// Try to read an HTTP request from the client data stream
		req, reqRaw, err := readHTTPRequest(clientReader)
		if err == io.EOF {
			break
		}
		if err != nil {
			// If we can't parse a request but there's still data, log and stop
			pterm.Warning.Printf("  stream %d: failed to parse HTTP request at tx %d: %v\n",
				streamID, txID, err)
			break
		}

		// Try to read the corresponding HTTP response
		_, respRaw, err := readHTTPResponse(serverReader, req)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// Response incomplete or missing — still record the request alone
			pterm.Warning.Printf("  stream %d tx %d: HTTP response missing or incomplete: %v\n",
				streamID, txID, err)
		}
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			pterm.Warning.Printf("  stream %d tx %d: failed to parse HTTP response: %v\n",
				streamID, txID, err)
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

// readHTTPRequest reads one HTTP request from reader and returns the parsed request
// along with its raw wire-format bytes.
func readHTTPRequest(r *bufio.Reader) (*http.Request, []byte, error) {
	// Peek ahead to check if there's data
	if _, err := r.Peek(1); err != nil {
		return nil, nil, err
	}

	// Read the request using net/http
	req, err := http.ReadRequest(r)
	if err != nil {
		return nil, nil, err
	}

	// Reconstruct raw wire-format bytes via Request.Write
	var buf bytes.Buffer
	if err := req.Write(&buf); err != nil {
		return req, nil, errors.WithStack(err)
	}

	return req, buf.Bytes(), nil
}

// readHTTPResponse reads one HTTP response from reader using the given request as context.
func readHTTPResponse(r *bufio.Reader, req *http.Request) (*http.Response, []byte, error) {
	if _, err := r.Peek(1); err != nil {
		return nil, nil, err
	}

	resp, err := http.ReadResponse(r, req)
	if err != nil {
		return nil, nil, err
	}

	var buf bytes.Buffer
	if err := resp.Write(&buf); err != nil {
		return resp, nil, errors.WithStack(err)
	}

	return resp, buf.Bytes(), nil
}

// joinHexPayloads joins multiple hex-encoded payloads into a single hex string.
func joinHexPayloads(payloads []string) string {
	var builder strings.Builder
	for _, p := range payloads {
		// tshark may output hex with colon separators (e.g. "aa:bb:cc")
		p = strings.ReplaceAll(p, ":", "")
		builder.WriteString(p)
	}
	return builder.String()
}

// hexDecode decodes a hex string into raw bytes.
func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// hexEncode encodes raw bytes into a hex string.
func hexEncode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

// writeTransactionFixture serializes a TransactionFixture to a JSON file.
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

// HexDecode decodes a hex string into raw bytes (convenience for test code).
func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
