package notify

import (
	// stdlib
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	// third-party
	"github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

var initServiceTestKeyOnce sync.Once

// initServiceTestKeyDerivation 初始化全局密钥派生：渠道/目标配置经 crypto 序列化器
// 加解密存储，测试须与生产一致地派生密钥，否则写库会报 KeyDerivation not initialized。
func initServiceTestKeyDerivation() {
	initServiceTestKeyOnce.Do(func() {
		provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
		book, err := key.LoadPasswordBook("")
		if err != nil {
			panic("failed to load password book: " + err.Error())
		}
		key.InitGlobalKeyDerivation(provider, book)
	})
}

// newServiceTestDB 创建共享已迁移的 PostgreSQL 测试库（testdb.Core）。
// newServiceTestDB creates a shared, pre-migrated PostgreSQL test DB (testdb.Core).
func newServiceTestDB(t *testing.T) *database.DB {
	t.Helper()
	initServiceTestKeyDerivation()
	return testdb.Core(t)
}

// newTestService 创建测试用的通知服务实例。
// newTestService creates a notification service for tests.
func newTestService(db *database.DB) *Service {
	logger, _ := logging.NewDefaultLogger()
	return NewService(db, logger)
}

// testMessage 构造一个基于真实模板键 tag_event.alert 的结构化通知消息，
// 使全链路发送经由 locales 模板渲染出具体文案（而非代码写死的标题与正文）。
// testMessage builds a structured message pinned to the real tag_event.alert
// template key so the full delivery flow renders concrete text via the locale
// templates instead of hardcoded titles and content.
func testMessage() Message {
	return Message{
		Template: TemplateTagEventAlert,
		Params:   map[string]any{"Namespace": "default", "RuleName": "alert-rule", "DetectedAt": "2026-01-01T00:00:00Z"},
	}
}

// seedWebhook 在指定命名空间下构造一个启用状态的 webhook 渠道、目标与默认通知组，
// 并将目标加入该组；返回组与目标实体（含 ID）。
// seedWebhook creates an enabled webhook channel, target and default group in the
// given namespace, binding the target into the group; it returns the group and
// target entities (with IDs).
func seedWebhook(t *testing.T, db *database.DB, namespace, url string) (*core.NotificationGroup, *core.NotificationTarget) {
	t.Helper()
	channel := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          "webhook-channel",
		Type:          TypeWebhook,
		Enabled:       true,
		Config:        serializer.NewCrypto([]byte(`{}`)),
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatal("failed to create channel: " + err.Error())
	}
	targetCfg, _ := json.Marshal(map[string]any{
		"url":     url + "/hooks/alert",
		"headers": map[string]string{"X-Token": "secret"},
	})
	target := &core.NotificationTarget{
		NamespaceName: namespace,
		Name:          "webhook-target",
		ChannelID:     channel.ID,
		Type:          TypeWebhook,
		Enabled:       true,
		Config:        serializer.NewCrypto(targetCfg),
	}
	if err := db.Create(target).Error; err != nil {
		t.Fatal("failed to create target: " + err.Error())
	}
	group := &core.NotificationGroup{
		NamespaceName: namespace,
		Name:          "default-group",
		IsDefault:     true,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatal("failed to create group: " + err.Error())
	}
	if err := db.Create(&core.NotificationGroupTarget{GroupID: group.ID, TargetID: target.ID}).Error; err != nil {
		t.Fatal("failed to create group-target: " + err.Error())
	}
	return group, target
}

