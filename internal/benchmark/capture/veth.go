//go:build linux

package bench

import (
	"fmt"
	"os/exec"
	"sync/atomic"
)

var vethSeq int64

// SetupVethPair 创建 veth pair 用于 XDP 基线测试。
// 返回捕获端网卡名、发包端网卡名、清理函数和错误。
// 调用者应在测试结束时执行 cleanup 函数。
func SetupVethPair() (capIface, sendIface string, cleanup func(), err error) {
	id := atomic.AddInt64(&vethSeq, 1)
	// Linux 网卡名最长为 15 字符 (IFNAMSIZ-1)
	capIface = fmt.Sprintf("zb%d", id)
	sendIface = fmt.Sprintf("zp%d", id)

	// 创建 veth pair
	if out, e := exec.Command("ip", "link", "add", capIface, "type", "veth", "peer", "name", sendIface).CombinedOutput(); e != nil {
		return "", "", nil, fmt.Errorf("failed to create veth pair: %v\n%s", e, out)
	}

	clean := func() {
		exec.Command("ip", "link", "delete", capIface).Run()
	}

	// 启动两端网卡
	if out, e := exec.Command("ip", "link", "set", capIface, "up").CombinedOutput(); e != nil {
		clean()
		return "", "", nil, fmt.Errorf("failed to bring up %s: %v\n%s", capIface, e, out)
	}
	if out, e := exec.Command("ip", "link", "set", sendIface, "up").CombinedOutput(); e != nil {
		clean()
		return "", "", nil, fmt.Errorf("failed to bring up %s: %v\n%s", sendIface, e, out)
	}

	return capIface, sendIface, clean, nil
}
