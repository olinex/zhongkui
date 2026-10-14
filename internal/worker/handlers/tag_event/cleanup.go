package tag_event

import (
	// 标准库
	"context"
	"encoding/json"
	"time"

	// 第三方

	"github.com/creasty/defaults"
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/provider"
)

const (
	// TagEventCleanupTask 标签事件无效记录清理定时任务类型，由 asynq 周期调度器按 cron 表达式投递。
	// 无需游标语义：清理不使用触发时间点（截止时间由 time.Now 计算），且天然幂等，
	// 因此作为普通定时任务调度即可。定期删除 status=suppressed 且已超过保留期的标签事件记录
	// （含其命中明细，数据库级联删除）。
	TagEventCleanupTask = "tag:event:cleanup"
)

// cleanupTaskPayload 清理任务载荷，保留期随任务调度参数一并维护于 etcd 的 scheduler/cfg 配置。
type cleanupTaskPayload struct {
	// Retention 清理保留期；suppressed 事件 detected_at 早于"当前时间-保留期"即被删除。
	// 缺省（未声明或非正值）为 default 标签声明的 7 天。
	Retention serializer.DurationNS `json:"retention" default:"168h"`
}

// EventCleanupHandler 标签事件无效记录清理处理器，消费 tag:event:cleanup 任务。
// 语义：suppressed 状态仅表示"同一事件键在抑制窗口内被抑制、从未向用户发送通知"，
// 属无效数据可安全清理；已通知后被自动/手动关闭的事件状态为 closed，是真实验告警历史，予以保留。
type EventCleanupHandler struct {
	coreDB *database.DB    // core 库，删除无效标签事件
	logger *logging.Logger // 日志记录器
}

// 编译期断言：确保 EventCleanupHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*EventCleanupHandler)(nil)

// NewEventCleanupHandler 从 do.Injector 解析依赖并创建无效记录清理处理器。
func NewEventCleanupHandler(i do.Injector) *EventCleanupHandler {
	coreDB := provider.MustInvokeCoreDatabase(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	return &EventCleanupHandler{coreDB: coreDB, logger: logger}
}

// Do 执行一次清理：解析任务载荷中的保留期（缺失或非正值时用默认 7 天），
// 单条 DELETE 删除 status=suppressed 且 detected_at 早于截止时间的记录，命中明细经数据库外键级联删除。
func (h *EventCleanupHandler) Do(ctx context.Context, task etl.Task) error {
	retention, err := parseCleanupRetention(task.Payload())
	if err != nil {
		// 解析失败时 parseCleanupRetention 已回退默认保留期，仅记录告警并继续清理
		h.logger.Ctx(ctx).Warnf("tag event cleanup: parse retention failed, use default: %v", err)
	}
	cutoff := time.Now().Add(-retention)
	res := h.coreDB.WithContext(ctx).
		Where("status = ? AND detected_at < ?", tag.EventStatusSuppressed, cutoff).
		Delete(&tag.TagEventRecord{})
	if res.Error != nil {
		return errors.WithStack(res.Error)
	}
	if res.RowsAffected > 0 {
		h.logger.Ctx(ctx).Infof("tag event cleanup: deleted %d suppressed events (retention=%s)", res.RowsAffected, retention)
	}
	return nil
}

// parseCleanupRetention 从任务载荷提取清理保留期；载荷缺失/解析失败/保留期非正值时回退默认值（7 天）。
// 默认值由 cleanupTaskPayload.Retention 的 default 标签声明，此处经 defaults.Set 取回，不重复字面量。
// 普通 asynq 定时任务的载荷即 sCfg.Payload（retention 位于顶层）；
// 兼容游标任务载荷（retention 嵌于 payload 子对象）以免被误配为游标模式。
func parseCleanupRetention(raw []byte) (time.Duration, error) {
	var fallback cleanupTaskPayload
	if err := defaults.Set(&fallback); err != nil {
		return 0, errors.WithStack(err)
	}
	defaultRetention := fallback.Retention.Std()
	if len(raw) == 0 {
		return defaultRetention, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return defaultRetention, errors.WithStack(err)
	}
	if inner, ok := m["payload"].(map[string]any); ok && len(inner) > 0 {
		m = inner
	}
	data, err := json.Marshal(m)
	if err != nil {
		return defaultRetention, errors.WithStack(err)
	}
	var p cleanupTaskPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return defaultRetention, errors.WithStack(err)
	}
	if p.Retention <= 0 {
		return defaultRetention, nil
	}
	return p.Retention.Std(), nil
}