// seedWebhookTarget 在给定命名空间下新建一个启用 webhook 渠道与目标，并将目标加入指定
// 通知组；渠道/目标名称以组 ID 与序号保证同命名空间内唯一。返回目标实体（含 ID）。
// seedWebhookTarget creates an enabled webhook channel and target in the namespace and
// binds the target into the given group; channel/target names are unique within the
// namespace via the group ID and a sequence. It returns the target entity (with ID).
func seedWebhookTarget(t *testing.T, db *database.DB, namespace, url string, groupID uint, seq int) *core.NotificationTarget {
	t.Helper()
	channel := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          fmt.Sprintf("webhook-channel-%d-%d", groupID, seq),
		Type:          TypeWebhook,
		Enabled:       true,
		Config:        serializer.NewCrypto([]byte(`{}`)),
	}
	if err := db.Create(channel).Error; err != nil {
		t.Fatal("failed to create channel: " + err.Error())
	}
	targetCfg, _ := json.Marshal(map[string]any{
		"url":     url + "/hooks/alert",
		"headers": map[string]string{"X-Token": "secret"},
	})
	target := &core.NotificationTarget{
		NamespaceName: namespace,
		Name:          fmt.Sprintf("webhook-target-%d-%d", groupID, seq),
		ChannelID:     channel.ID,
		Type:          TypeWebhook,
		Enabled:       true,
		Config:        serializer.NewCrypto(targetCfg),
	}
	if err := db.Create(target).Error; err != nil {
		t.Fatal("failed to create target: " + err.Error())
	}
	if err := db.Create(&core.NotificationGroupTarget{GroupID: groupID, TargetID: target.ID}).Error; err != nil {
		t.Fatal("failed to create group-target: " + err.Error())
	}
	return target
}

