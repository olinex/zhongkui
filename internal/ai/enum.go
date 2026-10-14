// 本文件把 dry-run 评估的阶段、阶段状态与改进建议归类、客户端工具调用的审核重要级注册为枚举类
// evalStage / evalStageStatus / diagnosisCategory / auditLevel。取值空间的唯一来源是本包的
// 类型化常量（评估三类的常量在 eval.go，审核重要级的常量在 audit.go），故不重复声明字面量：取值
// 序列按常量声明顺序就地复用为条目来源，条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的
// EnumClass 成员、i18n 键 enum.<类名>.<key> 以及消费方引用一致。助手页的工具调用阶段
// （toolCall / toolResult）与知识库页的阶段同属 EvalStage 类型，共用 evalStage 一个枚举类。
// 长期记忆类别/状态（memoryKind / memoryStatus）的枚举类注册已随其类型迁至 internal/models/ai
// （见该包的 enum.go 与 memory.go），本包统一引用该包的类型与常量。

package ai

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// evalStageValues 是 evalStage 枚举类的取值空间，声明顺序即枚举类条目顺序。
var evalStageValues = []EvalStage{
	EvalStageRetrieve,
	EvalStageAnswer,
	EvalStageChunkRelevance,
	EvalStageClaims,
	EvalStageReverseQuestions,
	EvalStageSuggestions,
	EvalStageToolCall,
	EvalStageToolResult,
}

// evalStageStatusValues 是 evalStageStatus 枚举类的取值空间，声明顺序即枚举类条目顺序。
var evalStageStatusValues = []EvalStageStatus{
	EvalStatusStart,
	EvalStatusDone,
	EvalStatusFailed,
}

// diagnosisCategoryValues 是 diagnosisCategory 枚举类的取值空间，声明顺序即枚举类条目顺序。
var diagnosisCategoryValues = []DiagnosisCategory{
	DiagnosisCategoryCoverage,
	DiagnosisCategoryRanking,
	DiagnosisCategoryGeneration,
	DiagnosisCategoryContext,
	DiagnosisCategoryOther,
}

// auditLevelValues 是 auditLevel 枚举类的取值空间，声明顺序即枚举类条目顺序。
var auditLevelValues = []AuditLevel{
	AuditLevelMust,
	AuditLevelSuggest,
	AuditLevelAuto,
	AuditLevelNone,
}

// init 注册枚举类 evalStage / evalStageStatus / diagnosisCategory / auditLevel（取值空间固定，条目
// 在注册时快照）。评估阶段、阶段状态与改进建议归类的取值常量同处 eval.go，审核重要级的取值常量
// 同处 audit.go，均保持单一来源。
func init() {
	enum.Register(enum.Type{Name: "evalStage", Entries: enum.EntriesOf(evalStageValues)})
	enum.Register(enum.Type{Name: "evalStageStatus", Entries: enum.EntriesOf(evalStageStatusValues)})
	enum.Register(enum.Type{Name: "diagnosisCategory", Entries: enum.EntriesOf(diagnosisCategoryValues)})
	enum.Register(enum.Type{Name: "auditLevel", Entries: enum.EntriesOf(auditLevelValues)})
}
