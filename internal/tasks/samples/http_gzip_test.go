package samples

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/models/network"
	_ "zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/network/tcp"
	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

var (
	httpGzipRequestRaw []byte = []byte(`GET /test/ethereal.html HTTP/1.1
Host: cerberus
User-Agent: Mozilla/5.0 (X11; U; Linux ppc; rv:1.7.3) Gecko/20041004 Firefox/0.10.1
Accept: text/xml,application/xml,application/xhtml+xml,text/html;q=0.9,text/plain;q=0.8,image/png,*/*;q=0.5
Accept-Language: en-us,en;q=0.5
Accept-Encoding: gzip,deflate
Accept-Charset: ISO-8859-1,utf-8;q=0.7,*;q=0.7
Keep-Alive: 300
Connection: keep-alive
Cookie: FGNCLIID=05c04axp1yaqynldtcdiwis0ag1

`)
	httpGzipResponseRaw []byte = append(
		[]byte(`HTTP/1.1 200 OK
Date: Fri, 29 Oct 2004 05:21:00 GMT
Server: Apache/2.0.50 (Fedora)
Last-Modified: Fri, 29 Oct 2004 05:20:21 GMT
ETag: "126e1f-6d-371b2f40"
Accept-Ranges: bytes
Vary: Accept-Encoding
Content-Encoding: gzip
Content-Length: 92
Connection: close
Content-Type: text/html; charset=UTF-8

`),
		base64Decode("H4sIAAAAAAAAA7PJKMnNseOyyUhNTLHj4rQpySzJSbVzLclILUpNzFFwrUjMLchJVQhITE+10YdIctnoQ1TbJOWnVAI1cWJVDjRMH6IAqB5sCRcA024MQ20AAAA=")...,
	)
)

func TestHttpGzipCapFile(t *testing.T) {
	Convey("Test http gzip cap file", t, func() {
		Convey("Should get http application", func(c C) {
			srcChan := make(chan *network.PacketIPv4TCP, 100)
			dstChan := make(chan any, 100)
			wg, ctx := errgroupx.WithContext(context.Background())
			logger, err := logging.NewDefaultLogger()
			c.So(err, ShouldBeNil)
			wg.Go(func() error {
				defer close(srcChan)
				return extractPackets(ctx, "http_gzip.cap", srcChan)
			})
			wg.Go(func() error {
				router := tcp.NewRouter[*network.PacketIPv4TCP](100, 100, time.Second)
				return router.Run(ctx, logger, srcChan, dstChan)
			})
			err = wg.Wait()
			c.So(err, ShouldBeNil)
			c.So(len(dstChan), ShouldEqual, 1)
			app := (<-dstChan).(*network.HttpTransport)
			compareHttpTransport(c, app, httpGzipRequestRaw, httpGzipResponseRaw)
			close(dstChan)
		})
	})
}
