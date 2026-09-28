package message

import (
	// 标准库
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	// 第三方
	"github.com/smartystreets/goconvey/convey"
)

// testType 自定义测试渠道类型键，验证渠道基座的可扩展性。
const testType = "test"

// testChannel 自定义测试渠道实现，模拟一次成功投递并返回固定消息 ID。
type testChannel struct{}

// Type 返回渠道类型键。
func (c *testChannel) Type() string { return testType }

// Send 模拟一次成功投递，返回固定消息 ID。
func (c *testChannel) Send(ctx context.Context, _ *Target, _ *RenderedMessage) (string, error) {
	return "test-msg-id", nil
}

// TestChannel_RegisterAndNew 测试注册工厂后可通过 New 构建渠道实例。
func TestChannel_RegisterAndNew(t *testing.T) {
	convey.Convey("注册自定义测试渠道后可通过 New 构建实例", t, func() {
		register(testType, func(_ json.RawMessage) (Channel, error) {
			return &testChannel{}, nil
		})
		ch, err := New(testType, json.RawMessage(`{}`))
		convey.So(err, convey.ShouldBeNil)
		convey.So(ch, convey.ShouldNotBeNil)
		convey.So(ch.Type(), convey.ShouldEqual, testType)
	})
}

// TestChannel_New_Unregistered 测试未注册的渠道类型返回错误。
func TestChannel_New_Unregistered(t *testing.T) {
	convey.Convey("未注册的渠道类型返回错误", t, func() {
		_, err := New("nonexistent", json.RawMessage(`{}`))
		convey.So(err, convey.ShouldNotBeNil)
	})
}

// TestChannel_Register_Duplicate 测试同一类型键重复注册触发 panic。
func TestChannel_Register_Duplicate(t *testing.T) {
	convey.Convey("重复注册同一渠道类型应 panic", t, func() {
		const dupType = "test:duplicate"
		register(dupType, func(_ json.RawMessage) (Channel, error) {
			return &testChannel{}, nil
		})
		defer func() {
			r := recover()
			convey.So(r, convey.ShouldNotBeNil)
			if e, ok := r.(error); ok {
				convey.So(strings.Contains(e.Error(), "already registered"), convey.ShouldBeTrue)
			}
		}()
		register(dupType, func(_ json.RawMessage) (Channel, error) {
			return &testChannel{}, nil
		})
	})
}

// TestChannel_Types 测试 Types 返回已注册类型键且按升序排列。
func TestChannel_Types(t *testing.T) {
	convey.Convey("Types 返回已注册类型键且升序排列", t, func() {
		types := Types()
		convey.So(slices.Contains(types, TypeEmail), convey.ShouldBeTrue)
		convey.So(slices.Contains(types, TypeWebhook), convey.ShouldBeTrue)
		convey.So(slices.IsSorted(types), convey.ShouldBeTrue)
	})
}

// TestChannel_Placeholder 验证占位符渠道判定：渠道级配置无字段的类型即为占位符，
// 其余类型（如 email 含 SMTP 字段）不是；PlaceholderTypes 返回升序占位类型列表。
func TestChannel_Placeholder(t *testing.T) {
	convey.Convey("按渠道级配置字段数判定占位符渠道", t, func() {
		convey.So(IsPlaceholderType(TypeWebhook), convey.ShouldBeTrue)
		convey.So(IsPlaceholderType(TypeEmail), convey.ShouldBeFalse)
		convey.So(IsPlaceholderType("not-registered"), convey.ShouldBeFalse)

		types := PlaceholderTypes()
		convey.So(slices.Contains(types, TypeWebhook), convey.ShouldBeTrue)
		convey.So(slices.Contains(types, TypeEmail), convey.ShouldBeFalse)
		convey.So(slices.IsSorted(types), convey.ShouldBeTrue)
	})
}
