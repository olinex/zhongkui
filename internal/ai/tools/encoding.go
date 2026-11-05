// Package tools 提供 AI 助手内置工具（builtin tools）的实现注册表：按工具名解析为 eino
// tool.BaseTool 并注入 DB / 日志等运行期依赖；工具元信息与出厂种子在别处声明，本包只承载实现。
//
// 本文件是三个无状态编码类内置工具的薄适配器：hash（文本摘要）、encode_convert（编码转换）与
// uuid（UUID 生成）。三者都不持有任何依赖，构造器忽略 Deps，全部信息均来自模型入参。纯算法实现已
// 下沉到 internal/pkg/utils（Hash / EncodeConvert / GenerateUUID），工具契约由 eino 的
// tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；本文件只保留
// 入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

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
	Algorithm string `json:"algorithm" jsonschema:"enum=md5,enum=sha1,enum=sha256,enum=sha512" jsonschema_description:"哈希算法，必填，可选 md5、sha1、sha256 或 sha512。"`
	// Text 待求摘要的 UTF-8 文本（必填）
	Text string `json:"text" jsonschema_description:"待计算摘要的文本，必须非空。"`
}

// hashTool 是内置工具 hash 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var hashTool = mustInferTool(
	"hash",
	"对一段 UTF-8 文本计算密码学摘要，返回十六进制与小写 base64 两种表示。"+
		"用于校验文本一致性、比对内容指纹或生成短标识。",
	func(_ context.Context, in hashArgs) (utils.HashResult, error) {
		result, err := utils.Hash(in.Text, in.Algorithm)
		if err != nil {
			return utils.HashResult{}, errors.Newf("hash: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// encode_convert：编码转换
// ============================================================================

// encodeConvertArgs 是 encode_convert 的 JSON 入参形态：operation、scheme 与 text 均必填。
type encodeConvertArgs struct {
	// Operation 操作方向（必填，encode 或 decode）
	Operation string `json:"operation" jsonschema:"enum=encode,enum=decode" jsonschema_description:"转换方向，必填，encode 表示编码，decode 表示解码。"`
	// Scheme 编码方案（必填，取值见 utils.EncodeSchemeNames）
	Scheme string `json:"scheme" jsonschema:"enum=base64,enum=base64url,enum=hex,enum=url,enum=html" jsonschema_description:"编码方案，必填，可选 base64、base64url、hex、url 或 html。"`
	// Text 待转换的文本（必填）
	Text string `json:"text" jsonschema_description:"待转换的文本，必须非空。"`
}

// encodeConvertTool 是内置工具 encode_convert 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var encodeConvertTool = mustInferTool(
	"encode_convert",
	"在多种编码方案之间转换文本：可对原文编码（encode），或把已编码的文本还原（decode）。"+
		"用于生成 URL 安全的参数、处理 base64 / hex 载荷或做 HTML 转义。",
	func(_ context.Context, in encodeConvertArgs) (utils.EncodeConvertResult, error) {
		result, err := utils.EncodeConvert(in.Scheme, in.Operation, in.Text)
		if err != nil {
			return utils.EncodeConvertResult{}, errors.Newf("encode_convert: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// uuid：UUID 生成
// ============================================================================

// uuidArgs 是 uuid 的 JSON 入参形态：version 与 count 均可选（缺省分别为 "4" 与 1）。
type uuidArgs struct {
	// Version UUID 版本（可选，缺省 "4"，取值 "4" 或 "7"）
	Version string `json:"version,omitempty" jsonschema:"enum=4,enum=7" jsonschema_description:"UUID 版本，可选，缺省为 4；4 为随机 UUID，7 为按时间有序 UUID。"`
	// Count 生成数量（可选，缺省 1，范围 1..100）；用指针区分「未传」（nil 取缺省）与「显式传 0」
	// （越界，应报错）
	Count *int `json:"count,omitempty" jsonschema_description:"生成数量，可选，缺省为 1，取值范围 1 到 100。"`
}

// uuidTool 是内置工具 uuid 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var uuidTool = mustInferTool(
	"uuid",
	"生成一个或多个随机 UUID，用于为示例数据、实体或会话构造唯一标识。",
	func(_ context.Context, in uuidArgs) (utils.UUIDResult, error) {
		count := utils.DefaultUUIDCount
		if in.Count != nil {
			count = *in.Count
		}
		result, err := utils.GenerateUUID(in.Version, count)
		if err != nil {
			return utils.UUIDResult{}, errors.Newf("uuid: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 hash、encode_convert 与 uuid；三者均无状态，不消费 Deps。
func init() {
	Register("hash", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return hashTool, nil
	})
	Register("encode_convert", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return encodeConvertTool, nil
	})
	Register("uuid", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return uuidTool, nil
	})
}
