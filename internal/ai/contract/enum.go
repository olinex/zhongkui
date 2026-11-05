// 本文件把 dry-run 评估的阶段与阶段状态、工具调用的审核重要级注册为枚举类 evalStage /
// evalStageStatus / auditLevel。取值空间的唯一来源是本包的类型化常量，故不重复声明字面量：
// 取值序列按常量声明顺序就地复用为条目来源，条目在包内 init 注册时快照。类名须与前端
// utils/enums.ts 的 EnumClass 成员、i18n 键 enum.<类名>.<key> 以及消费方引用一致。助手页的
// 工具调用阶段（toolCall / toolResult）与知识库页的阶段同属 EvalStage 类型，共用 evalStage
// 一个枚举类。

package contract

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

// init 注册枚举类 evalStage / evalStageStatus / auditLevel（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "evalStage", Entries: enum.EntriesOf(evalStageValues)})
	enum.Register(enum.Type{Name: "evalStageStatus", Entries: enum.EntriesOf(evalStageStatusValues)})
	enum.Register(enum.Type{Name: "auditLevel", Entries: enum.EntriesOf(AuditLevels)})
}
