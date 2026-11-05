// Package interaction 派生会话交互状态（等待 / 运行 / 暂停 / 锁定）与子状态，并提供操作守卫
// 判定。本包只依赖 internal/models 与 internal/pkg，是纯规则层，不读写数据库、不发起模型调用。
package interaction
