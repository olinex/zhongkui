package samples

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"sort"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
	"zhongkui.com/zhongkui/internal/pkg/hash"
)

type flowInfo struct {
	streamID     string
	packetCount  int
	synCount     int
	dataBytes    int
	expectedHTTP int
	srcPort      uint16
	dstPort      uint16
}

func TestAnalyzePcap(t *testing.T) {
	f, err := os.Open("diagnostic_http.cap")
	if err != nil {
		t.Skipf("diagnostic_http.cap not found: %v", err)
		return
	}
	f.Close()

	handle, err := pcap.OpenOffline("diagnostic_http.cap")
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	flows := make(map[string]*flowInfo)
	type dirData struct {
		c2s [][]byte
		s2c [][]byte
	}
	flowPayloads := make(map[string]*dirData)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	for packet := range packetSource.Packets() {
		if packet == nil {
			continue
		}
		nl := packet.NetworkLayer()
		tl := packet.TransportLayer()
		if nl == nil || tl == nil {
			continue
		}
		tcp, ok := tl.(*layers.TCP)
		if !ok {
			continue
		}

		sid := computeStreamIDFromGopacket(nl.NetworkFlow(), tcp)
		fi, ok := flows[sid]
		if !ok {
			fi = &flowInfo{streamID: sid, srcPort: uint16(tcp.SrcPort), dstPort: uint16(tcp.DstPort)}
			flows[sid] = fi
			flowPayloads[sid] = &dirData{}
		}
		fi.packetCount++
		fi.dataBytes += len(tcp.Payload)
		if tcp.SYN && !tcp.ACK {
			fi.synCount++
		}

		payload := append([]byte(nil), tcp.Payload...)
		dd := flowPayloads[sid]

		// Simple direction heuristic: the side that sent the first SYN is client
		// For now, just store by src

		// Split by src/dst
		if fi.srcPort == uint16(tcp.SrcPort) {
			dd.c2s = append(dd.c2s, payload)
		} else {
			dd.s2c = append(dd.s2c, payload)
		}
	}

	// Count HTTP per flow
	for sid, dd := range flowPayloads {
		fi := flows[sid]
		c2sData := concatPayloads(dd.c2s)
		s2cData := concatPayloads(dd.s2c)
		fi.expectedHTTP = countHTTPExchanges(c2sData, s2cData)
	}

	// Sort by stream ID
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	totalPkts := 0
	totalHTTP := 0
	for _, id := range ids {
		fi := flows[id]
		totalPkts += fi.packetCount
		totalHTTP += fi.expectedHTTP
	}

	report := "\n=== Independent Pcap Analysis ===\n"
	report += fmt.Sprintf("Total TCP flows: %d\n", len(flows))
	report += fmt.Sprintf("Total TCP packets: %d\n", totalPkts)
	report += fmt.Sprintf("Total expected HTTP: %d\n\n", totalHTTP)
	for _, id := range ids {
		fi := flows[id]
		report += fmt.Sprintf("  Flow %s: %d pkts, %d bytes → %d HTTP [%d→%d]\n",
			fi.streamID[:16], fi.packetCount, fi.dataBytes, fi.expectedHTTP, fi.srcPort, fi.dstPort)
	}
	t.Log(report)
	t.Logf("INDEPENDENT TOTAL: %d flows, %d HTTP expected", len(flows), totalHTTP)
}

func computeStreamIDFromGopacket(nf gopacket.Flow, tcp *layers.TCP) string {
	srcEndpoint := nf.Src()
	dstEndpoint := nf.Dst()
	leftIP := srcEndpoint.Raw()
	rightIP := dstEndpoint.Raw()
	leftPort := uint16(tcp.SrcPort)
	rightPort := uint16(tcp.DstPort)

	if leftPort > rightPort {
		leftPort, rightPort = rightPort, leftPort
	}
	leftGreater := false
	for i := 0; i < len(leftIP) && i < len(rightIP); i++ {
		if leftIP[i] > rightIP[i] {
			leftGreater = true
			break
		} else if leftIP[i] < rightIP[i] {
			break
		}
	}
	if leftGreater {
		leftIP, rightIP = rightIP, leftIP
	}

	h := hash.Acquire()
	defer h.Release()
	h.WriteUint16(leftPort)
	h.WriteUint16(rightPort)
	h.WriteBytes(leftIP)
	h.WriteBytes(rightIP)
	return h.SumHex()
}

func concatPayloads(payloads [][]byte) []byte {
	var buf bytes.Buffer
	for _, p := range payloads {
		buf.Write(p)
	}
	return buf.Bytes()
}

func countHTTPExchanges(c2sData, s2cData []byte) int {
	if len(c2sData) == 0 || len(s2cData) == 0 {
		return 0
	}
	clientReader := bufio.NewReader(bytes.NewReader(c2sData))
	serverReader := bufio.NewReader(bytes.NewReader(s2cData))
	count := 0
	for {
		req, reqErr := http.ReadRequest(clientReader)
		if reqErr != nil {
			break
		}
		if req.Body != nil {
			req.Body.Close()
		}
		rsp, rspErr := http.ReadResponse(serverReader, req)
		if rspErr != nil {
			break
		}
		if rsp.Body != nil {
			rsp.Body.Close()
		}
		count++
	}
	return count
}
