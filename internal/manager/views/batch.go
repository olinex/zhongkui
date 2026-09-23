// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件实现动作接口批量化所需的通用抽象：
//   - FlexList：兼容标量与数组两种 JSON 形式的标识符列表反序列化
//   - BatchResultOut：批量操作的汇总响应（成功数 + 失败明细）
//   - batchExecute：尽力而为的批量执行骨架（失败汇总不中断）
//   - validFlexLen：批量标识符列表长度校验（1..100）
package views

import (
	"encoding/json"

	"zhongkui.com/zhongkui/internal/manager/response"

	"github.com/gin-gonic/gin"
	uv "github.com/go-playground/validator/v10"
)

// maxBatchItems 批量操作单次允许的最大条数。
const maxBatchItems = 100

// FlexList 兼容标量与数组两种 JSON 形式的标识符列表。
// 标量形式（如 5 / "abc"）等价于单元素数组（[5] / ["abc"]），
// 用于动作接口（delete/enable/disable/activate/deactivate）的标识符入参：
// 单条可传标量或单元素数组，多条传数组。
// 元素为 serializer.HexUUID（[16]byte 底层）时标量形式为 JSON 字符串，
// 数组形式为字符串数组，由元素自身的 UnmarshalJSON 处理。
type FlexList[T any] []T

// UnmarshalJSON 实现 json.Unmarshaler 接口。
// 输入为数组时直接按 []T 解析；输入为标量时包装为单元素数组后解析。
func (l *FlexList[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*l = nil
		return nil
	}
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, (*[]T)(l))
	}
	// 标量形式：包装为单元素数组，复用元素自身的反序列化逻辑
	wrapped := make([]byte, 0, len(data)+2)
	wrapped = append(wrapped, '[')
	wrapped = append(wrapped, data...)
	wrapped = append(wrapped, ']')
	return json.Unmarshal(wrapped, (*[]T)(l))
}

// BatchFailureOut 批量操作中单条失败项的汇总信息。
type BatchFailureOut struct {
	// ID 失败项标识符（idOf 返回值）
	ID string `json:"id"`
	// Reason 失败原因（逐条执行返回的错误文本）
	Reason string `json:"reason"`
}

// BatchResultOut 批量操作的结果汇总（尽力而为 + 失败汇总）。
type BatchResultOut struct {
	// Succeeded 成功条数
	Succeeded int `json:"succeeded"`
	// Failed 失败明细列表，为空数组时表示全部成功
	Failed []BatchFailureOut `json:"failed"`
}

// batchExecute 批量执行骨架：逐条调用 act，错误捕获进 Failed（不中断后续条目），
// 最终统一返回 BatchResultOut 汇总（HTTP 200 表达批量结果，失败在明细中体现）。
func BatchExecute[T any](c *gin.Context, items []T, idOf func(T) string, act func(T) error) {
	out := BatchResultOut{Failed: []BatchFailureOut{}}
	for _, item := range items {
		if err := act(item); err != nil {
			out.Failed = append(out.Failed, BatchFailureOut{ID: idOf(item), Reason: err.Error()})
		} else {
			out.Succeeded++
		}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// validFlexLen 校验批量标识符列表长度（1..maxBatchItems）。
// 长度非法时以 400 中断请求并返回 false。
func ValidFlexLen[T any](c *gin.Context, items []T) bool {
	if len(items) >= 1 && len(items) <= maxBatchItems {
		return true
	}
	response.CreateBadBodyError(uv.ValidationErrors{}).AbortWithGinH(c)
	return false
}
