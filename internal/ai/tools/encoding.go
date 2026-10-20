// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态编码类内置工具的薄适配器：hash（文本摘要）、encode_convert（编码转换）与
// uuid（UUID 生成）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自模型入参。纯算法实现已
// 下沉到 internal/pkg/utils（Hash / EncodeConvert / GenerateUUID），本文件只保留入参结构体、Info
// （含中文 Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、调用 utils 导出函数、出错
// 时补上工具名前缀并以 JSON 序列化结果。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// ============================================================================
// hash：文本摘要
// ============================================================================

// hashArgs 是 hash 的 JSON 入参形态：algorithm 与 text 均必填。
type hashArgs struct {
	// Algorithm 哈希算法（必填，取值见 utils.HashAlgorithmNames）
	Algorithm string `json:"algorithm"`
	// Text 待求摘要的 UTF-8 文本（必填）
	Text string `json:"text"`
}

// hashTool 是内置工具 hash 的无状态实现：不持有依赖，全部输入取自调用入参。
type hashTool struct{}

// Info 返回 hash 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（algorithm 必填且带枚举取值，text 必填）。
func (t *hashTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "hash",
		Desc: "对一段 UTF-8 文本计算密码学摘要，返回十六进制与小写 base64 两种表示。" +
			"用于校验文本一致性、比对内容指纹或生成短标识。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"algorithm": {
				Type:     schema.String,
				Desc:     "哈希算法，必填，可选 md5、sha1、sha256 或 sha512。",
				Required: true,
				Enum:     utils.HashAlgorithmNames,
			},
			"text": {
				Type:     schema.String,
				Desc:     "待计算摘要的文本，必须非空。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 hash：解析 JSON 入参，调用 utils.Hash 计算摘要，返回形如
// {"algorithm":"sha256","hex":"...","base64":"..."} 的 JSON 字符串。算法越界或文本为空时返回英文
// 错误（工具名前缀在此补齐）。
func (t *hashTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args hashArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.Hash(args.Text, args.Algorithm)
	if err != nil {
		return "", errors.Newf("hash: %s", err.Error())
	}
	return encodingEncodeResult(result)
}

// ============================================================================
// encode_convert：编码转换
// ============================================================================

// encodeConvertArgs 是 encode_convert 的 JSON 入参形态：operation、scheme 与 text 均必填。
type encodeConvertArgs struct {
	// Operation 操作方向（必填，encode 或 decode）
	Operation string `json:"operation"`
	// Scheme 编码方案（必填，取值见 utils.EncodeSchemeNames）
	Scheme string `json:"scheme"`
	// Text 待转换的文本（必填）
	Text string `json:"text"`
}

// encodeConvertTool 是内置工具 encode_convert 的无状态实现：不持有依赖，全部输入取自调用入参。
type encodeConvertTool struct{}

// Info 返回 encode_convert 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（operation 与 scheme 必填且带枚举取值，text 必填）。
func (t *encodeConvertTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "encode_convert",
		Desc: "在多种编码方案之间转换文本：可对原文编码（encode），或把已编码的文本还原（decode）。" +
			"用于生成 URL 安全的参数、处理 base64 / hex 载荷或做 HTML 转义。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"operation": {
				Type:     schema.String,
				Desc:     "转换方向，必填，encode 表示编码，decode 表示解码。",
				Required: true,
				Enum:     []string{"encode", "decode"},
			},
			"scheme": {
				Type:     schema.String,
				Desc:     "编码方案，必填，可选 base64、base64url、hex、url 或 html。",
				Required: true,
				Enum:     utils.EncodeSchemeNames,
			},
			"text": {
				Type:     schema.String,
				Desc:     "待转换的文本，必须非空。",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun 执行 encode_convert：解析 JSON 入参，调用 utils.EncodeConvert 按方案转换，返回形如
// {"operation":"encode","scheme":"base64","result":"..."} 的 JSON 字符串。操作方向非法、方案越界或
// decode 输入非法（如非法 base64 / hex）时返回英文错误（工具名前缀在此补齐）。
func (t *encodeConvertTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args encodeConvertArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	result, err := utils.EncodeConvert(args.Scheme, args.Operation, args.Text)
	if err != nil {
		return "", errors.Newf("encode_convert: %s", err.Error())
	}
	return encodingEncodeResult(result)
}

// ============================================================================
// uuid：UUID 生成
// ============================================================================

// uuidArgs 是 uuid 的 JSON 入参形态：version 与 count 均可选（缺省分别为 "4" 与 1）。
type uuidArgs struct {
	// Version UUID 版本（可选，缺省 "4"，取值 "4" 或 "7"）
	Version string `json:"version"`
	// Count 生成数量（可选，缺省 1，范围 1..100）；用指针区分「未传」（nil 取缺省）与「显式传 0」
	// （越界，应报错）
	Count *int `json:"count"`
}

// uuidTool 是内置工具 uuid 的无状态实现：不持有依赖，全部输入取自调用入参。
type uuidTool struct{}

// Info 返回 uuid 的工具元信息与参数 JSON schema，供模型决定何时调用。参数经 eino 的
// ParameterInfo 形式描述（version 可选且带枚举取值，count 可选整数）。
func (t *uuidTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "uuid",
		Desc: "生成一个或多个随机 UUID，用于为示例数据、实体或会话构造唯一标识。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"version": {
				Type: schema.String,
				Desc: "UUID 版本，可选，缺省为 4；4 为随机 UUID，7 为按时间有序 UUID。",
				Enum: []string{"4", "7"},
			},
			"count": {
				Type: schema.Integer,
				Desc: "生成数量，可选，缺省为 1，取值范围 1 到 100。",
			},
		}),
	}, nil
}

// InvokableRun 执行 uuid：解析 JSON 入参，用 utils.DefaultUUIDCount 解析可选缺省后调用
// utils.GenerateUUID 按批生成，返回形如 {"version":"4","uuids":["...", ...]} 的 JSON 字符串。版本
// 越界、数量越界或生成失败时返回英文错误（工具名前缀在此补齐）。
func (t *uuidTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args uuidArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	count := utils.DefaultUUIDCount
	if args.Count != nil {
		count = *args.Count
	}
	result, err := utils.GenerateUUID(args.Version, count)
	if err != nil {
		return "", errors.Newf("uuid: %s", err.Error())
	}
	return encodingEncodeResult(result)
}

// encodingEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func encodingEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 hash、encode_convert 与 uuid；三者均无状态，不消费 Deps。
func init() {
	Register("hash", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &hashTool{}, nil
	})
	Register("encode_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &encodeConvertTool{}, nil
	})
	Register("uuid", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &uuidTool{}, nil
	})
}
