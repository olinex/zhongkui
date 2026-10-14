// 本文件提供视图层共享的 multipart/form-data 流式解析助手 ReadMultipartFile。
// 供需要直通上传文件流（如第三方数据源 sqlite .db、知识库文档原件）的接口复用，
// 避免每个接口各自重复实现「先读普通表单字段、再流式返回 file 部分」的解析逻辑。
package views

import (
	// 标准库
	"io"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// maxFormFieldBytes 是单个普通表单字段（非 file 部分）读入内存的上限：这类字段只承载标题、
// 名称、类型、配置等小数据，超过即视为异常/滥用请求并报错，避免超长字段把内存打满。
const maxFormFieldBytes = 1 << 20

// ReadMultipartFile 流式解析 multipart/form-data 请求：把 file 字段之前出现的普通表单字段经
// cb(name, body) 回调交给调用方填充，并把 file 部分原样作为读取流返回给调用方直通存储层——
// file 内容不预读、不缓冲，内存占用与文件大小无关。约定 file 字段须位于最后一个 part（前端
// 保证顺序），因此返回的流在回调完成后即可安全读取。未携带 file 时返回 nil。
// 普通表单字段按 maxFormFieldBytes 有界读取，超出即返回错误；返回的读取流须由调用方关闭。
// 调用方需要原始文件名时可把返回的 io.Reader 断言为 *multipart.Part 后调用其 FileName()。
func ReadMultipartFile(c *gin.Context, cb func(name string, body []byte)) (io.Reader, error) {
	mr, err := c.Request.MultipartReader()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			return nil, errors.WithStack(err)
		}
		name := part.FormName()
		if name == "file" {
			// 将整个 file 部分作为读取流返回（不预读），由存储层流式消费。
			return part, nil
		}
		// 普通字段有界读取：最多 maxFormFieldBytes+1 字节，超出即判定超限，而非继续膨胀内存。
		body, err := io.ReadAll(io.LimitReader(part, maxFormFieldBytes+1))
		if err != nil {
			_ = part.Close()
			return nil, errors.WithStack(err)
		}
		if len(body) > maxFormFieldBytes {
			// 超限字段：立即报错，不再排空剩余数据（避免为滥用请求空耗带宽/时间），交由服务端中止。
			return nil, response.CreateBadBodyMessageError("multipartFieldTooLarge", name, maxFormFieldBytes)
		}
		if err := part.Close(); err != nil {
			return nil, errors.WithStack(err)
		}
		cb(name, body)
	}
}
