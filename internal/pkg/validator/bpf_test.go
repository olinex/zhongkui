package validator

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestIsValidBPF 验证 bpf 标签谓词对常见 tcpdump 表达式的判定。
// 用例只使用数值地址/端口，避免测试依赖环境 DNS。
func TestIsValidBPF(t *testing.T) {
	Convey("Test IsValidBPF", t, func() {
		Convey("合法的 BPF 表达式应通过", func() {
			valid := []string{
				"tcp",
				"tcp port 80",
				"udp port 53",
				"host 192.168.1.1",
				"net 10.0.0.0/8",
				"(tcp or udp) and not port 22",
				"tcp portrange 8000-9000",
				"tcp[13] & 2 != 0",
				"wlan type mgt",
				"wlan ra 00:11:22:33:44:55",
			}
			for _, expr := range valid {
				expr := expr
				Convey(fmt.Sprintf("合法：%q", expr), func() {
					So(IsValidBPF(expr), ShouldBeTrue)
				})
			}
		})

		Convey("非法的 BPF 表达式应被拒绝", func() {
			invalid := []string{
				"",
				"   ",
				"tcp and",
				"tcp port 70000",
				"net 10.0.0.0/33",
				"vlan 4096",
				"(tcp",
				"tcp)",
			}
			for _, expr := range invalid {
				expr := expr
				Convey(fmt.Sprintf("非法：%q", expr), func() {
					So(IsValidBPF(expr), ShouldBeFalse)
				})
			}
		})
	})
}
