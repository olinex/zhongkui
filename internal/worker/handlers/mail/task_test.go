package mail

import (
	// 标准库
	"context"
	"encoding/json"
	"testing"

	// 第三方
	"github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	message "zhongkuisec.com/zhongkui/internal/pkg/message"
)

// fakeETLTask 实现 etl.Task 接口的测试替身。
type fakeETLTask struct {
	typ     string
	payload []byte
}

// 编译期断言：确保 fakeETLTask 实现了 etl.Task 接口。
var _ etl.Task = (*fakeETLTask)(nil)

// Type 返回任务类型。
func (t *fakeETLTask) Type() string { return t.typ }

// Payload 返回任务载荷。
func (t *fakeETLTask) Payload() []byte { return t.payload }

// fakeMailSender 记录 Send 调用的测试替身，sendErr 非 nil 时 Send 返回该错误。
type fakeMailSender struct {
	sendErr error
	calls   int
}

// Send 记录一次调用并返回预设错误（可为 nil）。
func (f *fakeMailSender) Send(ctx context.Context, to string, msg message.Message, lang string) error {
	f.calls++
	return f.sendErr
}

// testMessage 构造一个可用的结构化邮件消息。
func testMessage() message.Message {
	return message.Message{Template: "custom.mail.alert", Params: map[string]any{"Name": "zk"}}
}

// TestValidateMailSendInput 测试邮件直发载荷的字段校验。
func TestValidateMailSendInput(t *testing.T) {
	convey.Convey("validateMailSendInput 校验邮件直发载荷", t, func() {
		convey.Convey("合法输入通过校验", func() {
			err := validateMailSendInput(MailSendInput{To: "a@example.com", Message: testMessage(), Language: "zh-Hans"})
			convey.So(err, convey.ShouldBeNil)
		})

		convey.Convey("空收件人返回错误", func() {
			err := validateMailSendInput(MailSendInput{Message: testMessage()})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("非法邮箱返回错误", func() {
			err := validateMailSendInput(MailSendInput{To: "not-an-email", Message: testMessage()})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("空消息模板返回错误", func() {
			err := validateMailSendInput(MailSendInput{To: "a@example.com", Message: message.Message{Template: ""}})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestNewMailSendTask 测试邮件直发任务的编码。
func TestNewMailSendTask(t *testing.T) {
	convey.Convey("NewMailSendTask 编码邮件直发任务", t, func() {
		convey.Convey("合法输入编码成功、类型为 mail:send 且往返一致", func() {
			task, err := NewMailSendTask(MailSendInput{To: "a@example.com", Message: testMessage(), Language: "en"})
			convey.So(err, convey.ShouldBeNil)
			convey.So(task, convey.ShouldNotBeNil)
			convey.So(task.Type(), convey.ShouldEqual, MailSendTask)
			var decoded MailSendInput
			convey.So(json.Unmarshal(task.Payload(), &decoded), convey.ShouldBeNil)
			convey.So(decoded.To, convey.ShouldEqual, "a@example.com")
			convey.So(decoded.Message.Template, convey.ShouldEqual, "custom.mail.alert")
			convey.So(decoded.Message.Params["Name"], convey.ShouldEqual, "zk")
			convey.So(decoded.Language, convey.ShouldEqual, "en")
		})

		convey.Convey("非法输入返回错误", func() {
			_, err := NewMailSendTask(MailSendInput{To: "not-an-email", Message: testMessage()})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestMailSendHandler_Do 测试邮件直发处理器的载荷解析与投递结果。
func TestMailSendHandler_Do(t *testing.T) {
	convey.Convey("MailSendHandler 载荷解析与投递", t, func() {
		convey.Convey("非法 JSON 载荷返回错误", func() {
			logger, _ := logging.NewDefaultLogger()
			handler := &MailSendHandler{sender: &fakeMailSender{}, logger: logger}
			err := handler.Do(context.Background(), &fakeETLTask{typ: MailSendTask, payload: []byte(`{invalid`)})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失收件人或模板返回错误", func() {
			logger, _ := logging.NewDefaultLogger()
			handler := &MailSendHandler{sender: &fakeMailSender{}, logger: logger}
			err := handler.Do(context.Background(), &fakeETLTask{typ: MailSendTask, payload: []byte(`{"to":"a@example.com"}`)})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("合法载荷且发送成功返回 nil", func() {
			logger, _ := logging.NewDefaultLogger()
			fake := &fakeMailSender{}
			handler := &MailSendHandler{sender: fake, logger: logger}
			payload, _ := json.Marshal(MailSendInput{To: "a@example.com", Message: testMessage(), Language: "zh-Hans"})
			err := handler.Do(context.Background(), &fakeETLTask{typ: MailSendTask, payload: payload})
			convey.So(err, convey.ShouldBeNil)
			convey.So(fake.calls, convey.ShouldEqual, 1)
		})

		convey.Convey("发送失败返回错误", func() {
			logger, _ := logging.NewDefaultLogger()
			fake := &fakeMailSender{sendErr: context.DeadlineExceeded}
			handler := &MailSendHandler{sender: fake, logger: logger}
			payload, _ := json.Marshal(MailSendInput{To: "a@example.com", Message: testMessage(), Language: "zh-Hans"})
			err := handler.Do(context.Background(), &fakeETLTask{typ: MailSendTask, payload: payload})
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(fake.calls, convey.ShouldEqual, 1)
		})
	})
}
