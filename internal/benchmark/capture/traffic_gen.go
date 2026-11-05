// Package bench 提供抓包链路的基准测试与压测工具：流量生成、回放与性能指标采集。
package bench

import (
	_ "embed"
)

//go:generate sh -c "curl -sL -o testdata/smallFlows.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/smallFlows.pcap && echo 'smallFlows.pcap downloaded' || echo 'WARNING: failed to download smallFlows.pcap'"
//go:generate sh -c "curl -sL -o testdata/bigFlows.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/bigFlows.pcap && echo 'bigFlows.pcap downloaded' || echo 'WARNING: failed to download bigFlows.pcap'"
//go:generate sh -c "curl -sL -o testdata/test.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/test.pcap && echo 'test.pcap downloaded' || echo 'WARNING: failed to download test.pcap'"
//go:generate go run ../../../../cmd/gen_fallback_pcap/main.go testdata/fallback.pcap 10000

// TestPcapData 是内嵌的测试用 pcap 文件（testdata/test.pcap）内容，供基准与回放测试使用。
//
//go:embed testdata/test.pcap
var TestPcapData []byte
