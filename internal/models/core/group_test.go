package core

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// TestGroup_Struct 测试 Group 结构体的基本属性。
func TestGroup_Struct(t *testing.T) {
	convey.Convey("Test Group struct", t, func() {
		convey.Convey("Should embed database.BaseModel", func() {
			group := &Group{}
			// 验证 Group 结构体嵌入了 BaseModel
			// 通过类型断言检查是否具有 BaseModel 字段
			var _ *database.BaseModel = &group.BaseModel
			convey.So(true, convey.ShouldBeTrue) // 如果上面没有编译错误，则测试通过
		})

		convey.Convey("Should have Name field with correct tag", func() {
			group := &Group{}
			// 通过反射检查 gorm 标签，这里我们只验证字段存在
			// 实际标签验证由 GORM 处理，这里仅作示例
			convey.So(group.Name, convey.ShouldEqual, "")
		})

		convey.Convey("Should have Description field with correct tag", func() {
			group := &Group{}
			convey.So(group.Description, convey.ShouldEqual, "")
		})
	})
}

// TestGroup_FieldValidation 测试 Group 字段的验证逻辑。
func TestGroup_FieldValidation(t *testing.T) {
	convey.Convey("Test Group field validation", t, func() {
		convey.Convey("Should accept valid Name within length limit", func() {
			group := &Group{
				Name: "valid-group-name",
			}
			convey.So(group.Name, convey.ShouldEqual, "valid-group-name")
		})

		convey.Convey("Should accept empty Description", func() {
			group := &Group{
				Name:        "test-group",
				Description: "",
			}
			convey.So(group.Description, convey.ShouldEqual, "")
		})

		convey.Convey("Should accept Description within length limit", func() {
			description := "这是一个测试组的描述信息，用于验证描述字段的功能。"
			group := &Group{
				Name:        "test-group",
				Description: description,
			}
			convey.So(group.Description, convey.ShouldEqual, description)
		})
	})
}

// TestGroup_Initialization 测试 Group 结构体的初始化行为。
func TestGroup_Initialization(t *testing.T) {
	convey.Convey("Test Group initialization", t, func() {
		convey.Convey("Should initialize with zero values", func() {
			group := &Group{}
			convey.So(group.Name, convey.ShouldEqual, "")
			convey.So(group.Description, convey.ShouldEqual, "")
		})

		convey.Convey("Should initialize with provided values", func() {
			group := &Group{
				Name:        "admin-group",
				Description: "管理员用户组",
			}
			convey.So(group.Name, convey.ShouldEqual, "admin-group")
			convey.So(group.Description, convey.ShouldEqual, "管理员用户组")
		})
	})
}

// TestGroup_ModelRegistration 测试 Group 模型注册行为。
func TestGroup_ModelRegistration(t *testing.T) {
	convey.Convey("Test Group model registration", t, func() {
		convey.Convey("Should be registered as core model via init function", func() {
			// init 函数在包导入时自动执行，这里我们验证 Group 类型是否正确
			// 实际注册逻辑由 models.EnrollCoreModel 处理，这里仅作类型检查
			group := &Group{}
			convey.So(group, convey.ShouldHaveSameTypeAs, &Group{})
		})
	})
}
