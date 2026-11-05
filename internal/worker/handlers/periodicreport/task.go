// Package periodicreport 提供周期性报告计划的 asynq 任务处理器。
// DispatchHandler 消费 periodic-report:plan:dispatch 定时任务：每分钟扫描到期计划并投递 periodic-report:plan:run；
// RunHandler 消费 periodic-report:plan:run 任务：按触发时间点加载计划与技能，调用 Agent 生成 Markdown
// 报告并写入对象存储，最终收敛运行记录状态；CleanupHandler 消费 periodic-report:run:cleanup 定时任务：
// 按保留天数清理过期报告对象与运行记录，并停用已到期计划。
package periodicreport

import (
	// 标准库
	"encoding/json"
	"math"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// TaskPlanRun 报告计划生成任务类型，与领域层声明的处理器常量保持一致；
	// 统一调度器按每个计划的 etcd 配置投递，同一计划同一触发时间点只生成一份报告。
	TaskPlanRun = periodic.TaskHandler

	// TaskRunCleanup 报告生命周期清理任务类型，由 asynq 周期调度器按 cron 表达式投递；
	// 清理到期计划与超过保留期的报告对象及其运行记录，天然幂等。
	TaskRunCleanup = "periodic-report:run:cleanup"
)

// parsePlanID 从游标任务载荷的固定参数中解析计划 ID，兼容 JSON 反序列化产生的 float64、
// 显式 json.Number、字符串与无符号整数等取值；缺失、非整数或非正整数时返回英文错误。
func parsePlanID(payload map[string]any) (uint, error) {
	raw, ok := payload["planId"]
	if !ok || raw == nil {
		return 0, errors.New("periodic report plan run: missing planId in task payload")
	}
	switch v := raw.(type) {
	case float64:
		if v <= 0 || v != math.Trunc(v) {
			return 0, errors.Newf("periodic report plan run: invalid planId %v", v)
		}
		return uint(v), nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil || parsed <= 0 {
			return 0, errors.Newf("periodic report plan run: invalid planId %q", v.String())
		}
		return uint(parsed), nil
	case string:
		trimmed := strings.TrimSpace(v)
		parsed, err := strconv.ParseUint(trimmed, 10, 64)
		if err != nil || parsed == 0 {
			return 0, errors.Newf("periodic report plan run: invalid planId %q", v)
		}
		return uint(parsed), nil
	case uint:
		if v == 0 {
			return 0, errors.New("periodic report plan run: invalid planId 0")
		}
		return v, nil
	case uint64:
		if v == 0 {
			return 0, errors.New("periodic report plan run: invalid planId 0")
		}
		return uint(v), nil
	case int:
		if v <= 0 {
			return 0, errors.Newf("periodic report plan run: invalid planId %d", v)
		}
		return uint(v), nil
	case int64:
		if v <= 0 {
			return 0, errors.Newf("periodic report plan run: invalid planId %d", v)
		}
		return uint(v), nil
	default:
		return 0, errors.Newf("periodic report plan run: unsupported planId type %T", raw)
	}
}
