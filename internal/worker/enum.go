package worker

import "zhongkuisec.com/zhongkui/internal/pkg/enum"

// taskTypeEnumName 任务类型枚举类名。
// 与视图层 search.toml 的 set 引用、前端 EnumClass.taskType 保持一致。
const taskTypeEnumName = "taskType"

// init 注册 taskType 枚举类：取值空间直接取自 worker 的任务处理器注册表，
// 每次读取时经 TaskTypes() 实时派生，避免枚举清单与消费侧登记清单漂移。
func init() {
	enum.RegisterSource(taskTypeEnumName, func() []enum.Entry {
		return enum.EntriesOf(TaskTypes())
	})
}
