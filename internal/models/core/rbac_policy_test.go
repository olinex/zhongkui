package core

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestRbacPolicy_Struct 测试 RbacPolicy 结构体的基本属性
func TestRbacPolicy_Struct(t *testing.T) {
	convey.Convey("Test RbacPolicy struct", t, func() {
		convey.Convey("Should embed database.BaseModel", func() {
			policy := &RbacPolicy{}
			// 验证 RbacPolicy 结构体嵌入了 BaseModel
			var _ *database.BaseModel = &policy.BaseModel
			convey.So(true, convey.ShouldBeTrue) // 如果上面没有编译错误，则测试通过
		})

		convey.Convey("Should have Ptype field with correct constraints", func() {
			policy := &RbacPolicy{}
			// Ptype 字段应该存在且为空字符串初始值
			convey.So(policy.Ptype, convey.ShouldEqual, "")
		})

		convey.Convey("Should have V0 field with correct constraints", func() {
			policy := &RbacPolicy{}
			convey.So(policy.V0, convey.ShouldEqual, "")
		})

		convey.Convey("Should have V1 field with correct constraints", func() {
			policy := &RbacPolicy{}
			convey.So(policy.V1, convey.ShouldEqual, "")
		})

		convey.Convey("Should have V2 field with correct constraints", func() {
			policy := &RbacPolicy{}
			convey.So(policy.V2, convey.ShouldEqual, "")
		})

		convey.Convey("Should have V3 field with correct default value", func() {
			policy := &RbacPolicy{}
			// V3 字段应该有默认空字符串值
			convey.So(policy.V3, convey.ShouldEqual, "")
		})
	})
}

// TestNewGroupPolicy 测试 NewGroupPolicy 函数
func TestNewGroupPolicy(t *testing.T) {
	convey.Convey("Test NewGroupPolicy function", t, func() {
		convey.Convey("Should create group policy with correct Ptype", func() {
			group := "admin"
			target := "/api/users"
			action := "read"
			effect := base.EFFECT_ALLOW

			policy := NewGroupPolicy(group, target, action, effect)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.Ptype, convey.ShouldEqual, base.POLICY_LABEL)
			convey.So(policy.V0, convey.ShouldEqual, group)
			convey.So(policy.V1, convey.ShouldEqual, target)
			convey.So(policy.V2, convey.ShouldEqual, action)
			convey.So(policy.V3, convey.ShouldEqual, effect)
		})

		convey.Convey("Should create group policy with deny effect", func() {
			group := "guest"
			target := "/api/admin"
			action := "write"
			effect := base.EFFECT_DENY

			policy := NewGroupPolicy(group, target, action, effect)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.Ptype, convey.ShouldEqual, base.POLICY_LABEL)
			convey.So(policy.V3, convey.ShouldEqual, effect)
		})

		convey.Convey("Should handle empty strings for all parameters", func() {
			policy := NewGroupPolicy("", "", "", "")
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.Ptype, convey.ShouldEqual, base.POLICY_LABEL)
			convey.So(policy.V0, convey.ShouldEqual, "")
			convey.So(policy.V1, convey.ShouldEqual, "")
			convey.So(policy.V2, convey.ShouldEqual, "")
			convey.So(policy.V3, convey.ShouldEqual, "")
		})

		convey.Convey("Should handle long target names within limit", func() {
			// 创建接近长度限制的目标名
			longTarget := "/api/" + string(make([]byte, 500)) + "/resource"
			policy := NewGroupPolicy("test", longTarget, "read", base.EFFECT_ALLOW)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(len(policy.V1), convey.ShouldBeGreaterThan, 500)
		})
	})
}

