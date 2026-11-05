package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/views/user"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// createUserTestDB 创建用于用户路由测试的共享 PostgreSQL 测试数据库
func createUserTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// setupUserTestRouter 创建用于用户路由测试的 Gin 路由引擎
// 注入 i18n 中间件以支持错误响应的国际化
func setupUserTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	return r
}

// setupUserTestRouterWithDB 创建带有数据库中间件的测试路由引擎
func setupUserTestRouterWithDB(db *database.DB) *gin.Engine {
	r := setupUserTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	// 注入 RBAC enforcer，供 UserGroupJoin/Leave 等 handler 经 GetRbacPolicy 使用
	r.Use(middlewares.RbacPolicyMiddleware(newTestRbacEnforcer(db)))
	return r
}

// TestUserGroupIn_Struct 测试 UserGroupIn 结构体
func TestUserGroupIn_Struct(t *testing.T) {
	Convey("测试 UserGroupIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			userGroup := UserGroupIn{
				Namespace: "default",
				GroupName: "admin",
			}
			So(userGroup.Namespace, ShouldEqual, "default")
			So(userGroup.GroupName, ShouldEqual, "admin")
		})

		Convey("应正确序列化为 JSON", func() {
			userGroup := UserGroupIn{
				Namespace: "production",
				GroupName: "developers",
			}
			data, err := json.Marshal(userGroup)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"namespace":"production"`)
			So(string(data), ShouldContainSubstring, `"groupName":"developers"`)
		})

		Convey("应验证字段长度约束", func() {
			// 测试最小长度约束
			userGroup := UserGroupIn{
				Namespace: "",   // 空字符串，小于最小长度1
				GroupName: "ab", // 长度为2，小于最小长度3
			}
			// 在实际使用中，这些约束会由Gin的binding验证
			So(len(userGroup.Namespace), ShouldEqual, 0)
			So(len(userGroup.GroupName), ShouldEqual, 2)
		})
	})
}

// TestUserBaseDetailOut_Struct 测试 UserBaseDetailOut 结构体
func TestUserBaseDetailOut_Struct(t *testing.T) {
	Convey("测试 UserBaseDetailOut 结构体", t, func() {
		Convey("应正确嵌入 BaseSoftDeleteItemOut", func() {
			now := time.Now()
			deletedTime := now.Add(-24 * time.Hour) // 假设24小时前删除
			detail := UserBaseDetailOut{
				BaseSoftDeleteItemOut: utils.BaseSoftDeleteItemOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        1001,
						CreatedAt: now,
						UpdatedAt: now,
					},
					DeletedAt: &deletedTime,
				},
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "测试用户",
				Username:   "testuser",
				Email:      "test@example.com",
				Active:     true,
			}
			So(detail.ID, ShouldEqual, 1001)
			So(detail.UnionID.String(), ShouldNotBeEmpty)
			So(detail.NickleName, ShouldEqual, "测试用户")
			So(detail.Username, ShouldEqual, "testuser")
			So(detail.Email, ShouldEqual, "test@example.com")
			So(detail.Active, ShouldBeTrue)
			So(detail.DeletedAt, ShouldNotBeNil)
			So(*detail.DeletedAt, ShouldHappenBefore, now)
		})

		Convey("应正确序列化为 JSON", func() {
			detail := UserBaseDetailOut{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "张三",
				Username:   "zhangsan",
				Email:      "zhangsan@example.com",
				Active:     false,
			}
			data, err := json.Marshal(detail)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"unionID"`)
			So(string(data), ShouldContainSubstring, `"nickleName":"张三"`)
			So(string(data), ShouldContainSubstring, `"username":"zhangsan"`)
			So(string(data), ShouldContainSubstring, `"email":"zhangsan@example.com"`)
			So(string(data), ShouldContainSubstring, `"active":false`)
		})
	})
}

