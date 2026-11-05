package tagengine_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// registryValidModel 含共享 ID 锚点（serializer.HexUUID）的合法目标模型。
type registryValidModel struct {
	ID  serializer.HexUUID
	Foo string
}

// registryNoIDModel 缺少 ID 字段的非法目标模型。
type registryNoIDModel struct {
	Foo string
}

// registryWrongIDModel ID 字段类型与共享锚点不一致的非法目标模型。
type registryWrongIDModel struct {
	ID uint64
}

// registryValidTypeKey registryValidModel 的类型键，断言 TargetInfo 命中时使用。
var registryValidTypeKey = func() string {
	t, err := tagengine.TypeKey(&registryValidModel{})
	if err != nil {
		panic("failed to compute registry valid model type key: " + err.Error())
	}
	return t
}()

// TestRegister 校验 Register 的 ID 约束：
// 目标模型必须具有唯一 ID 字段且类型为 serializer.HexUUID，违规注册直接 panic。
func TestRegister(t *testing.T) {
	Convey("Test Register", t, func() {
		Convey("含唯一 ID 字段（serializer.HexUUID）的模型注册成功", func() {
			tagengine.Register(tagengine.Target{Type: &registryValidModel{}})
			target, ok := tagengine.TargetInfo(registryValidTypeKey)
			So(ok, ShouldBeTrue)
			So(target.Type, ShouldNotBeNil)
			So(target.Dimensions, ShouldBeEmpty)

			// TypeKeys 返回已注册模型的类型键（不含 label；模型展示 label 由 tagRuleModel
			// 枚举类推导键 enum.tagRuleModel.<ModelKey> 经统一枚举接口本地化）
			So(tagengine.TypeKeys(), ShouldContain, registryValidTypeKey)
		})

		Convey("缺少 ID 字段的模型注册时 panic", func() {
			recovered := recoverPanic(func() {
				tagengine.Register(tagengine.Target{Type: &registryNoIDModel{}})
			})
			So(recovered, ShouldNotBeNil)
			msg, ok := recovered.(string)
			So(ok, ShouldBeTrue)
			So(msg, ShouldContainSubstring, "unique ID field")
		})

		Convey("ID 字段类型不为 serializer.HexUUID 的模型注册时 panic", func() {
			recovered := recoverPanic(func() {
				tagengine.Register(tagengine.Target{Type: &registryWrongIDModel{}})
			})
			So(recovered, ShouldNotBeNil)
			msg, ok := recovered.(string)
			So(ok, ShouldBeTrue)
			So(msg, ShouldContainSubstring, "ID field type")
		})
	})
}

// recoverPanic 执行 fn 并捕获其 panic 值，未 panic 时返回 nil。
func recoverPanic(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

// TestModelKey 校验 ModelKey 从完整类型键推导 camelCase 短展示键（枚举 Option.Key 唯一来源）。
func TestModelKey(t *testing.T) {
	Convey("Test ModelKey", t, func() {
		Convey("完整类型键取末段并转小驼峰", func() {
			So(tagengine.ModelKey("zhongkuisec.com/zhongkui/internal/models/network.HttpTransport"), ShouldEqual, "httpTransport")
		})

		Convey("已是枚举短键时幂等", func() {
			So(tagengine.ModelKey("httpTransport"), ShouldEqual, "httpTransport")
		})

		Convey("无点段时回退原串转小驼峰", func() {
			So(tagengine.ModelKey("HttpTransport"), ShouldEqual, "httpTransport")
			So(tagengine.ModelKey("m1"), ShouldEqual, "m1")
		})

		Convey("空键返回空串", func() {
			So(tagengine.ModelKey(""), ShouldEqual, "")
			So(tagengine.ModelKey("abc."), ShouldEqual, "")
		})
	})
}