// TestNewUserPolicy 测试 NewUserPolicy 函数
func TestNewUserPolicy(t *testing.T) {
	convey.Convey("Test NewUserPolicy function", t, func() {
		convey.Convey("Should create user policy with correct Ptype", func() {
			unionID := serializer.HexUUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
				0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
			group := "admin"
			namespace := "default"

			policy := NewUserPolicy(unionID, group, namespace)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.Ptype, convey.ShouldEqual, base.GROUP_LABEL)
			convey.So(policy.V0, convey.ShouldEqual, unionID.String())
			convey.So(policy.V1, convey.ShouldEqual, group)
			convey.So(policy.V2, convey.ShouldEqual, namespace)
			convey.So(policy.V3, convey.ShouldEqual, "")
		})

		convey.Convey("Should handle nil union ID", func() {
			var unionID serializer.HexUUID
			group := "user"
			namespace := "test"

			policy := NewUserPolicy(unionID, group, namespace)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.V0, convey.ShouldEqual, "00000000-0000-0000-0000-000000000000")
		})

		convey.Convey("Should handle empty group and namespace", func() {
			unionID := serializer.NewHexUUIDv4()
			policy := NewUserPolicy(unionID, "", "")
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.V1, convey.ShouldEqual, "")
			convey.So(policy.V2, convey.ShouldEqual, "")
		})

		convey.Convey("Should handle built-in namespace", func() {
			unionID := serializer.NewHexUUIDv4()
			group := base.DEFAULT_GROUP
			namespace := base.DEFAULT_NAMESPACE

			policy := NewUserPolicy(unionID, group, namespace)
			
			convey.So(policy, convey.ShouldNotBeNil)
			convey.So(policy.V1, convey.ShouldEqual, base.DEFAULT_GROUP)
			convey.So(policy.V2, convey.ShouldEqual, base.DEFAULT_NAMESPACE)
		})
	})
}

// TestRbacPolicyTemplate_Struct 测试 RbacPolicyTemplate 结构体的基本属性
func TestRbacPolicyTemplate_Struct(t *testing.T) {
	convey.Convey("Test RbacPolicyTemplate struct", t, func() {
		convey.Convey("Should embed database.BaseModel", func() {
			template := &RbacPolicyTemplate{}
			// 验证 RbacPolicyTemplate 结构体嵌入了 BaseModel
			var _ *database.BaseModel = &template.BaseModel
			convey.So(true, convey.ShouldBeTrue) // 如果上面没有编译错误，则测试通过
		})

		convey.Convey("Should have Target field with correct constraints", func() {
			template := &RbacPolicyTemplate{}
			// Target 字段应该存在且为空字符串初始值
			convey.So(template.Target, convey.ShouldEqual, "")
		})

		convey.Convey("Should have Action field with correct constraints", func() {
			template := &RbacPolicyTemplate{}
			convey.So(template.Action, convey.ShouldEqual, "")
		})
	})
}

// TestNewRbacPolicyTemplate 测试 NewRbacPolicyTemplate 函数
func TestNewRbacPolicyTemplate(t *testing.T) {
	convey.Convey("Test NewRbacPolicyTemplate function", t, func() {
		convey.Convey("Should create policy template with correct values", func() {
			target := "/api/users"
			action := "read"

			template := NewRbacPolicyTemplate(target, action)
			
			convey.So(template, convey.ShouldNotBeNil)
			convey.So(template.Target, convey.ShouldEqual, target)
			convey.So(template.Action, convey.ShouldEqual, action)
		})

		convey.Convey("Should handle empty strings for parameters", func() {
			template := NewRbacPolicyTemplate("", "")
			
			convey.So(template, convey.ShouldNotBeNil)
			convey.So(template.Target, convey.ShouldEqual, "")
			convey.So(template.Action, convey.ShouldEqual, "")
		})

		convey.Convey("Should handle long target names within limit", func() {
			// 创建接近长度限制的目标名
			longTarget := "/api/" + string(make([]byte, 500)) + "/resource"
			template := NewRbacPolicyTemplate(longTarget, "write")
			
			convey.So(template, convey.ShouldNotBeNil)
			convey.So(len(template.Target), convey.ShouldBeGreaterThan, 500)
		})

		convey.Convey("Should handle various action types", func() {
			testCases := []struct {
				name   string
				target string
				action string
			}{
				{"Read action", "/api/data", "read"},
				{"Write action", "/api/data", "write"},
				{"Delete action", "/api/data", "delete"},
				{"Execute action", "/api/scripts", "execute"},
				{"Admin action", "/api/admin", "admin"},
			}

			for _, tc := range testCases {
				convey.Convey(tc.name, func() {
					template := NewRbacPolicyTemplate(tc.target, tc.action)
					
					convey.So(template, convey.ShouldNotBeNil)
					convey.So(template.Target, convey.ShouldEqual, tc.target)
					convey.So(template.Action, convey.ShouldEqual, tc.action)
				})
			}
		})
	})
}

