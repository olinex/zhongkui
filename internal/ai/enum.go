// 本文件把 dry-run 评估的阶段、阶段状态与改进建议归类注册为枚举类 evalStage / evalStageStatus /
// diagnosisCategory。三个取值空间的唯一来源是本包 eval.go 内的类型化常量，故此处不重复声明字面量：
// 取值序列按常量声明顺序就地复用为条目来源，条目在包内 init 注册时快照。类名须与前端
// utils/enums.ts 的 EnumClass 成员、i18n 键 enum.<类名>.<key> 以及消费方引用一致。助手页的工具调用
// 阶段（toolCall / toolResult）与知识库页的阶段同属 EvalStage 类型，共用 evalStage 一个枚举类。
//
// This file registers the dry-run evaluation stages, stage statuses and improvement suggestion
// categories as the enum classes evalStage / evalStageStatus / diagnosisCategory. The single source of
// all three value spaces is the typed constants in eval.go of this package, so no literal is repeated
// here: each value sequence is reused in constant declaration order as the entry source and the entries
// are snapshotted by an in-package init. The class names must match the frontend EnumClass members in
// utils/enums.ts, the enum.<class>.<key> i18n keys and the references of consumers. The tool-call stages
// of the assistant page (toolCall / toolResult) belong to the same EvalStage type as the knowledge page
// stages and share the single evalStage class.
package ai

import (
	// internal
	"zhongkui.com/zhongkui/internal/pkg/enum"
)

// evalStageValues 是 evalStage 枚举类的取值空间，声明顺序即枚举类条目顺序。
// evalStageValues is the value space of the evalStage enum class; the declaration order is the entry
// order of the enum class.
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
// evalStageStatusValues is the value space of the evalStageStatus enum class; the declaration order is
// the entry order of the enum class.
var evalStageStatusValues = []EvalStageStatus{
	EvalStatusStart,
	EvalStatusDone,
	EvalStatusFailed,
}

// diagnosisCategoryValues 是 diagnosisCategory 枚举类的取值空间，声明顺序即枚举类条目顺序。
// diagnosisCategoryValues is the value space of the diagnosisCategory enum class; the declaration order
// is the entry order of the enum class.
var diagnosisCategoryValues = []DiagnosisCategory{
	DiagnosisCategoryCoverage,
	DiagnosisCategoryRanking,
	DiagnosisCategoryGeneration,
	DiagnosisCategoryContext,
	DiagnosisCategoryOther,
}

// init 注册枚举类 evalStage / evalStageStatus / diagnosisCategory（取值空间固定，条目在注册时快照）。
// init registers the enum classes evalStage / evalStageStatus / diagnosisCategory (fixed value spaces,
// their entries snapshotted at registration time).
func init() {
	enum.Register(enum.Type{Name: "evalStage", Entries: enum.EntriesOf(evalStageValues)})
	enum.Register(enum.Type{Name: "evalStageStatus", Entries: enum.EntriesOf(evalStageStatusValues)})
	enum.Register(enum.Type{Name: "diagnosisCategory", Entries: enum.EntriesOf(diagnosisCategoryValues)})
}
