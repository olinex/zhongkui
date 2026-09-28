package notify

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	// 第三方
	"github.com/hibiken/asynq"
	"github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkui.com/zhongkui/internal/models/core"
	notifybase "zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etl"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	message "zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
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

// fakeTaskEnqueuer 记录被投递任务的测试替身，用于断言组任务扇出的逐目标直投任务。
type fakeTaskEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
}

// EnqueueContext 记录任务并返回 (nil, nil)。
func (f *fakeTaskEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks = append(f.tasks, task)
	return nil, nil
}

// tasks 返回按投递顺序排好的任务快照。
func (f *fakeTaskEnqueuer) tasksSnapshot() []*asynq.Task {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*asynq.Task, len(f.tasks))
	copy(out, f.tasks)
	return out
}

// TestNewSendGroupTask 测试通知组发送任务的编码。
func TestNewSendGroupTask(t *testing.T) {
	convey.Convey("NewSendGroupTask 编码通知组发送任务", t, func() {
		convey.Convey("合法输入编码成功且类型为 notify:send:group", func() {
			gid := uint(1)
			task, err := NewSendGroupTask(notifybase.SendGroupInput{
				Namespace: "default",
				GroupID:   &gid,
				EventType: "tag_event_record",
				EventID:   42,
				Message:   message.Message{Template: notifybase.TemplateTagEventAlert, Params: map[string]any{"RuleName": "r"}},
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(task, convey.ShouldNotBeNil)
			convey.So(task.Type(), convey.ShouldEqual, SendGroupTask)
			var decoded notifybase.SendGroupInput
			convey.So(json.Unmarshal(task.Payload(), &decoded), convey.ShouldBeNil)
			convey.So(decoded.Namespace, convey.ShouldEqual, "default")
			convey.So(decoded.GroupID, convey.ShouldNotBeNil)
			convey.So(decoded.EventType, convey.ShouldEqual, "tag_event_record")
			convey.So(decoded.EventID, convey.ShouldEqual, 42)
			convey.So(decoded.Message.Template, convey.ShouldEqual, notifybase.TemplateTagEventAlert)
			convey.So(decoded.Message.Params["RuleName"], convey.ShouldEqual, "r")
		})

		convey.Convey("缺失命名空间返回错误", func() {
			_, err := NewSendGroupTask(notifybase.SendGroupInput{EventType: "tag_event_record", EventID: 1})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失事件类型返回错误", func() {
			_, err := NewSendGroupTask(notifybase.SendGroupInput{Namespace: "default", EventID: 1})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失消息模板返回错误", func() {
			_, err := NewSendGroupTask(notifybase.SendGroupInput{Namespace: "default", EventType: "tag_event_record", EventID: 1, Message: message.Message{Params: map[string]any{"RuleName": "r"}}})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失事件 ID 返回错误", func() {
			_, err := NewSendGroupTask(notifybase.SendGroupInput{Namespace: "default", EventType: "tag_event_record", Message: message.Message{Template: notifybase.TemplateTagEventAlert}})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失消息返回错误", func() {
			_, err := NewSendGroupTask(notifybase.SendGroupInput{Namespace: "default", EventType: "tag_event_record", EventID: 1})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestNewSendTargetTask 测试单目标直投任务的编码。
func TestNewSendTargetTask(t *testing.T) {
	convey.Convey("NewSendTargetTask 编码单目标直投任务", t, func() {
		convey.Convey("合法输入编码成功、类型为 notify:send:target 且 TargetID 往返一致", func() {
			task, err := NewSendTargetTask(notifybase.SendTargetInput{
				Namespace: "default",
				EventType: "tag_event_record",
				EventID:   42,
				TargetID:  7,
				Message:   message.Message{Template: notifybase.TemplateTagEventAlert, Params: map[string]any{"RuleName": "r"}},
			})
			convey.So(err, convey.ShouldBeNil)
			convey.So(task, convey.ShouldNotBeNil)
			convey.So(task.Type(), convey.ShouldEqual, SendTargetTask)
			var decoded notifybase.SendTargetInput
			convey.So(json.Unmarshal(task.Payload(), &decoded), convey.ShouldBeNil)
			convey.So(decoded.Namespace, convey.ShouldEqual, "default")
			convey.So(decoded.EventType, convey.ShouldEqual, "tag_event_record")
			convey.So(decoded.EventID, convey.ShouldEqual, 42)
			convey.So(decoded.TargetID, convey.ShouldEqual, 7)
			convey.So(decoded.Message.Template, convey.ShouldEqual, notifybase.TemplateTagEventAlert)
		})

		convey.Convey("缺失命名空间返回错误", func() {
			_, err := NewSendTargetTask(notifybase.SendTargetInput{
				EventType: "tag_event_record", EventID: 1, TargetID: 1,
				Message: message.Message{Template: notifybase.TemplateTagEventAlert},
			})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失事件类型返回错误", func() {
			_, err := NewSendTargetTask(notifybase.SendTargetInput{
				Namespace: "default", EventID: 1, TargetID: 1,
				Message: message.Message{Template: notifybase.TemplateTagEventAlert},
			})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失目标 ID 返回错误", func() {
			_, err := NewSendTargetTask(notifybase.SendTargetInput{
				Namespace: "default", EventType: "tag_event_record", EventID: 1,
				Message: message.Message{Template: notifybase.TemplateTagEventAlert},
			})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestSendTargetHandler_Do 测试单目标直投处理器的载荷解析。
func TestSendTargetHandler_Do(t *testing.T) {
	convey.Convey("SendTargetHandler 载荷解析", t, func() {
		// 载荷非法路径不会触及 service/logger，可传 nil 直接验证错误分支
		handler := &SendTargetHandler{service: nil}

		convey.Convey("非法 JSON 载荷返回错误", func() {
			err := handler.Do(context.Background(), &fakeETLTask{typ: SendTargetTask, payload: []byte(`{invalid`)})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// TestSendGroupHandler_Do 测试通知组发送处理器的载荷解析。
func TestSendGroupHandler_Do(t *testing.T) {
	convey.Convey("SendGroupHandler 载荷解析", t, func() {
		// 载荷非法路径不会触及 service/logger/client，可传 nil 直接验证错误分支
		handler := &SendGroupHandler{service: nil}

		convey.Convey("非法 JSON 载荷返回错误", func() {
			err := handler.Do(context.Background(), &fakeETLTask{typ: SendGroupTask, payload: []byte(`{invalid`)})
			convey.So(err, convey.ShouldNotBeNil)
		})

		convey.Convey("缺失命名空间或事件类型返回错误", func() {
			err := handler.Do(context.Background(), &fakeETLTask{typ: SendGroupTask, payload: []byte(`{"eventID":1}`)})
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

// ---- 以下为 SendGroupHandler / SendTargetHandler 经真实 service 的全链路测试 ----
// 测试辅助（newServiceTestDB/seedWebhook/testMessage）与 internal/notify 包内的同名
// helper 相互独立，此处按需复制以保持 handlers/notify 包的自包含性。

var initTaskTestKeyOnce sync.Once

// initTaskTestKeyDerivation 初始化全局密钥派生：渠道/目标配置经 crypto 序列化器加密存储，
// 测试须与生产一致地派生密钥，否则写库会报 KeyDerivation not initialized。
func initTaskTestKeyDerivation() {
	initTaskTestKeyOnce.Do(func() {
		provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
		book, err := key.LoadPasswordBook("")
		if err != nil {
			panic("failed to load password book: " + err.Error())
		}
		key.InitGlobalKeyDerivation(provider, book)
	})
}

// newServiceTestDB 创建共享已迁移的 PostgreSQL 测试库（testdb.Core）。
func newServiceTestDB(t *testing.T) *database.DB {
	t.Helper()
	initTaskTestKeyDerivation()
	return testdb.Core(t)
}

// newTestService 创建测试用的通知服务实例。
func newTestService(db *database.DB) *notifybase.Service {
	logger, _ := logging.NewDefaultLogger()
	return notifybase.NewService(db, logger, message.NewPool())
}

// ensureTestNamespace 确保命名空间在测试库中存在（FirstOrCreate 幂等创建），
// 以满足 notification_* 表上 namespace_name 的外键约束。
func ensureTestNamespace(db *database.DB, namespace string) {
	db.FirstOrCreate(&core.Namespace{Name: namespace})
}

// seedWebhook 在指定命名空间下构造一个启用状态的 webhook 渠道、目标与默认通知组，
// 并将目标加入该组；返回组与目标实体（含 ID）。
func seedWebhook(t *testing.T, db *database.DB, namespace, url string) (*core.NotificationGroup, *core.NotificationTarget) {
	t.Helper()
	ensureTestNamespace(db, namespace)
	channel := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          "webhook-channel",
		Type:          message.TypeWebhook,
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
		Type:          message.TypeWebhook,
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
func seedWebhookTarget(t *testing.T, db *database.DB, namespace, url string, groupID uint, seq int) *core.NotificationTarget {
	t.Helper()
	ensureTestNamespace(db, namespace)
	channel := &core.NotificationChannel{
		NamespaceName: namespace,
		Name:          fmt.Sprintf("webhook-channel-%d-%d", groupID, seq),
		Type:          message.TypeWebhook,
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
		Type:          message.TypeWebhook,
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

// testMessage 构造一个基于真实模板键 tag_event.alert 的结构化通知消息，
// 使全链路发送经由 locales 模板渲染出具体文案（而非代码写死的标题与正文）。
func testMessage() message.Message {
	return message.Message{
		Template: notifybase.TemplateTagEventAlert,
		Params:   map[string]any{"Namespace": "default", "RuleName": "alert-rule", "DetectedAt": "2026-01-01T00:00:00Z"},
	}
}

// TestSendGroupHandler_Full 测试 SendGroupHandler 经真实 service 将通知组扇出为逐目标
// 直投任务的完整链路。
func TestSendGroupHandler_Full(t *testing.T) {
	convey.Convey("SendGroupHandler 完整扇出链路", t, func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		db := newServiceTestDB(t)
		group, targetA := seedWebhook(t, db, "default", srv.URL)
		targetB := seedWebhookTarget(t, db, "default", srv.URL, group.ID, 2)
		logger, _ := logging.NewDefaultLogger()
		client := &fakeTaskEnqueuer{}
		handler := &SendGroupHandler{service: newTestService(db), client: client, logger: logger}
		gid := group.ID
		payload, _ := json.Marshal(notifybase.SendGroupInput{
			Namespace: "default",
			GroupID:   &gid,
			EventType: core.EventTypeTagEventRecord,
			EventID:   200,
			Message:   testMessage(),
		})

		err := handler.Do(context.Background(), &fakeETLTask{typ: SendGroupTask, payload: payload})
		convey.So(err, convey.ShouldBeNil)

		tasks := client.tasksSnapshot()
		convey.So(len(tasks), convey.ShouldEqual, 2)
		var gotIDs []uint
		for _, task := range tasks {
			convey.So(task.Type(), convey.ShouldEqual, SendTargetTask)
			var in notifybase.SendTargetInput
			convey.So(json.Unmarshal(task.Payload(), &in), convey.ShouldBeNil)
			convey.So(in.Namespace, convey.ShouldEqual, "default")
			convey.So(in.EventType, convey.ShouldEqual, core.EventTypeTagEventRecord)
			convey.So(in.EventID, convey.ShouldEqual, 200)
			gotIDs = append(gotIDs, in.TargetID)
		}
		convey.So(gotIDs, convey.ShouldResemble, []uint{targetA.ID, targetB.ID})
	})
}

// TestSendTargetHandler_Full 测试 SendTargetHandler 经真实 service 的完整直投投递链路：
// 析取目标发送并写入发送记录。
func TestSendTargetHandler_Full(t *testing.T) {
	convey.Convey("SendTargetHandler 完整直投链路", t, func() {
		reqs := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqs++
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		db := newServiceTestDB(t)
		_, target := seedWebhook(t, db, "default", srv.URL)
		handler := &SendTargetHandler{service: newTestService(db)}
		payload, _ := json.Marshal(notifybase.SendTargetInput{
			Namespace: "default",
			EventType: core.EventTypeTagEventRecord,
			EventID:   300,
			TargetID:  target.ID,
			Message:   testMessage(),
		})

		err := handler.Do(context.Background(), &fakeETLTask{typ: SendTargetTask, payload: payload})
		convey.So(err, convey.ShouldBeNil)
		convey.So(reqs, convey.ShouldEqual, 1)

		var record core.NotificationRecord
		convey.So(db.Select("status", "group_id", "target_id").
			Where("event_type = ? AND event_id = ? AND target_id = ?", core.EventTypeTagEventRecord, 300, target.ID).
			First(&record).Error, convey.ShouldBeNil)
		convey.So(record.Status, convey.ShouldEqual, core.RecordStatusSuccess)
		convey.So(record.GroupID, convey.ShouldEqual, 0)
		convey.So(record.TargetID, convey.ShouldEqual, target.ID)
	})
}