// TestService_ResolveGroupTargets 测试 ResolveGroupTargets：将通知组解析为逐目标直投
// 入参列表，供上层编排器扇出为逐目标直投任务。
// TestService_ResolveGroupTargets tests ResolveGroupTargets: expanding a group into a
// list of per-target direct-send inputs for the orchestration layer to fan out.
func TestService_ResolveGroupTargets(t *testing.T) {
	convey.Convey("ResolveGroupTargets 通知组扇出解析", t, func() {
		convey.Convey("显式指定组时返回组内启用目标的直投入参列表", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			group := &core.NotificationGroup{NamespaceName: "default", Name: "group-a"}
			convey.So(db.Create(group).Error, convey.ShouldBeNil)
			targetA := seedWebhookTarget(t, db, "default", srv.URL, group.ID, 1)
			targetB := seedWebhookTarget(t, db, "default", srv.URL, group.ID, 2)

			msg := testMessage()
			inputs, err := svc.ResolveGroupTargets(context.Background(), SendGroupInput{
				Namespace: "default",
				GroupID:   &group.ID,
				EventType: core.EventTypeTagEventRecord,
				EventID:   100,
				Message:   msg,
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(inputs), convey.ShouldEqual, 2)
			convey.So(inputs[0].TargetID, convey.ShouldEqual, targetA.ID)
			convey.So(inputs[1].TargetID, convey.ShouldEqual, targetB.ID)
			for _, in := range inputs {
				convey.So(in.Namespace, convey.ShouldEqual, "default")
				convey.So(in.EventType, convey.ShouldEqual, core.EventTypeTagEventRecord)
				convey.So(in.EventID, convey.ShouldEqual, 100)
				convey.So(in.Message, convey.ShouldResemble, msg)
			}
		})

		convey.Convey("未显式指定组时使用命名空间默认组", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			group, targetA := seedWebhook(t, db, "default", srv.URL)
			targetB := seedWebhookTarget(t, db, "default", srv.URL, group.ID, 2)

			inputs, err := svc.ResolveGroupTargets(context.Background(), SendGroupInput{
				Namespace: "default",
				EventType: core.EventTypeTagEventRecord,
				EventID:   101,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(inputs), convey.ShouldEqual, 2)
			convey.So(inputs[0].TargetID, convey.ShouldEqual, targetA.ID)
			convey.So(inputs[1].TargetID, convey.ShouldEqual, targetB.ID)
			convey.So(inputs[0].EventID, convey.ShouldEqual, 101)
		})

		convey.Convey("组内无启用目标时返回空列表且无错误", func() {
			reqs := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqs++
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			group, target := seedWebhook(t, db, "default", srv.URL)
			convey.So(db.Model(&core.NotificationTarget{}).Where("id = ?", target.ID).Update("enabled", false).Error, convey.ShouldBeNil)

			inputs, err := svc.ResolveGroupTargets(context.Background(), SendGroupInput{
				Namespace: "default",
				GroupID:   &group.ID,
				EventType: core.EventTypeTagEventRecord,
				EventID:   102,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(inputs), convey.ShouldEqual, 0)
			convey.So(reqs, convey.ShouldEqual, 0)
		})

		convey.Convey("无通知组时返回空列表（命名空间不存在或组不匹配）", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)

			inputs, err := svc.ResolveGroupTargets(context.Background(), SendGroupInput{
				Namespace: "empty",
				EventType: core.EventTypeTagEventRecord,
				EventID:   103,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(len(inputs), convey.ShouldEqual, 0)
		})
	})
}

// TestService_SendToTarget 测试单目标直投：不经通知组加载目标发送并写入 GroupID=0 的
// 记录，同目标幂等去重，以及目标禁用时静默跳过。
// TestService_SendToTarget tests direct single-target send: loading a target without a
// group, delivering and writing a record with GroupID=0, idempotent dedup per target,
// and silent skip when the target is disabled.
func TestService_SendToTarget(t *testing.T) {
	convey.Convey("单目标直投", t, func() {
		convey.Convey("向未分组目标直投写入 GroupID=0 的记录并命中 webhook", func() {
			var (
				mu       sync.Mutex
				reqs     int
				lastBody string
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				reqs++
				b, _ := io.ReadAll(r.Body)
				lastBody = string(b)
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			_, target := seedWebhook(t, db, "default", srv.URL)

			err := svc.SendToTarget(context.Background(), SendTargetInput{
				Namespace: "default",
				EventType: core.EventTypeTagEventRecord,
				EventID:   300,
				TargetID:  target.ID,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)

			mu.Lock()
			convey.So(reqs, convey.ShouldEqual, 1)
			convey.So(lastBody, convey.ShouldContainSubstring, "标签事件告警: alert-rule")
			mu.Unlock()

			var record core.NotificationRecord
			convey.So(db.Select("status", "group_id", "target_id").
				Where("event_type = ? AND event_id = ? AND target_id = ?", core.EventTypeTagEventRecord, 300, target.ID).
				First(&record).Error, convey.ShouldBeNil)
			convey.So(record.Status, convey.ShouldEqual, core.RecordStatusSuccess)
			convey.So(record.GroupID, convey.ShouldEqual, 0)
			convey.So(record.TargetID, convey.ShouldEqual, target.ID)
		})

		convey.Convey("相同 (eventType,eventID,targetID) 重复直投仅投递一次（幂等）", func() {
			var (
				mu   sync.Mutex
				reqs int
			)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				reqs++
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			_, target := seedWebhook(t, db, "default", srv.URL)
			input := SendTargetInput{
				Namespace: "default",
				EventType: core.EventTypeTagEventRecord,
				EventID:   301,
				TargetID:  target.ID,
				Message:   testMessage(),
			}

			convey.So(svc.SendToTarget(context.Background(), input), convey.ShouldBeNil)
			convey.So(svc.SendToTarget(context.Background(), input), convey.ShouldBeNil)
			mu.Lock()
			convey.So(reqs, convey.ShouldEqual, 1)
			mu.Unlock()

			var count int64
			convey.So(db.Model(&core.NotificationRecord{}).
				Where("event_type = ? AND event_id = ? AND target_id = ?", core.EventTypeTagEventRecord, 301, target.ID).
				Count(&count).Error, convey.ShouldBeNil)
			convey.So(count, convey.ShouldEqual, 1)
		})

		convey.Convey("目标禁用时直投静默跳过不产生记录且无错误", func() {
			var reqs int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqs++
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			db := newServiceTestDB(t)
			svc := newTestService(db)
			_, target := seedWebhook(t, db, "default", srv.URL)
			convey.So(db.Model(&core.NotificationTarget{}).Where("id = ?", target.ID).Update("enabled", false).Error, convey.ShouldBeNil)

			err := svc.SendToTarget(context.Background(), SendTargetInput{
				Namespace: "default",
				EventType: core.EventTypeTagEventRecord,
				EventID:   302,
				TargetID:  target.ID,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(reqs, convey.ShouldEqual, 0)

			var count int64
			convey.So(db.Model(&core.NotificationRecord{}).Count(&count).Error, convey.ShouldBeNil)
			convey.So(count, convey.ShouldEqual, 0)
		})

		convey.Convey("目标不存在时直投静默跳过不产生记录且无错误", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)

			err := svc.SendToTarget(context.Background(), SendTargetInput{
				Namespace: "default",
				EventType: core.EventTypeTagEventRecord,
				EventID:   303,
				TargetID:  999,
				Message:   testMessage(),
			})
			convey.So(err, convey.ShouldBeNil)

			var count int64
			convey.So(db.Model(&core.NotificationRecord{}).Count(&count).Error, convey.ShouldBeNil)
			convey.So(count, convey.ShouldEqual, 0)
		})
	})
}

// TestService_ResolveGroup 测试通知组解析：显式组、命名空间默认组与不存在组。
// TestService_ResolveGroup tests group resolution: explicit group, namespace default
// group and a non-existent group.
func TestService_ResolveGroup(t *testing.T) {
	convey.Convey("通知组解析", t, func() {
		convey.Convey("显式指定组时返回该组", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			group := &core.NotificationGroup{NamespaceName: "default", Name: "group-a"}
			convey.So(db.Create(group).Error, convey.ShouldBeNil)
			other := &core.NotificationGroup{NamespaceName: "default", Name: "group-b"}
			convey.So(db.Create(other).Error, convey.ShouldBeNil)

			got, err := svc.resolveGroup(context.Background(), "default", &group.ID)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldNotBeNil)
			convey.So(got.ID, convey.ShouldEqual, group.ID)
		})

		convey.Convey("未显式指定组时返回命名空间默认组", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			group := &core.NotificationGroup{NamespaceName: "default", Name: "default-group", IsDefault: true}
			convey.So(db.Create(group).Error, convey.ShouldBeNil)
			other := &core.NotificationGroup{NamespaceName: "default", Name: "group-b"}
			convey.So(db.Create(other).Error, convey.ShouldBeNil)

			got, err := svc.resolveGroup(context.Background(), "default", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldNotBeNil)
			convey.So(got.ID, convey.ShouldEqual, group.ID)
		})

		convey.Convey("指定组不存在或命名空间不匹配时返回 nil", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			group := &core.NotificationGroup{NamespaceName: "default", Name: "group-a"}
			convey.So(db.Create(group).Error, convey.ShouldBeNil)

			got, err := svc.resolveGroup(context.Background(), "default", func() *uint { id := uint(999); return &id }())
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldBeNil)

			got, err = svc.resolveGroup(context.Background(), "other", &group.ID)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldBeNil)
		})

		convey.Convey("无默认组的命名空间未指定组时返回 nil", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			group := &core.NotificationGroup{NamespaceName: "default", Name: "group-a"}
			convey.So(db.Create(group).Error, convey.ShouldBeNil)

			got, err := svc.resolveGroup(context.Background(), "default", nil)
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldBeNil)
		})
	})
}

