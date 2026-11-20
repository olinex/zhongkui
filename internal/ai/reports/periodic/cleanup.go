package periodic

import (
	// 标准库
	"context"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// defaultCleanupPerPlanLimit 是清理流程单次为每个计划处理的运行记录默认上限。
const defaultCleanupPerPlanLimit = 200

// ObjectRemover 抽象报告对象存储的删除能力，生产实现为 *s3.ObjectCache。
type ObjectRemover interface {
	// Remove 删除指定对象键对应的报告对象；cryptoKey 须与写入时一致。
	Remove(ctx context.Context, objectKey string, cryptoKey []byte) error
}

// SessionRemover 抽象报告运行会话的删除能力，生产实现为 *session.Store。报告每次运行使用
// 独立会话，运行记录按保留天数清理时须一并删除其会话，避免会话与消息无限累积。
type SessionRemover interface {
	// DeleteSession 删除指定命名空间与属主下的会话及其从属数据。
	DeleteSession(ctx context.Context, id uint, namespaceName string, userID uint) error
}

// CleanupResult 是一次清理流程的累计结果。
type CleanupResult struct {
	// ExpiredPlans 被自动停用的到期计划数
	ExpiredPlans int
	// RemovedObjects 被删除的报告对象数
	RemovedObjects int
	// RemovedRuns 被软删除的运行记录数
	RemovedRuns int
}

// Cleanup 执行到期计划停用与超期报告清理：先把到期计划停用（不再需要清理调度配置，分发器
// 只挑选启用的计划），再对全部计划按保留天数清理已结束的运行记录（先删对象、再删本次运行的
// 独立会话、最后软删记录；对象或会话删除失败时保留记录待下轮）。单项失败不中断整体流程，
// 最终返回首个错误；perPlanLimit 非正时取默认上限。
func (s *Store) Cleanup(ctx context.Context, now time.Time, objects ObjectRemover, sessions SessionRemover, perPlanLimit int) (CleanupResult, error) {
	var result CleanupResult
	var firstErr error
	recordErr := func(err error) {
		if firstErr == nil && err != nil {
			firstErr = errors.WithStack(err)
		}
	}
	expiredPlans, err := s.ListExpiredPlans(ctx, now)
	if err != nil {
		return result, err
	}
	for _, plan := range expiredPlans {
		if _, err := s.SetPlanEnabled(ctx, plan.NamespaceName, plan.ID, false); err != nil {
			recordErr(err)
			continue
		}
		result.ExpiredPlans++
	}
	plans, err := s.ListAllPlans(ctx)
	if err != nil {
		return result, err
	}
	if perPlanLimit <= 0 {
		perPlanLimit = defaultCleanupPerPlanLimit
	}
	// 报告对象的加密密钥由 object_key 字段的 s3crypto 标签派生，删除须与写入使用同一密钥
	reportKey, err := (&ai.PeriodicReportRun{}).ObjectCryptoKey()
	if err != nil {
		return result, errors.WithStack(err)
	}
	for _, plan := range plans {
		if plan.RetentionDays <= 0 {
			continue
		}
		threshold := now.AddDate(0, 0, -plan.RetentionDays)
		runs, err := s.ListExpiredRuns(ctx, plan.ID, threshold, perPlanLimit)
		if err != nil {
			recordErr(err)
			continue
		}
		for _, run := range runs {
			if strings.TrimSpace(run.ObjectKey.Key()) != "" {
				if objects == nil {
					recordErr(errors.New("periodic-report cleanup: nil object remover"))
					continue
				}
				if err := objects.Remove(ctx, run.ObjectKey.Key(), reportKey); err != nil {
					// 对象删除失败时保留记录待下轮重试，不因此中断清理流程。
					continue
				}
				result.RemovedObjects++
			}
			if run.SessionID != nil && *run.SessionID != 0 {
				if sessions == nil {
					recordErr(errors.New("periodic-report cleanup: nil session remover"))
					continue
				}
				if err := sessions.DeleteSession(ctx, *run.SessionID, run.NamespaceName, plan.UserID); err != nil {
					// 会话删除失败时保留记录待下轮重试，避免留下无主会话。
					recordErr(err)
					continue
				}
			}
			if err := s.DeleteRun(ctx, run.ID); err != nil {
				recordErr(err)
				continue
			}
			result.RemovedRuns++
		}
	}
	return result, firstErr
}
