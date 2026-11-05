// 本文件把 dry-run 评估的改进建议归类注册为枚举类 diagnosisCategory。取值空间的唯一来源是本包
// 的类型化常量（见 dryrun.go），条目在包内 init 注册时快照。类名须与前端 utils/enums.ts 的
// EnumClass 成员、i18n 键 enum.<类名>.<key> 以及消费方引用一致。

package knowledge

import (
	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/enum"
)

// diagnosisCategoryValues 是 diagnosisCategory 枚举类的取值空间，声明顺序即枚举类条目顺序。
var diagnosisCategoryValues = []DiagnosisCategory{
	DiagnosisCategoryCoverage,
	DiagnosisCategoryRanking,
	DiagnosisCategoryGeneration,
	DiagnosisCategoryContext,
	DiagnosisCategoryOther,
}

// init 注册枚举类 diagnosisCategory（取值空间固定，条目在注册时快照）。
func init() {
	enum.Register(enum.Type{Name: "diagnosisCategory", Entries: enum.EntriesOf(diagnosisCategoryValues)})
}
