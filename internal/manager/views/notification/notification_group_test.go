package notification

import (
	// 标准库
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// createNotificationGroupTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
func createNotificationGroupTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// registerNotificationGroupTestRoutes 注册通知目标组管理路由（与 router.go 的注册保持一致）。
func registerNotificationGroupTestRoutes(admin *gin.RouterGroup) {
	group := admin.Group("/notification/group")
	{
		group.POST("/list", NotificationGroupListGet)
		group.OPTIONS("/list", views.ListMetaGinHandler[*views.NotificationGroupListGetSearchIn, *NotificationGroupListOut]())
		group.POST("/create", NotificationGroupCreate)
		group.POST("/:id/get", NotificationGroupDetailGet)
		group.POST("/:id/update", NotificationGroupUpdate)
		group.POST("/delete", NotificationGroupBatchDelete)
		group.POST("/:id/set-default", NotificationGroupSetDefault)
	}
}

// setupNotificationGroupTestRouter 创建测试路由引擎，注入 i18n、日志、数据库与固定 default 命名空间。
func setupNotificationGroupTestRouter(db *database.DB) *gin.Engine {
	return setupNotificationGroupTestRouterWithNamespace(db, middlewares.DefaultNamespace)
}

// setupNotificationGroupTestRouterWithNamespace 创建测试路由引擎，注入指定命名空间，
// 用于验证多租户命名空间隔离。
func setupNotificationGroupTestRouterWithNamespace(db *database.DB, namespace string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newTagRuleTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	// 模拟生产环境 NamespaceMiddleware：固定注入指定命名空间
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Next()
	})
	registerNotificationGroupTestRoutes(r.Group(""))
	return r
}

// doNotificationGroupRequest 执行一次通知目标组 HTTP 请求并返回响应记录器。
func doNotificationGroupRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic("failed to marshal test request body: " + err.Error())
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// createNotificationGroup 通过创建接口落库一条通知目标组，返回规则 ID。
func createNotificationGroup(r *gin.Engine, db *database.DB, name string, isDefault bool) uint {
	w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/create", map[string]any{
		"name":        name,
		"description": "test group",
		"isDefault":   isDefault,
	})
	So(w.Code, ShouldEqual, http.StatusOK)
	var group core.NotificationGroup
	So(db.Where("name = ?", name).First(&group).Error, ShouldBeNil)
	return group.ID
}

// createNotificationTarget 直接落库一个属于指定命名空间的通知目标，返回其 ID。
func createNotificationTarget(db *database.DB, namespace, name string, channelID uint) uint {
	tgt := &core.NotificationTarget{
		NamespaceName: namespace,
		Name:          name,
		ChannelID:     channelID,
		Type:          "email",
		Enabled:       true,
		Language:      "zh-Hans",
	}
	So(db.Create(tgt).Error, ShouldBeNil)
	return tgt.ID
}

// notificationGroupIDPath 拼接通知目标组路由路径。
func notificationGroupIDPath(action string, id uint) string {
	return "/notification/group/" + strconv.FormatUint(uint64(id), 10) + action
}

// notificationGroupCountRows 统计通知组目标关联表的记录数。
func notificationGroupCountRows(db *database.DB, where string, args ...any) int64 {
	var count int64
	So(db.Model(&core.NotificationGroupTarget{}).Where(where, args...).Count(&count).Error, ShouldBeNil)
	return count
}

// TestNotificationGroupCreateAndDetail 创建与详情查询全链路（无绑定目标）。
func TestNotificationGroupCreateAndDetail(t *testing.T) {
	Convey("测试通知目标组创建与详情查询", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)

		Convey("合法组创建成功且详情返回空目标列表", func() {
			id := createNotificationGroup(r, db, "ops-group", false)

			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                         `json:"code"`
				Data *NotificationGroupDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Name, ShouldEqual, "ops-group")
			So(resp.Data.IsDefault, ShouldBeFalse)
			So(resp.Data.Description, ShouldEqual, "test group")
			So(resp.Data.Targets, ShouldHaveLength, 0)
		})

		Convey("空名称被拒绝", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/create", map[string]any{
				"name": "",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("组名超过 128 字符被拒绝", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/create", map[string]any{
				"name": string(bytes.Repeat([]byte("a"), 129)),
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestNotificationGroupDefaultUniqueness 创建默认组 A 后再创建默认组 B，A 的默认标记应被取消。
func TestNotificationGroupDefaultUniqueness(t *testing.T) {
	Convey("测试默认组唯一性维护", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)

		idA := createNotificationGroup(r, db, "group-a", true)
		idB := createNotificationGroup(r, db, "group-b", true)

		var a, b core.NotificationGroup
		So(db.First(&a, idA).Error, ShouldBeNil)
		So(db.First(&b, idB).Error, ShouldBeNil)
		So(a.IsDefault, ShouldBeFalse)
		So(b.IsDefault, ShouldBeTrue)
	})
}

