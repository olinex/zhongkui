// Package audit 承载工具调用审核分级的策略与批量评估器：分级规则唯一实现在 NeedsHumanReview，
// 评估由独立的闪存档位模型调用完成，与 Agent 执行管线解耦。
package audit

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// NeedsHumanReview 判定一次工具调用是否需要人工审核：以审核重要级（level）与会话级审核阀门
// （valve）合成，规则严格遵循判定矩阵（人工=需人工审核，自动=可自动执行）：
//
//	重要级 \ 阀门   each   auto   none
//	must            人工   人工   人工
//	suggest         人工   人工   自动
//	auto            人工   自动   自动
//	none            人工   自动   自动
//
// 即：must 为绝对下界，任何阀门下均需人工；each 为绝对下界，任何重要级下均需人工；
// auto 阀门下仅 auto / none 重要级可自动；none 阀门下仅 must 需人工。
func NeedsHumanReview(level contract.AuditLevel, valve modelsai.ReviewValve) bool {
	return level == contract.AuditLevelMust || valve == modelsai.ReviewValveEach ||
		(valve == modelsai.ReviewValveAuto && level != contract.AuditLevelAuto && level != contract.AuditLevelNone)
}
