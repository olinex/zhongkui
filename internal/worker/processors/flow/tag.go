// Package flow 实现基于 TCP 流的分布式抓包处理。
// 包含从 ClickHouse 轮询数据、TCP 流重组、批量输出到 ClickHouse 的完整流水线。
package flow

import (
	"context"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// tagStage 基于 tagengine 的标签识别数据处理阶段。
// 位于 routerStage 之后、批量聚合之前，仅处理 *network.HttpTransport：
// 按本流程全局写死的标签识别组合关系（目标类型 = HttpTransport，标签维度 = tagengine.TagTypeRisk）
// 调用引擎识别并将结果写入 Tags 字段，NetworkStack 等其余产物原样透传。
// 该组合与 tag_rules 表的 target_type + tag_type 分组约定对应，业务逻辑写死，不接受运行时配置。
// 标签识别为尽力而为（best-effort）：识别失败仅记录告警日志并透传，
// 不中断流水线，避免标签规则库异常导致抓包数据入库流程整体停止。
type tagStage struct {
	engine *tagengine.Engine
	logger *logging.Logger
}

// Process 实现 etl.Stage[any, any] 接口。
// 消费 routerStage 产出的 any 项，HttpTransport 打标签后写入 out，
// 其余类型透传，保持聚合产物类型不变。
func (s *tagStage) Process(ctx context.Context, in <-chan any, out chan<- any) error {
	defer close(out)
	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(ctx.Err())
		case v, ok := <-in:
			if !ok {
				return nil
			}
			if transport, ok := v.(*network.HttpTransport); ok {
				s.recognize(ctx, transport)
			}
			select {
			case out <- v:
			case <-ctx.Done():
				return errors.WithStack(ctx.Err())
			}
		}
	}
}

// recognize 对单个 HttpTransport 执行标签识别并写入 Tags 字段。
// 识别失败时记录告警日志并保留原值，不中断流水线。
func (s *tagStage) recognize(ctx context.Context, transport *network.HttpTransport) {
	tags, err := s.engine.Recognize(ctx, transport, tagengine.TagTypeRisk)
	if err != nil {
		s.logger.Ctx(ctx).Warnf("flow tag recognize failed, tagType=%s, err=%v", tagengine.TagTypeRisk, err)
		return
	}
	transport.Tags = tags
}
