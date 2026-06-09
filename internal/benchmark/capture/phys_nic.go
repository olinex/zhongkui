//go:build linux

package bench

import (
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// isPhysicalNic 检查网卡是否为物理设备。
// 判断标准：/sys/class/net/<name>/device 存在 → PCI 设备 → 物理网卡。
// 虚拟网卡（veth、bridge、bond、lo 等）没有此路径。
func isPhysicalNic(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/class/net", name, "device"))
	return err == nil
}

// isWireless 检查网卡是否为无线网卡。
// 判断标准：/sys/class/net/<name>/phy80211 存在 或 uevent 包含 DEVTYPE=wlan。
func isWireless(name string) bool {
	// 快速路径：phy80211 目录
	if _, err := os.Stat(filepath.Join("/sys/class/net", name, "phy80211")); err == nil {
		return true
	}
	// 慢路径：读取 uevent
	data, err := os.ReadFile(filepath.Join("/sys/class/net", name, "uevent"))
	if err == nil && strings.Contains(string(data), "DEVTYPE=wlan") {
		return true
	}
	return false
}

// FindPhysicalEthernetNics 返回所有物理有线网卡名称列表。
// 排除虚拟网卡（veth、bridge 等）、loopback 和无线网卡。
func FindPhysicalEthernetNics() []string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil
	}

	var nics []string
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		if !isPhysicalNic(name) {
			continue
		}
		if isWireless(name) {
			continue
		}
		nics = append(nics, name)
	}
	return nics
}

// ProbeAFXdpSupport 探测指定网卡是否支持 AF_XDP。
// 创建 AF_XDP socket 并尝试 bind，成功表示支持。
// 此探测轻量级，不分配 UMEM，不加载 eBPF 程序。
func ProbeAFXdpSupport(ifaceName string) bool {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return false
	}

	fd, err := unix.Socket(unix.AF_XDP, unix.SOCK_RAW, 0)
	if err != nil {
		return false
	}
	defer unix.Close(fd)

	sa := unix.SockaddrXDP{
		Ifindex:      uint32(iface.Index),
		QueueID:      0,
		SharedUmemFD: 0,
	}
	// bind 失败（如 veth 返回 EPROTONOSUPPORT）表示不支持 AF_XDP
	return unix.Bind(fd, &sa) == nil
}

// FindXdpCapableNics 返回最多 n 个支持 AF_XDP 的物理有线网卡。
// 先找出物理网卡，再逐个探测 AF_XDP 支持。
func FindXdpCapableNics(n int) []string {
	nics := FindPhysicalEthernetNics()
	if len(nics) == 0 {
		return nil
	}

	var capable []string
	for _, nic := range nics {
		if ProbeAFXdpSupport(nic) {
			capable = append(capable, nic)
			if len(capable) >= n {
				break
			}
		}
	}
	return capable
}
