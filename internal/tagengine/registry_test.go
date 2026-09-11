package tagengine_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/tagengine"
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
			tagengine.Register(tagengine.Target{Type: &registryValidModel{}, DisplayName: "model.test.registryValid"})
			target, ok := tagengine.TargetInfo(registryValidTypeKey)
			So(ok, ShouldBeTrue)
			So(target.DisplayName, ShouldEqual, "model.test.registryValid")
		})

		Convey("缺少 ID 字段的模型注册时 panic", func() {
			recovered := recoverPanic(func() {
				tagengine.Register(tagengine.Target{Type: &registryNoIDModel{}, DisplayName: "model.test.registryNoID"})
			})
			So(recovered, ShouldNotBeNil)
			msg, ok := recovered.(string)
			So(ok, ShouldBeTrue)
			So(msg, ShouldContainSubstring, "unique ID field")
		})

		Convey("ID 字段类型不为 serializer.HexUUID 的模型注册时 panic", func() {
			recovered := recoverPanic(func() {
				tagengine.Register(tagengine.Target{Type: &registryWrongIDModel{}, DisplayName: "model.test.registryWrongID"})
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
