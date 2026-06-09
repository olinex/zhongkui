//go:build linux

package main

import (
	"fmt"
	"os"

	"zhongkui.com/zhongkui/internal/benchmark/capture"
)

func main() {
	if os.Getuid() != 0 {
		fmt.Println("需要 sudo 运行")
		os.Exit(1)
	}

	fmt.Println("=== 物理有线网卡 ===")
	phys := bench.FindPhysicalEthernetNics()
	fmt.Printf("找到 %d 张:\n", len(phys))
	for _, nic := range phys {
		fmt.Printf("  - %s\n", nic)
	}

	fmt.Println("\n=== AF_XDP 探测 ===")
	for _, nic := range phys {
		ok := bench.ProbeAFXdpSupport(nic)
		fmt.Printf("  %s: AF_XDP = %v\n", nic, ok)
	}

	fmt.Println("\n=== FindXdpCapableNics(2) ===")
	nics := bench.FindXdpCapableNics(2)
	fmt.Printf("找到 %d 张 XDP 可用:\n", len(nics))
	for _, nic := range nics {
		fmt.Printf("  - %s\n", nic)
	}
}