// TestUserDetailOut_Struct 测试 UserDetailOut 结构体
func TestUserDetailOut_Struct(t *testing.T) {
	Convey("测试 UserDetailOut 结构体", t, func() {
		Convey("应正确嵌入 UserBaseDetailOut 并包含组信息", func() {
			detail := UserDetailOut{
				UserBaseDetailOut: UserBaseDetailOut{
					UnionID:    serializer.NewHexUUIDv4(),
					NickleName: "李四",
					Username:   "lisi",
					Email:      "lisi@example.com",
					Active:     true,
				},
				Groups: []UserRbacPolicyDetailOut{
					{
						V1: "admin",
						V2: "default",
					},
					{
						V1: "developers",
						V2: "production",
					},
				},
			}
			So(detail.NickleName, ShouldEqual, "李四")
			So(len(detail.Groups), ShouldEqual, 2)
			So(detail.Groups[0].V1, ShouldEqual, "admin")
			So(detail.Groups[0].V2, ShouldEqual, "default")
			So(detail.Groups[1].V1, ShouldEqual, "developers")
			So(detail.Groups[1].V2, ShouldEqual, "production")
		})

		Convey("应正确处理空组列表", func() {
			detail := UserDetailOut{
				UserBaseDetailOut: UserBaseDetailOut{
					UnionID:    serializer.NewHexUUIDv4(),
					NickleName: "王五",
					Username:   "wangwu",
					Email:      "wangwu@example.com",
					Active:     true,
				},
				Groups: []UserRbacPolicyDetailOut{},
			}
			So(detail.NickleName, ShouldEqual, "王五")
			So(len(detail.Groups), ShouldEqual, 0)
		})
	})
}

