package tcp

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/models/network"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
)

func TestBase(t *testing.T) {
	Convey("Test extractPackets with IPv4 TCP", t, func() {
		Convey("Should read pcap file without error", func(c C) {
			srcChan := make(chan *network.PacketIPv4TCP)
			wg, ctx := errgroupx.WithContext(context.Background())
			wg.Go(func() error {
				defer close(srcChan)
				return extractPackets(ctx, "testdata/http_base.pcap", srcChan)
			})
			wg.Go(func() error {
				for packet := range srcChan {
					c.So(packet, ShouldNotBeNil)
				}
				return nil
			})
			err := wg.Wait()
			c.So(err, ShouldBeNil)
		})
	})

	Convey("Test runRouter function", t, func() {
		Convey("Should run router without error for base HTTP pcap", func(c C) {
			err := runRouter("testdata/http_base.pcap")
			c.So(err, ShouldBeNil)
		})

		Convey("Should run router without error for gzipped HTTP pcap", func(c C) {
			// 测试Gzip压缩的HTTP pcap文件，添加超时控制
			err := runRouter("testdata/http_gzip.pcap")
			c.So(err, ShouldBeNil)
		})

		Convey("Should run router without error for chunked+gzip HTTP pcap", func(c C) {
			// 测试Chunked+Gzip编码的HTTP pcap文件，添加超时控制
			err := runRouter("testdata/http_chunked_gzip.pcap")
			c.So(err, ShouldBeNil)
		})

		Convey("Should handle non-existent file gracefully", func(c C) {
			// 测试处理不存在的文件
			// 由于runRouter内部会调用pcap.OpenOffline，对于不存在的文件会返回错误
			err := runRouter("testdata/non_existent.pcap")
			c.So(err, ShouldNotBeNil)
		})
	})
}
