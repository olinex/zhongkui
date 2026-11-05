// 本文件是图表类内置工具 mermaid_check 的薄适配器：用官方 mermaid 解析器校验 Agent 生成的
// 源码，让它在把带图表的回答交给前端前先自检、按官方报错的行号与修改建议整改。JS 运行时由
// provider 管理并经 tools.Deps 注入（internal/pkg/jsengine），mermaid 只是其中一个注入项；
// 本文件只保留入参结构体、工具描述与调用薄函数。工具元信息（internal/ai/config 的 ToolSpecs）
// 与出厂行（002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
	"zhongkuisec.com/zhongkui/internal/pkg/mermaid"
)

// mermaidCheckArgs 是 mermaid_check 的 JSON 入参形态：code 必填。
type mermaidCheckArgs struct {
	// Code 待校验的 mermaid 源码（必填，可带 ```mermaid 围栏）
	Code string `json:"code" jsonschema_description:"待校验的 mermaid 源码，可包含 mermaid 围栏（三个反引号包裹）；一次只传一个图表。"`
}

// buildMermaidCheckTool 按注入的 JS 运行时构造 mermaid_check：工具本身无状态，校验能力由
// internal/pkg/mermaid 提供（官方 bundle 已在该运行时启动时执行）。
func buildMermaidCheckTool(engine *jsengine.Engine) tool.InvokableTool {
	return mustInferTool(
		"mermaid_check",
		"用 mermaid 官方解析器校验图表源码是否合法。生成或修改任何 mermaid 图表后、把回答交给用户前，"+
			"必须先调用本工具；若返回 valid=false，按 issues 中的 line / message / hint 修改源码并再次校验，"+
			"直到 valid=true 再输出。支持官方解析器覆盖的全部图表类型（flowchart/graph、sequenceDiagram、"+
			"classDiagram、stateDiagram、erDiagram、gantt、pie、mindmap、gitGraph 等）；入参可带 ```mermaid 围栏。"+
			"返回 diagramType、valid 与 issues（行号 / 官方报错 / 中文修改建议）。"+
			"流程图（flowchart/graph）另需用 classDef 标注 1-3 个重点环节，前端会把这些类统一渲染为平台主题色："+
			"关键步骤用 zkKey，风险或待确认用 zkWarning，失败或异常用 zkError；写法形如 "+
			"`classDef zkKey stroke-width:2px;` 与 `class A zkKey;`。classDef 内不要写具体颜色值，普通步骤不要加类。",
		func(_ context.Context, in mermaidCheckArgs) (mermaid.CheckResult, error) {
			return mermaid.Check(engine, in.Code), nil
		},
	)
}

func init() {
	Register("mermaid_check", func(_ context.Context, deps Deps) (tool.BaseTool, error) {
		if deps.JS == nil {
			return nil, errors.New("mermaid_check: js engine is not configured")
		}
		return buildMermaidCheckTool(deps.JS), nil
	})
}