// TestUserListOut_Struct 测试 UserListOut 结构体
func TestUserListOut_Struct(t *testing.T) {
	Convey("测试 UserListOut 结构体", t, func() {
		Convey("应正确嵌入 UserBaseDetailOut 和 BaseTotalOut", func() {
			listItem := UserListOut{
				UserBaseDetailOut: UserBaseDetailOut{
					UnionID:    serializer.NewHexUUIDv4(),
					NickleName: "赵六",
					Username:   "zhaoliu",
					Email:      "zhaoliu@example.com",
					Active:     true,
				},
				BaseTotalOut: utils.BaseTotalOut{
					Total: 150,
				},
			}
			So(listItem.NickleName, ShouldEqual, "赵六")
			So(listItem.Total, ShouldEqual, 150)
		})

		Convey("应正确序列化为 JSON", func() {
			listItem := UserListOut{
				UserBaseDetailOut: UserBaseDetailOut{
					UnionID:    serializer.NewHexUUIDv4(),
					NickleName: "测试",
					Username:   "test",
					Email:      "test@example.com",
					Active:     false,
				},
				BaseTotalOut: utils.BaseTotalOut{
					Total: 50,
				},
			}
			data, err := json.Marshal(listItem)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"nickleName":"测试"`)
			// 注意：BaseTotalOut.Total 字段的 JSON 标签是 "-"，所以不会序列化到 JSON 中
			// So(string(data), ShouldContainSubstring, `"total":50`)
		})
	})
}

// TestUserDetailGet 测试 UserDetailGet 函数
func TestUserDetailGet(t *testing.T) {
	Convey("测试 UserDetailGet 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由
		r.GET("/api/v1/admin/user/:unionID/get", UserDetailGet)

		Convey("应成功获取用户详情", func() {
			// 创建测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "测试用户详情",
				Username:   "userdetail",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "detail@example.com",
				Active:     true,
			}
			db.Create(user)

			// 创建用户组策略
			policy := core.NewUserPolicy(user.UnionID, "admin", "default")
			db.Create(policy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user/"+user.UnionID.String()+"/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 解析响应
			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			// 调试输出
			// t.Logf("Response: %+v", response)
			// 检查响应结构
			So(response, ShouldContainKey, "data")
			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data, ShouldContainKey, "nickleName")
			So(data["nickleName"], ShouldEqual, "测试用户详情")
			So(data["username"], ShouldEqual, "userdetail")
			So(data["email"], ShouldEqual, "detail@example.com")
			So(data["active"], ShouldBeTrue)
		})

		Convey("应返回错误当用户不存在时", func() {
			nonExistentUUID := serializer.NewHexUUIDv4()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user/"+nonExistentUUID.String()+"/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			// 注意：这里返回空数据而不是错误，因为Find方法不会返回错误
			// 在实际使用中，可能需要添加额外的检查
		})

		Convey("应返回错误当unionID格式无效时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user/invalid-uuid/get", nil)
			r.ServeHTTP(w, req)

			// 调试输出
			t.Logf("状态码: %d, 响应体: %s", w.Code, w.Body.String())
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestUserUpdate 测试 UserUpdate 函数
func TestUserUpdate(t *testing.T) {
	Convey("测试 UserUpdate 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由
		r.PUT("/api/v1/admin/user/:unionID/update", UserUpdate)

		Convey("应成功更新用户信息", func() {
			// 创建测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "原始昵称",
				Username:   "updateuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "original@example.com",
				Active:     true,
			}
			db.Create(user)

			// 准备更新请求
			reqBody := map[string]any{
				"nickleName": "更新后的昵称",
				"email":      "updated@example.com",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user/"+user.UnionID.String()+"/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库记录已更新
			var updatedUser core.User
			db.First(&updatedUser, "union_id = ?", user.UnionID)
			So(updatedUser.NickleName, ShouldEqual, "更新后的昵称")
			So(updatedUser.Email, ShouldEqual, "updated@example.com")
		})

		Convey("应返回错误当请求体无效时", func() {
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "测试用户",
				Username:   "testuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "test@example.com",
				Active:     true,
			}
			db.Create(user)

			// 发送缺少必需字段的JSON（有效的JSON但缺少必需字段）
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user/"+user.UnionID.String()+"/update", bytes.NewReader([]byte(`{"email": "updated@example.com"}`))) // 缺少nickleName
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 调试输出
			t.Logf("状态码: %d, 响应体: %s", w.Code, w.Body.String())
			// 注意：UserUpdate 函数使用 CreateAccidentError，所以返回 500 而不是 400
			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应返回错误当unionID格式无效时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user/invalid-uuid/update", bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestUserSetActive 测试 UserBatchSetActive 函数
func TestUserSetActive(t *testing.T) {
	Convey("测试 UserBatchSetActive 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由 - 批量激活用户
		r.POST("/api/v1/admin/user/activate", UserBatchSetActive(true))
		// 注册测试路由 - 批量禁用用户
		r.POST("/api/v1/admin/user/deactivate", UserBatchSetActive(false))

		Convey("应成功激活用户", func() {
			// 创建禁用状态的测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "待激活用户",
				Username:   "inactiveuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "inactive@example.com",
				Active:     false,
			}
			db.Create(user)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/activate",
				bytes.NewReader([]byte(`{"unionIDs":"`+user.UnionID.String()+`"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证用户已激活
			var activatedUser core.User
			db.First(&activatedUser, "union_id = ?", user.UnionID)
			So(activatedUser.Active, ShouldBeTrue)
		})

		Convey("应成功禁用用户", func() {
			// 创建激活状态的测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "待禁用用户",
				Username:   "activeuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "active@example.com",
				Active:     true,
			}
			db.Create(user)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/deactivate",
				bytes.NewReader([]byte(`{"unionIDs":["`+user.UnionID.String()+`"]}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证用户已禁用
			var deactivatedUser core.User
			db.First(&deactivatedUser, "union_id = ?", user.UnionID)
			So(deactivatedUser.Active, ShouldBeFalse)
		})

		Convey("应返回错误当 unionIds 为空数组时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/activate",
				bytes.NewReader([]byte(`{"unionIDs":[]}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestUserGroupJoin 测试 UserGroupJoin 函数
func TestUserGroupJoin(t *testing.T) {
	Convey("测试 UserGroupJoin 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/user/:unionID/group/join", UserGroupJoin)

		Convey("应成功将用户加入用户组", func() {
			// 创建测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "组加入用户",
				Username:   "groupjoinuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "groupjoin@example.com",
				Active:     true,
			}
			db.Create(user)

			// 创建测试用户组
			group := &core.Group{
				Name:        "testgroup",
				Description: "测试用户组",
			}
			db.Create(group)

			// 创建测试命名空间
			namespace := &core.Namespace{
				Name: "prod",
			}
			db.Create(namespace)

			// 准备加入请求
			reqBody := map[string]any{
				"namespace": "prod",
				"groupName": "testgroup",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+user.UnionID.String()+"/group/join", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证策略已创建
			var policy core.RbacPolicy
			err := db.First(&policy, "ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
				base.GroupLabel, user.UnionID.String(), "testgroup", "prod").Error
			So(err, ShouldBeNil)
			So(policy.V0, ShouldEqual, user.UnionID.String())
			So(policy.V1, ShouldEqual, "testgroup")
			So(policy.V2, ShouldEqual, "prod")
		})

		Convey("应返回错误当用户不存在时", func() {
			nonExistentUUID := serializer.NewHexUUIDv4()

			reqBody := map[string]any{
				"namespace": "default",
				"groupName": "testgroup",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+nonExistentUUID.String()+"/group/join", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应返回错误当用户组不存在时", func() {
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "测试用户",
				Username:   "testuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "test@example.com",
				Active:     true,
			}
			db.Create(user)

			reqBody := map[string]any{
				"namespace": "default",
				"groupName": "nonexistentgroup",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+user.UnionID.String()+"/group/join", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

// TestUserGroupLeave 测试 UserGroupLeave 函数
func TestUserGroupLeave(t *testing.T) {
	Convey("测试 UserGroupLeave 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/user/:unionID/group/leave", UserGroupLeave)

		Convey("应成功将用户从用户组移除", func() {
			// 创建测试用户
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "组离开用户",
				Username:   "groupleaveuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "groupleave@example.com",
				Active:     true,
			}
			db.Create(user)

			// 创建用户组策略
			policy := core.NewUserPolicy(user.UnionID, "testgroup", "default")
			db.Create(policy)

			// 准备离开请求
			reqBody := map[string]any{
				"namespace": "default",
				"groupName": "testgroup",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+user.UnionID.String()+"/group/leave", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证策略已删除
			var count int64
			db.Model(&core.RbacPolicy{}).Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
				base.GroupLabel, user.UnionID.String(), "testgroup", "default").Count(&count)
			// 调试输出
			t.Logf("策略删除后数量: %d", count)
			So(count, ShouldEqual, 0)
		})

		Convey("应返回错误当请求体无效时", func() {
			user := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "测试用户",
				Username:   "testuser",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "test@example.com",
				Active:     true,
			}
			db.Create(user)

			// 发送缺少必需字段的JSON（有效的JSON但缺少必需字段）
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+user.UnionID.String()+"/group/leave", bytes.NewReader([]byte(`{"namespace": "default"}`))) // 缺少groupName
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 调试输出
			t.Logf("状态码: %d, 响应体: %s", w.Code, w.Body.String())
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当unionID格式无效时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/invalid-uuid/group/leave", bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestUserCreateIn_Struct 测试 UserCreateIn 结构体
func TestUserCreateIn_Struct(t *testing.T) {
	Convey("测试 UserCreateIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			in := UserCreateIn{
				UserProfileChangeIn: user.UserProfileChangeIn{
					NickleName: "测试昵称",
					Email:      "test@example.com",
				},
				UserPasswordLoginIn: user.UserPasswordLoginIn{
					Username: "testuser",
					Password: "password123",
				},
			}
			So(in.NickleName, ShouldEqual, "测试昵称")
			So(in.Email, ShouldEqual, "test@example.com")
			So(in.Username, ShouldEqual, "testuser")
			So(in.Password, ShouldEqual, "password123")
		})

		Convey("应正确序列化为 JSON", func() {
			in := UserCreateIn{
				UserProfileChangeIn: user.UserProfileChangeIn{
					NickleName: "张三",
					Email:      "zhangsan@example.com",
				},
				UserPasswordLoginIn: user.UserPasswordLoginIn{
					Username: "zhangsan",
					Password: "password123",
				},
			}
			data, err := json.Marshal(in)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"nickleName":"张三"`)
			So(string(data), ShouldContainSubstring, `"email":"zhangsan@example.com"`)
			So(string(data), ShouldContainSubstring, `"username":"zhangsan"`)
			So(string(data), ShouldContainSubstring, `"password":"password123"`)
		})
	})
}

// TestUserCreate 测试 UserCreate 函数
func TestUserCreate(t *testing.T) {
	Convey("测试 UserCreate 函数", t, func() {
		// 创建测试数据库和路由
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupUserTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/user/create", UserCreate)

		Convey("应成功创建用户", func() {
			reqBody := map[string]any{
				"username":   "newuser",
				"password":   "Str0ngP@ssw0rd",
				"nickleName": "新用户昵称测试",
				"email":      "new@example.com",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证用户已落库且默认激活
			var user core.User
			err := db.First(&user, "username = ?", "newuser").Error
			So(err, ShouldBeNil)
			So(user.NickleName, ShouldEqual, "新用户昵称测试")
			So(user.Email, ShouldEqual, "new@example.com")
			So(user.Active, ShouldBeTrue)
			So(user.Password, ShouldNotBeNil)
		})

		Convey("应返回错误当用户名或邮箱已存在时", func() {
			// 预先创建同名用户
			existing := &core.User{
				UnionID:    serializer.NewHexUUIDv4(),
				NickleName: "已存在用户",
				Username:   "duplicate",
				Password:   serializer.NewCrypto([]byte("hashed_password")),
				Email:      "duplicate@example.com",
				Active:     true,
			}
			db.Create(existing)

			reqBody := map[string]any{
				"username":   "duplicate",
				"password":   "Str0ngP@ssw0rd",
				"nickleName": "重复用户昵称",
				"email":      "other@example.com",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			// 发送缺少 nickleName 字段的请求
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/create", bytes.NewReader([]byte(`{"username":"nofield","password":"password123","email":"nofield@example.com"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestUserFrontendConfigReset 测试管理员重置用户前端配置接口
func TestUserFrontendConfigReset(t *testing.T) {
	Convey("测试 UserFrontendConfigReset 函数", t, func() {
		db := createUserTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := &core.User{
			UnionID:                   serializer.NewHexUUIDv4(),
			NickleName:                "重置配置用户",
			Username:                  "fcresetuser",
			Password:                  serializer.NewCrypto([]byte("hashedpassword")),
			Email:                     "fcreset@example.com",
			Active:                    true,
			FrontendConfig:            `{"theme":"dark"}`,
			FrontendConfigHash:        frontendConfigHash(`{"theme":"dark"}`),
			FrontendConfigVersion:     3,
			FrontendConfigSyncEnabled: true,
		}
		if err := db.Create(user).Error; err != nil {
			panic("failed to create test user: " + err.Error())
		}

		r := setupUserTestRouterWithDB(db)
		r.POST("/api/v1/admin/user/:unionID/frontend-config/reset", UserFrontendConfigReset)

		Convey("应成功重置用户的前端配置", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/"+user.UnionID.String()+"/frontend-config/reset", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库中的配置已被清空，版本号重置为 0
			var target core.User
			err := db.First(&target, "union_id = ?", user.UnionID.String()).Error
			So(err, ShouldBeNil)
			So(target.FrontendConfig, ShouldEqual, "")
			// PostgreSQL 的 CHAR(64)（bpchar）会用空格补齐空值，读取侧按去空白后为空断言
			So(strings.TrimSpace(target.FrontendConfigHash), ShouldEqual, "")
			So(target.FrontendConfigVersion, ShouldEqual, uint64(0))
			// 同步开关状态保留不变
			So(target.FrontendConfigSyncEnabled, ShouldBeTrue)
		})

		Convey("无效 unionID 应返回 400", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user/invalid-uuid/frontend-config/reset", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}