// seedTemplate 在测试库中直接插入一条命名空间级通知模板行（source=default）。
// seedTemplate inserts a namespace-scoped notification template row (source=default)
// directly into the test DB.
func seedTemplate(t *testing.T, db *database.DB, tmpl *core.NotificationTemplate) *core.NotificationTemplate {
	t.Helper()
	if err := db.Create(tmpl).Error; err != nil {
		t.Fatal("failed to create template: " + err.Error())
	}
	return tmpl
}

// renderTestTarget 构造一个最小化的通知目标（仅含命名空间与语言），供 renderMessage
// 直接测试：renderMessage 只依赖 NamespaceName 与 Language 及外部传入的渠道类型。
// renderTestTarget builds a minimal notification target (namespace + language only) for
// directly testing renderMessage, which depends solely on NamespaceName and Language plus
// the externally passed channel type.
func renderTestTarget(namespace, language string) *core.NotificationTarget {
	return &core.NotificationTarget{NamespaceName: namespace, Language: language}
}

// TestService_ResolveTemplate 测试持久化模板按三要素（事件/渠道类型/语言）的最低特异性
// 解析：精确优先于通配、总分相同时精确渠道优先于精确语言。
// TestService_ResolveTemplate tests persisting template resolution by the three factors
// (event/channel/language) with the lowest specificity: exact beats wildcard, and on a tie
// an exact channel beats an exact language.
func TestService_ResolveTemplate(t *testing.T) {
	convey.Convey("持久化模板解析", t, func() {
		convey.Convey("精确渠道+精确语言优先于通配组合", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   "*",
				Language:      "zh-Hans",
				Title:         "WILD-CH-TITLE",
				Content:       "WILD-CH-CONTENT",
				Source:        core.TemplateSourceDefault,
			})
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "zh-Hans",
				Title:         "EXACT-TITLE",
				Content:       "EXACT-CONTENT",
				Source:        core.TemplateSourceDefault,
			})

			got, err := svc.resolveTemplate(context.Background(), "default", core.EventTypeTagEventRecord, TypeWebhook, "zh-Hans")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldNotBeNil)
			convey.So(got.Title, convey.ShouldEqual, "EXACT-TITLE")
		})

		convey.Convey("总分相同时精确渠道优先于精确语言", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   "*",
				Language:      "zh-Hans",
				Title:         "CH-WILD-TITLE",
				Content:       "CH-WILD-CONTENT",
				Source:        core.TemplateSourceDefault,
			})
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "*",
				Title:         "LANG-WILD-TITLE",
				Content:       "LANG-WILD-CONTENT",
				Source:        core.TemplateSourceDefault,
			})

			got, err := svc.resolveTemplate(context.Background(), "default", core.EventTypeTagEventRecord, TypeWebhook, "zh-Hans")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldNotBeNil)
			convey.So(got.Title, convey.ShouldEqual, "LANG-WILD-TITLE")
		})

		convey.Convey("无匹配行时返回 nil", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			got, err := svc.resolveTemplate(context.Background(), "empty", core.EventTypeTagEventRecord, TypeWebhook, "zh-Hans")
			convey.So(err, convey.ShouldBeNil)
			convey.So(got, convey.ShouldBeNil)
		})
	})
}

