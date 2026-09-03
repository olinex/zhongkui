package message

import (
	// stdlib
	"context"
	"encoding/json"
	"strings"
	"testing"

	// third-party
	"github.com/smartystreets/goconvey/convey"
)

// TestEmailChannel_Factory 测试邮件渠道工厂的配置校验。
// TestEmailChannel_Factory tests the config validation of the email channel factory.
func TestEmailChannel_Factory(t *testing.T) {
	convey.Convey("邮件渠道工厂配置校验", t, func() {
		convey.Convey("host 与 from 齐全时构建成功", func() {
			_, err := New(TypeEmail, json.RawMessage(`{"host":"smtp.example.com","from":"alert@example.com"}`))
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("host 与 from 缺失返回错误", func() {
			_, err := New(TypeEmail, json.RawMessage(`{}`))
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("port 缺省时默认 25", func() {
			ch, err := New(TypeEmail, json.RawMessage(`{"host":"smtp.example.com","from":"alert@example.com"}`))
			convey.So(err, convey.ShouldBeNil)
			email, ok := ch.(*emailChannel)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(email.port, convey.ShouldEqual, 25)
		})

		convey.Convey("非法 JSON 返回错误", func() {
			_, err := New(TypeEmail, json.RawMessage(`{invalid`))
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestEmailChannel_Send 测试邮件渠道发送的目标配置校验。
// TestEmailChannel_Send tests the target config validation of the email channel send.
func TestEmailChannel_Send(t *testing.T) {
	convey.Convey("邮件渠道发送配置校验", t, func() {
		ch := &emailChannel{host: "smtp.example.com", port: 25, from: "alert@example.com"}

		convey.Convey("目标配置缺失 to 返回错误", func() {
			_, err := ch.Send(context.Background(), &Target{Config: json.RawMessage(`{}`)}, &RenderedMessage{})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("收件人邮箱非法返回错误", func() {
			_, err := ch.Send(context.Background(), &Target{Config: json.RawMessage(`{"to":"not-an-email"}`)}, &RenderedMessage{})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(strings.Contains(err.Error(), "invalid recipient"), convey.ShouldBeTrue)
		})
	})
}

// TestEmailChannel_BuildMailBody 测试邮件报文组装：标准邮件头 + 正文。
// TestEmailChannel_BuildMailBody tests the mail body assembly with standard headers
// plus the body.
func TestEmailChannel_BuildMailBody(t *testing.T) {
	convey.Convey("组装标准邮件报文", t, func() {
		body := string(buildMailBody("alert@example.com", "ops@example.com", &RenderedMessage{Title: "alert", Content: "hello"}))
		convey.So(strings.Contains(body, "From: alert@example.com"), convey.ShouldBeTrue)
		convey.So(strings.Contains(body, "To: ops@example.com"), convey.ShouldBeTrue)
		convey.So(strings.Contains(body, "Subject: alert"), convey.ShouldBeTrue)
		convey.So(strings.Contains(body, "MIME-Version: 1.0"), convey.ShouldBeTrue)
		convey.So(strings.Contains(body, "charset=UTF-8"), convey.ShouldBeTrue)
		convey.So(strings.Contains(body, "\r\n\r\nhello"), convey.ShouldBeTrue)
	})
}