// Package periodic 提供周期性报告计划的领域层：负责报告计划与运行记录的持久化、计划资格校验、
// 下次触发时间计算与历史清理，不承载 Agent 生成逻辑（报告生成由调用方经 asynq 任务驱动）。
//
// 文件分工：
//   - types.go 定义领域常量、错误哨兵与输入/视图值类型；
//   - store.go 提供报告计划、运行记录与关注关系的持久化读写，全部以命名空间为隔离维度；
//   - scheduler.go 计算计划的下一次 cron 触发时间，并把触发点投递为 periodic-report:plan:run 任务；
//   - cleanup.go 执行到期计划停用与超期报告对象的清理。
//
// 边界与不变量：计划读写在 SQL 层以 namespace_name 限定，跨命名空间一律按不存在处理；
// 运行记录以 (plan_id, trigger_at) 唯一约束保证同一计划同一触发点只落一条记录，重复投递
// 由 CreateRunIfAbsent 幂等吸收；分发器对到期计划最多补投一次、不补历史积压，避免停机后
// 一次性补生成大量历史报告。
package periodic