// TestService_RenderMessage_TemplateFallback 测试 renderMessage 的模板解析回退链：
// 精确持久化模板渲染、渠道通配、语言通配、无行回退 locales 代码模板，以及 {{range}}
// 渲染。
// TestService_RenderMessage_TemplateFallback tests renderMessage's template resolution
// fallback chain: exact persisted template rendering, channel wildcard, language wildcard,
// locale fallback with no row, and {{range}} rendering.
func TestService_RenderMessage_TemplateFallback(t *testing.T) {
	convey.Convey("renderMessage 模板回退链", t, func() {
		convey.Convey("命中精确持久化模板（事件+渠道+语言）时渲染持久化文本", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "zh-Hans",
				Title:         "P-TITLE {{.RuleName}}",
				Content:       "P-CONTENT {{.RuleName}}",
				Source:        core.TemplateSourceDefault,
			})

			target := renderTestTarget("default", "zh-Hans")
			msg := testMessage()
			rendered, err := svc.renderMessage(context.Background(), target, TypeWebhook, &msg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldEqual, "P-TITLE alert-rule")
			convey.So(rendered.Content, convey.ShouldEqual, "P-CONTENT alert-rule")
		})

		convey.Convey("无精确渠道时使用 channel_type=\"*\" 通配行", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   "*",
				Language:      "zh-Hans",
				Title:         "CH-WILD-TITLE {{.RuleName}}",
				Content:       "CH-WILD-CONTENT",
				Source:        core.TemplateSourceDefault,
			})

			target := renderTestTarget("default", "zh-Hans")
			msg := testMessage()
			rendered, err := svc.renderMessage(context.Background(), target, TypeWebhook, &msg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldEqual, "CH-WILD-TITLE alert-rule")
			convey.So(rendered.Content, convey.ShouldEqual, "CH-WILD-CONTENT")
		})

		convey.Convey("无精确语言时使用 language=\"*\" 通配行", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "*",
				Title:         "LANG-WILD-TITLE {{.RuleName}}",
				Content:       "LANG-WILD-CONTENT",
				Source:        core.TemplateSourceDefault,
			})

			target := renderTestTarget("default", "zh-Hans")
			msg := testMessage()
			rendered, err := svc.renderMessage(context.Background(), target, TypeWebhook, &msg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldEqual, "LANG-WILD-TITLE alert-rule")
			convey.So(rendered.Content, convey.ShouldEqual, "LANG-WILD-CONTENT")
		})

		convey.Convey("无持久化行时回退 locales 代码模板", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			target := renderTestTarget("empty", "zh-Hans")
			msg := testMessage()
			rendered, err := svc.renderMessage(context.Background(), target, TypeWebhook, &msg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldContainSubstring, "标签事件告警")
		})

		convey.Convey("持久化模板支持 {{range}} 渲染", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "zh-Hans",
				Title:         "RANGE-TITLE",
				Content:       "{{range .Hits}}[{{.Name}}]{{end}}",
				Source:        core.TemplateSourceDefault,
			})

			msg := Message{
				Template: TemplateTagEventAlert,
				Params:   map[string]any{"Hits": []map[string]any{{"Name": "a"}, {"Name": "b"}}},
			}
			target := renderTestTarget("default", "zh-Hans")
			rendered, err := svc.renderMessage(context.Background(), target, TypeWebhook, &msg)
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldEqual, "RANGE-TITLE")
			convey.So(rendered.Content, convey.ShouldEqual, "[a][b]")
		})
	})
}

