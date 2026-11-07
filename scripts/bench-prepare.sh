#!/usr/bin/env bash
# bench-prepare.sh - 下载 tcpreplay 采样流量并生成 fallback pcap
#
# 下载 test.pcap / smallFlows.pcap / bigFlows.pcap 到
# internal/benchmark/capture/testdata/，再用 cmd/template 生成 10,000 包
# 的 fallback.pcap，供抓包实现基线测试使用。
#
# 用法：
#   scripts/bench-prepare.sh
#
# 退出码：0 成功；1 失败。
set -uo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT_DIR" || exit 1

TESTDATA="internal/benchmark/capture/testdata"
mkdir -p "$TESTDATA"

echo "=== 下载 tcpreplay 采样流量 ==="
curl -sL -o "$TESTDATA/test.pcap" \
	https://s3.amazonaws.com/tcpreplay-pcap-files/test.pcap &&
	echo "  test.pcap (141 包)     ✓" ||
	echo "  test.pcap              ✗"

curl -sL -o "$TESTDATA/smallFlows.pcap" \
	https://s3.amazonaws.com/tcpreplay-pcap-files/smallFlows.pcap &&
	echo "  smallFlows.pcap (14k 包) ✓" ||
	echo "  smallFlows.pcap          ✗"

curl -sL -o "$TESTDATA/bigFlows.pcap" \
	https://s3.amazonaws.com/tcpreplay-pcap-files/bigFlows.pcap &&
	echo "  bigFlows.pcap (790k 包)   ✓" ||
	echo "  bigFlows.pcap             ✗"

echo "=== 生成 fallback pcap (10,000 包) ==="
go run cmd/template/main.go gen-fallback-pcap \
	--output "$TESTDATA/fallback.pcap" \
	--numPackets 10000

echo "=== 完成 ==="
ls -lh "$TESTDATA/"
