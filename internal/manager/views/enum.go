// Package views 提供 HTTP 接口的请求输入（In）与响应输出（Out）视图结构体及处理器。
// 本文件实现统一枚举获取接口（/admin/enum/get 与 /admin/enum/list）：
// 该接口是前端获取枚举值的唯一通道，值来源为 enum 注册表（internal/pkg/enum），
// 条目携带按请求语言翻译的 label（translate.EnumLabel，键 enum.<类名>.<key>，
// 未命中回退 key）。
package views

import (
	// stdlib
	"net/http"

	// third-party
	"github.com/gin-gonic/gin"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/enum"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// EnumEntryOut 枚举条目出参：key（i18n 定位）、value（数据库存储）、label（按请求语言翻译）。
type EnumEntryOut struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// EnumTypeOut 单个枚举类出参：类名 + 条目列表。
type EnumTypeOut struct {
	Type    string         `json:"type"`
	Entries []EnumEntryOut `json:"entries"`
}

// EnumGetIn 枚举类查询入参：type 为枚举类名（必填），lang 为请求语言（缺省回退请求头语言）。
type EnumGetIn struct {
	Type string        `json:"type" binding:"required"`
	Lang base.Language `json:"lang"`
}

// EnumGet 处理 /admin/enum/get：按类名 + 语言返回条目（key/value/label）。
// 未知枚举类返回 400；label 经 translate.EnumLabel 按 enum.<类名>.<key> 翻译，未命中回退 key。
func EnumGet(c *gin.Context) {
	var in EnumGetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	t, ok := enum.Get(in.Type)
	if !ok {
		response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody,
			errors.Newf("enum: unknown type %q", in.Type)).AbortWithGinH(c)
		return
	}
	lang := enumRequestLang(c, in.Lang)
	entries := make([]EnumEntryOut, 0, len(t.Entries))
	for _, e := range t.Entries {
		entries = append(entries, EnumEntryOut{
			Key:   e.Key,
			Value: e.Value,
			Label: translate.EnumLabel(lang, t.Name, e.Key),
		})
	}
	response.SimpleOK.AbortWithGinW(c, EnumTypeOut{Type: t.Name, Entries: entries})
}

// EnumListIn 枚举全量类查询入参：lang 为请求语言（缺省回退请求头语言）。
type EnumListIn struct {
	Lang base.Language `json:"lang"`
}

// EnumListOut 枚举全量类出参。
type EnumListOut struct {
	Types []EnumTypeOut `json:"types"`
}

// EnumList 处理 /admin/enum/list：按语言返回全部枚举类条目。
func EnumList(c *gin.Context) {
	var in EnumListIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	lang := enumRequestLang(c, in.Lang)
	types := enum.List()
	outs := make([]EnumTypeOut, 0, len(types))
	for _, t := range types {
		entries := make([]EnumEntryOut, 0, len(t.Entries))
		for _, e := range t.Entries {
			entries = append(entries, EnumEntryOut{
				Key:   e.Key,
				Value: e.Value,
				Label: translate.EnumLabel(lang, t.Name, e.Key),
			})
		}
		outs = append(outs, EnumTypeOut{Type: t.Name, Entries: entries})
	}
	response.SimpleOK.AbortWithGinW(c, EnumListOut{Types: outs})
}

// enumRequestLang 解析枚举接口的请求语言：入参 lang 非空时优先使用，
// 否则回退请求头语言（middlewares.GetLanguage，恒非 nil）；两者皆缺省时由
// translate 层 resolveLang 自行回退默认语言。
func enumRequestLang(c *gin.Context, lang base.Language) *base.Language {
	if lang.String() != "" {
		return &lang
	}
	return middlewares.GetLanguage(c)
}