// TestNotificationGroupSetDefault 通过 set-default 接口切换默认组。
func TestNotificationGroupSetDefault(t *testing.T) {
	Convey("测试默认组切换", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)

		idA := createNotificationGroup(r, db, "group-a", true)
		idB := createNotificationGroup(r, db, "group-b", false)

		w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/set-default", idB), nil)
		So(w.Code, ShouldEqual, http.StatusOK)

		var a, b core.NotificationGroup
		So(db.First(&a, idA).Error, ShouldBeNil)
		So(db.First(&b, idB).Error, ShouldBeNil)
		So(a.IsDefault, ShouldBeFalse)
		So(b.IsDefault, ShouldBeTrue)

		Convey("不存在的组返回 404", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/set-default", 99999), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestNotificationGroupList 列表查询：isDefault 过滤、目标计数与列表选项。
func TestNotificationGroupList(t *testing.T) {
	Convey("测试通知目标组列表查询", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)

		idA := createNotificationGroup(r, db, "group-a", true)
		idB := createNotificationGroup(r, db, "group-b", false)
		// 为目标组 A 绑定 2 个目标
		channelID := dbCreateNotificationChannel(db, "default", "main-channel")
		t1 := createNotificationTarget(db, "default", "target-1", channelID)
		t2 := createNotificationTarget(db, "default", "target-2", channelID)
		w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", idA), map[string]any{"targetIds": []uint64{uint64(t1), uint64(t2)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		_ = idB

		Convey("按 isDefault=true 过滤且目标计数正确", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/list", map[string]any{
				"filterBy": map[string]any{
					"isDefault": map[string]any{"action": "eq", "value": true},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                        `json:"total"`
					Items []*NotificationGroupListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 1)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].Name, ShouldEqual, "group-a")
			So(resp.Data.Items[0].TargetCount, ShouldEqual, 2)
		})

		Convey("默认列表返回全部组，未绑定组目标计数为 0", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/list", map[string]any{"pageSize": 10})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Total uint                        `json:"total"`
					Items []*NotificationGroupListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Total, ShouldEqual, 2)
			var countA, countB int64
			for _, item := range resp.Data.Items {
				switch item.Name {
				case "group-a":
					countA = item.TargetCount
				case "group-b":
					countB = item.TargetCount
				}
			}
			So(countA, ShouldEqual, 2)
			So(countB, ShouldEqual, 0)
		})

		Convey("列表选项接口返回过滤与列元数据", func() {
			w := doNotificationGroupRequest(r, http.MethodOptions, "/notification/group/list", nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *views.ListMeta `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(len(resp.Data.OrderBy), ShouldEqual, 2)
			So(len(resp.Data.FilterBy), ShouldBeGreaterThan, 0)
			So(len(resp.Data.Headers), ShouldBeGreaterThan, 0)
		})
	})
}

// TestNotificationGroupCreateUpdateWithTargets 创建/编辑携带目标绑定：
// 创建随组绑定、创建时跨命名空间目标回滚、编辑整集替换、缺省不改动、空数组解绑全部。
func TestNotificationGroupCreateUpdateWithTargets(t *testing.T) {
	Convey("测试通知目标组创建/编辑的目标绑定", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)
		channelID := dbCreateNotificationChannel(db, "default", "main-channel")
		t1 := createNotificationTarget(db, "default", "target-1", channelID)
		t2 := createNotificationTarget(db, "default", "target-2", channelID)

		Convey("创建时随组绑定两个目标", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/create", map[string]any{
				"name":      "group-t",
				"isDefault": false,
				"targetIds": []uint64{uint64(t1), uint64(t2)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var group core.NotificationGroup
			So(db.Where("name = ?", "group-t").First(&group).Error, ShouldBeNil)
			So(notificationGroupCountRows(db, "group_id = ?", group.ID), ShouldEqual, 2)
		})

		Convey("创建时绑定跨命名空间目标返回 400 且组未创建（事务回滚）", func() {
			insertTestNamespace(db, "other-ns")
			otherChannelID := dbCreateNotificationChannel(db, "other-ns", "other-channel")
			otherTarget := createNotificationTarget(db, "other-ns", "other-target", otherChannelID)
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/create", map[string]any{
				"name":      "group-invalid",
				"targetIds": []uint64{uint64(otherTarget)},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.NotificationGroup{}).Where("name = ?", "group-invalid").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("编辑携带 targetIds 整集替换绑定目标", func() {
			id := createNotificationGroup(r, db, "group-e", false)
			doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{"targetIds": []uint64{uint64(t1), uint64(t2)}})
			// 编辑仅传 t1，t2 被移除
			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{
				"description": "updated",
				"targetIds":   []uint64{uint64(t1)},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(notificationGroupCountRows(db, "group_id = ?", id), ShouldEqual, 1)
		})

		Convey("编辑缺省 targetIds 保持既有绑定不变", func() {
			id := createNotificationGroup(r, db, "group-keep", false)
			doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{"targetIds": []uint64{uint64(t1), uint64(t2)}})
			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{
				"description": "desc only",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(notificationGroupCountRows(db, "group_id = ?", id), ShouldEqual, 2)
		})

		Convey("编辑携带空 targetIds 解绑全部", func() {
			id := createNotificationGroup(r, db, "group-clear", false)
			doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{"targetIds": []uint64{uint64(t1), uint64(t2)}})
			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{
				"targetIds": []uint64{},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			So(notificationGroupCountRows(db, "group_id = ?", id), ShouldEqual, 0)
		})
	})
}

// TestNotificationGroupUpdateDescription 仅更新描述，名称与默认标记保持不变。
func TestNotificationGroupUpdateDescription(t *testing.T) {
	Convey("测试通知目标组描述更新", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)
		id := createNotificationGroup(r, db, "group-up", true)

		w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{
			"description": "updated description",
		})
		So(w.Code, ShouldEqual, http.StatusOK)

		var group core.NotificationGroup
		So(db.First(&group, id).Error, ShouldBeNil)
		So(group.Description, ShouldEqual, "updated description")
		So(group.Name, ShouldEqual, "group-up")
		So(group.IsDefault, ShouldBeTrue)
	})
}

// TestNotificationGroupBatchDeleteCascade 批量删除组时同时删除其目标关联行。
func TestNotificationGroupBatchDeleteCascade(t *testing.T) {
	Convey("测试通知目标组批量删除级联", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)
		id := createNotificationGroup(r, db, "group-del", false)
		channelID := dbCreateNotificationChannel(db, "default", "del-channel")
		t1 := createNotificationTarget(db, "default", "del-target-1", channelID)
		t2 := createNotificationTarget(db, "default", "del-target-2", channelID)
		doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{"targetIds": []uint64{uint64(t1), uint64(t2)}})
		So(notificationGroupCountRows(db, "group_id = ?", id), ShouldEqual, 2)

		w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/delete", map[string]any{"ids": []uint64{uint64(id)}})
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int                  `json:"code"`
			Data views.BatchResultOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Code, ShouldEqual, http.StatusOK)
		So(resp.Data.Succeeded, ShouldEqual, 1)
		So(len(resp.Data.Failed), ShouldEqual, 0)

		// 组与关联行均被删除，目标保留
		var groupCount int64
		So(db.Model(&core.NotificationGroup{}).Where("id = ?", id).Count(&groupCount).Error, ShouldBeNil)
		So(groupCount, ShouldEqual, 0)
		So(notificationGroupCountRows(db, "group_id = ?", id), ShouldEqual, 0)
		var targetCount int64
		So(db.Model(&core.NotificationTarget{}).Count(&targetCount).Error, ShouldBeNil)
		So(targetCount, ShouldEqual, 2)

		Convey("重复删除幂等成功", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, "/notification/group/delete", map[string]any{"ids": []uint64{uint64(id)}})
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestNotificationGroupNamespaceIsolation 组按命名空间隔离，跨命名空间访问视为不存在。
func TestNotificationGroupNamespaceIsolation(t *testing.T) {
	Convey("测试通知目标组命名空间隔离", t, func() {
		db := createNotificationGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNotificationGroupTestRouter(db)
		id := createNotificationGroup(r, db, "ns-group", false)

		// 使用其他命名空间的 router 访问 default 命名空间内的组，均应视为不存在
		rOther := setupNotificationGroupTestRouterWithNamespace(db, "other-ns")

		Convey("详情查询跨命名空间返回 404", func() {
			w := doNotificationGroupRequest(rOther, http.MethodPost, notificationGroupIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("更新操作跨命名空间返回 404", func() {
			w := doNotificationGroupRequest(rOther, http.MethodPost, notificationGroupIDPath("/update", id), map[string]any{"description": "x"})
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("本命名空间内访问仍正常", func() {
			w := doNotificationGroupRequest(r, http.MethodPost, notificationGroupIDPath("/get", id), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// dbCreateNotificationChannel 直接落库一个通知渠道，返回其 ID。
func dbCreateNotificationChannel(db *database.DB, namespace, name string) uint {
	ch := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          name,
		Type:          "email",
		Enabled:       true,
		Description:   "test channel",
	}
	So(db.Create(ch).Error, ShouldBeNil)
	return ch.ID
}
