// Package views（本文件）提供 ReadMultipartFile 的单测：验证 file 部分以未缓冲的读取流返回、
// 普通表单字段经回调填充且有大小上限、超限字段直接报错。
package views

import (
	// 标准库
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/response"
)

// newMultipartContext 构造携带 multipart/form-data 请求体的 gin 测试上下文，body 由 build 写入。
func newMultipartContext(t *testing.T, build func(w *multipart.Writer)) *gin.Context {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	build(w)
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", &buf)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	return c
}

func TestReadMultipartFile(t *testing.T) {
	Convey("普通字段经回调填充，file 部分以未缓冲的读取流返回", t, func() {
		var title string
		c := newMultipartContext(t, func(w *multipart.Writer) {
			So(w.WriteField("title", "报告"), ShouldBeNil)
			fw, err := w.CreateFormFile("file", "a.db")
			So(err, ShouldBeNil)
			_, err = fw.Write([]byte("BINARY-CONTENT"))
			So(err, ShouldBeNil)
		})

		file, err := ReadMultipartFile(c, func(name string, body []byte) {
			if name == "title" {
				title = string(body)
			}
		})
		So(err, ShouldBeNil)
		So(title, ShouldEqual, "报告")

		// 返回的必须是尚未读尽的 *multipart.Part（流式），而非整文件已缓冲的 bytes.Reader。
		part, ok := file.(*multipart.Part)
		So(ok, ShouldBeTrue)
		So(part.FileName(), ShouldEqual, "a.db")
		defer part.Close()
		got, err := io.ReadAll(file)
		So(err, ShouldBeNil)
		So(string(got), ShouldEqual, "BINARY-CONTENT")
	})

	Convey("单个普通字段超限即报错，且不触发回调", t, func() {
		c := newMultipartContext(t, func(w *multipart.Writer) {
			So(w.WriteField("config", strings.Repeat("x", maxFormFieldBytes+1)), ShouldBeNil)
		})
		called := false
		file, err := ReadMultipartFile(c, func(string, []byte) { called = true })
		So(err, ShouldNotBeNil)
		// 超限详情为可本地化的 DetailContext，须经 ToGinH 按语言渲染后再校验文案。
		codeErr, ok := err.(*response.CodeError)
		So(ok, ShouldBeTrue)
		So(codeErr.ToGinH(newBaseTestLocalizer(), nil)["details"], ShouldContainSubstring, "exceeds the maximum size")
		So(file, ShouldBeNil)
		So(called, ShouldBeFalse)
	})

	Convey("恰好等于上限的字段仍可正常读取", t, func() {
		c := newMultipartContext(t, func(w *multipart.Writer) {
			So(w.WriteField("config", strings.Repeat("y", maxFormFieldBytes)), ShouldBeNil)
		})
		var got string
		_, err := ReadMultipartFile(c, func(name string, body []byte) {
			if name == "config" {
				got = string(body)
			}
		})
		So(err, ShouldBeNil)
		So(len(got), ShouldEqual, maxFormFieldBytes)
	})

	Convey("未携带 file 部分时返回 nil 且不报错", t, func() {
		c := newMultipartContext(t, func(w *multipart.Writer) {
			So(w.WriteField("title", "t"), ShouldBeNil)
		})
		file, err := ReadMultipartFile(c, func(string, []byte) {})
		So(err, ShouldBeNil)
		So(file, ShouldBeNil)
	})
}
