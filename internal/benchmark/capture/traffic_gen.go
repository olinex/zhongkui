package bench

import (
	_ "embed"
)

//go:generate sh -c "curl -sL -o testdata/smallFlows.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/smallFlows.pcap && echo 'smallFlows.pcap downloaded' || echo 'WARNING: failed to download smallFlows.pcap'"
//go:generate sh -c "curl -sL -o testdata/bigFlows.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/bigFlows.pcap && echo 'bigFlows.pcap downloaded' || echo 'WARNING: failed to download bigFlows.pcap'"
//go:generate sh -c "curl -sL -o testdata/test.pcap https://s3.amazonaws.com/tcpreplay-pcap-files/test.pcap && echo 'test.pcap downloaded' || echo 'WARNING: failed to download test.pcap'"
//go:generate go run ../../../../cmd/gen_fallback_pcap/main.go testdata/fallback.pcap 10000

//go:embed testdata/test.pcap
var TestPcapData []byte