// TestRbacPolicy_FieldMapping 测试 RbacPolicy 字段的正确映射
func TestRbacPolicy_FieldMapping(t *testing.T) {
	convey.Convey("Test RbacPolicy field mapping for policy type", t, func() {
		convey.Convey("Should correctly map fields for policy type 'p'", func() {
			policy := NewGroupPolicy("admin", "/api/users", "read", base.EFFECT_ALLOW)
			
			// 验证字段映射
			convey.So(policy.Ptype, convey.ShouldEqual, "p")
			convey.So(policy.V0, convey.ShouldEqual, "admin")      // group name
			convey.So(policy.V1, convey.ShouldEqual, "/api/users") // target object
			convey.So(policy.V2, convey.ShouldEqual, "read")       // action
			convey.So(policy.V3, convey.ShouldEqual, "allow")      // effect
		})

		convey.Convey("Should correctly map fields for policy type 'g'", func() {
			unionID := serializer.NewHexUUIDv4()
			policy := NewUserPolicy(unionID, "admin", "default")
			
			// 验证字段映射
			convey.So(policy.Ptype, convey.ShouldEqual, "g")
			convey.So(policy.V0, convey.ShouldEqual, unionID.String()) // user union id
			convey.So(policy.V1, convey.ShouldEqual, "admin")          // group name
			convey.So(policy.V2, convey.ShouldEqual, "default")        // namespace
			convey.So(policy.V3, convey.ShouldEqual, "")               // blank for group type
		})
	})
}

// TestRbacPolicy_Integration 测试 RbacPolicy 的集成场景
func TestRbacPolicy_Integration(t *testing.T) {
	convey.Convey("Test RbacPolicy integration scenarios", t, func() {
		convey.Convey("Should create complete RBAC policy system", func() {
			// 创建用户策略
			userID := serializer.NewHexUUIDv4()
			userPolicy := NewUserPolicy(userID, "admin", "production")
			
			// 创建组策略
			groupPolicy1 := NewGroupPolicy("admin", "/api/users", "read", base.EFFECT_ALLOW)
			groupPolicy2 := NewGroupPolicy("admin", "/api/users", "write", base.EFFECT_ALLOW)
			groupPolicy3 := NewGroupPolicy("admin", "/api/admin", "*", base.EFFECT_DENY)
			
			// 创建策略模板
			template1 := NewRbacPolicyTemplate("/api/users", "read")
			template2 := NewRbacPolicyTemplate("/api/users", "write")
			template3 := NewRbacPolicyTemplate("/api/admin", "admin")
			
			// 验证所有对象都被正确创建
			convey.So(userPolicy, convey.ShouldNotBeNil)
			convey.So(groupPolicy1, convey.ShouldNotBeNil)
			convey.So(groupPolicy2, convey.ShouldNotBeNil)
			convey.So(groupPolicy3, convey.ShouldNotBeNil)
			convey.So(template1, convey.ShouldNotBeNil)
			convey.So(template2, convey.ShouldNotBeNil)
			convey.So(template3, convey.ShouldNotBeNil)
			
			// 验证策略类型
			convey.So(userPolicy.Ptype, convey.ShouldEqual, "g")
			convey.So(groupPolicy1.Ptype, convey.ShouldEqual, "p")
			convey.So(groupPolicy2.Ptype, convey.ShouldEqual, "p")
			convey.So(groupPolicy3.Ptype, convey.ShouldEqual, "p")
			
			// 验证效果字段
			convey.So(groupPolicy1.V3, convey.ShouldEqual, "allow")
			convey.So(groupPolicy2.V3, convey.ShouldEqual, "allow")
			convey.So(groupPolicy3.V3, convey.ShouldEqual, "deny")
			convey.So(userPolicy.V3, convey.ShouldEqual, "")
		})

		convey.Convey("Should handle wildcard matching in policies", func() {
			// 测试通配符策略
			wildcardPolicy := NewGroupPolicy("superuser", "*", "*", base.EFFECT_ALLOW)
			
			convey.So(wildcardPolicy, convey.ShouldNotBeNil)
			convey.So(wildcardPolicy.V1, convey.ShouldEqual, base.WIDE_MATCH_LABEL)
			convey.So(wildcardPolicy.V2, convey.ShouldEqual, base.WIDE_MATCH_LABEL)
		})
	})
}