// TestService_RenderPreview 测试模板渲染预览：成功时返回标题/正文，未知事件返回错误。
// TestService_RenderPreview tests the template render preview: success returns title/content,
// and an unknown event returns an error.
func TestService_RenderPreview(t *testing.T) {
	convey.Convey("模板渲染预览", t, func() {
		convey.Convey("命中持久化模板时渲染标题与正文", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			seedTemplate(t, db, &core.NotificationTemplate{
				NamespaceName: "default",
				Event:         core.EventTypeTagEventRecord,
				ChannelType:   TypeWebhook,
				Language:      "zh-Hans",
				Title:         "PREVIEW-TITLE {{.RuleName}}",
				Content:       "PREVIEW-CONTENT {{.RuleName}}",
				Source:        core.TemplateSourceDefault,
			})

			rendered, err := svc.RenderPreview(context.Background(), "default", core.EventTypeTagEventRecord, TypeWebhook, "zh-Hans", map[string]any{"RuleName": "alert-rule"})
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldEqual, "PREVIEW-TITLE alert-rule")
			convey.So(rendered.Content, convey.ShouldEqual, "PREVIEW-CONTENT alert-rule")
		})

		convey.Convey("无持久化行时回退规格默认（locales 文本）", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			rendered, err := svc.RenderPreview(context.Background(), "empty", core.EventTypeTagEventRecord, TypeWebhook, "zh-Hans", map[string]any{"RuleName": "alert-rule"})
			convey.So(err, convey.ShouldBeNil)
			convey.So(rendered.Title, convey.ShouldContainSubstring, "标签事件告警")
		})

		convey.Convey("未知事件返回错误", func() {
			db := newServiceTestDB(t)
			svc := newTestService(db)
			rendered, err := svc.RenderPreview(context.Background(), "default", "no_such_event", TypeWebhook, "zh-Hans", nil)
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(rendered, convey.ShouldBeNil)
		})
	})
}
