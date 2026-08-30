package notify

import (
	// stdlib
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	// third-party
	"github.com/smartystreets/goconvey/convey"
)

// TestWebhookChannel_Send 测试 webhook 渠道的发送行为。
// TestWebhookChannel_Send tests the send behavior of the webhook channel.
func TestWebhookChannel_Send(t *testing.T) {
	convey.Convey("webhook 渠道发送", t, func() {
		convey.Convey("POST 到目标 URL，携带消息 JSON 与附加请求头", func() {
			var (
				mu      sync.Mutex
				method  string
				path    string
				token   string
				content string
				body    string
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				method = r.Method
				path = r.URL.Path
				token = r.Header.Get("X-Token")
				content = r.Header.Get("Content-Type")
				b, _ := io.ReadAll(r.Body)
				body = string(b)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			ch := &webhookChannel{}
			cfg, _ := json.Marshal(map[string]any{
				"url":     srv.URL + "/hooks/alert",
				"headers": map[string]string{"X-Token": "secret"},
			})
			_, err := ch.Send(context.Background(), &Target{Config: cfg}, &RenderedMessage{Title: "t", Content: "c"})
			convey.So(err, convey.ShouldBeNil)

			mu.Lock()
			defer mu.Unlock()
			convey.So(method, convey.ShouldEqual, http.MethodPost)
			convey.So(path, convey.ShouldEqual, "/hooks/alert")
			convey.So(token, convey.ShouldEqual, "secret")
			convey.So(content, convey.ShouldContainSubstring, "application/json")
			convey.So(body, convey.ShouldContainSubstring, `"title":"t"`)
			convey.So(body, convey.ShouldContainSubstring, `"content":"c"`)
		})

		convey.Convey("非 2xx 响应返回错误", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, "boom")
			}))
			defer srv.Close()

			ch := &webhookChannel{}
			cfg, _ := json.Marshal(map[string]any{"url": srv.URL})
			_, err := ch.Send(context.Background(), &Target{Config: cfg}, &RenderedMessage{})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(err.Error(), convey.ShouldContainSubstring, "unexpected status 500")
		})

		convey.Convey("目标配置缺失 URL 返回错误", func() {
			ch := &webhookChannel{}
			cfg, _ := json.Marshal(map[string]any{"headers": map[string]string{}})
			_, err := ch.Send(context.Background(), &Target{Config: cfg}, &RenderedMessage{})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("目标配置为非法 JSON 返回错误", func() {
			ch := &webhookChannel{}
			_, err := ch.Send(context.Background(), &Target{Config: json.RawMessage(`{invalid`)}, &RenderedMessage{})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}
