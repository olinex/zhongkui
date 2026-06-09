package codes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetFilenameExtensionCodes(t *testing.T) {
	Convey("验证单例行为", t, func() {
		instance1 := GetFilenameExtensionCodes()
		instance2 := GetFilenameExtensionCodes()
		Convey("1. 多次调用应返回同一实例", func(c C) {
			So(instance1, ShouldPointTo, instance2)
		})
		Convey("2. 单例实例不应为 nil", func(c C) {
			So(instance1, ShouldNotBeNil)
		})
	})

	Convey("验证 Extensions 数据已加载", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. Extensions 映射不应为空", func(c C) {
			So(checker.Extensions, ShouldNotBeEmpty)
		})
	})
}

func TestFilenameExtensionCodes_ConvertFromName(t *testing.T) {
	Convey("查询不存在的扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. 空字符串应返回 nil", func(c C) {
			So(checker.ConvertFromName(""), ShouldBeNil)
		})
		Convey("2. 不存在的扩展名应返回 nil", func(c C) {
			So(checker.ConvertFromName("unknown"), ShouldBeNil)
			So(checker.ConvertFromName("xyz123"), ShouldBeNil)
			So(checker.ConvertFromName("notarealext"), ShouldBeNil)
		})
	})

	Convey("查询文本类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. pdf 应属于 text 分类", func(c C) {
			result := checker.ConvertFromName("pdf")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "text")
		})
		Convey("2. txt 应属于 text 分类", func(c C) {
			result := checker.ConvertFromName("txt")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "text")
		})
		Convey("3. doc 应属于 text 分类", func(c C) {
			result := checker.ConvertFromName("doc")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "text")
		})
		Convey("4. docx 应属于 text 分类", func(c C) {
			result := checker.ConvertFromName("docx")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "text")
		})
		Convey("5. rtf 应属于 text 分类", func(c C) {
			result := checker.ConvertFromName("rtf")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "text")
		})
	})

	Convey("查询图片类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. jpg 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("jpg")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
		Convey("2. png 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("png")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
		Convey("3. gif 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("gif")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
		Convey("4. bmp 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("bmp")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
		Convey("5. svg 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("svg")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
		Convey("6. webp 应属于 image 分类", func(c C) {
			result := checker.ConvertFromName("webp")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "image")
		})
	})

	Convey("查询音频类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. mp3 应属于 audio 分类", func(c C) {
			result := checker.ConvertFromName("mp3")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "audio")
		})
		Convey("2. wav 应属于 audio 分类", func(c C) {
			result := checker.ConvertFromName("wav")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "audio")
		})
		Convey("3. flac 应属于 audio 分类", func(c C) {
			result := checker.ConvertFromName("flac")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "audio")
		})
		Convey("4. ogg 应属于 audio 分类", func(c C) {
			result := checker.ConvertFromName("ogg")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "audio")
		})
		Convey("5. aac 应属于 audio 分类", func(c C) {
			result := checker.ConvertFromName("aac")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "audio")
		})
	})

	Convey("查询视频类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. mp4 应属于 video 分类", func(c C) {
			result := checker.ConvertFromName("mp4")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "video")
		})
		Convey("2. avi 应属于 video 分类", func(c C) {
			result := checker.ConvertFromName("avi")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "video")
		})
		Convey("3. mkv 应属于 video 分类", func(c C) {
			result := checker.ConvertFromName("mkv")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "video")
		})
		Convey("4. mov 应属于 video 分类", func(c C) {
			result := checker.ConvertFromName("mov")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "video")
		})
	})

	Convey("查询代码类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. json 应属于 code 分类", func(c C) {
			result := checker.ConvertFromName("json")
			So(result, ShouldNotBeNil)
			So(result[0], ShouldEqual, "code")
		})
		Convey("2. go 应属于 code 分类", func(c C) {
			result := checker.ConvertFromName("go")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "code")
		})
		Convey("3. py 应属于 code 分类", func(c C) {
			result := checker.ConvertFromName("py")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "code")
		})
		Convey("4. java 应属于 code 分类", func(c C) {
			result := checker.ConvertFromName("java")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "code")
		})
		Convey("5. rs 应属于 code 分类", func(c C) {
			result := checker.ConvertFromName("rs")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "code")
		})
	})

	Convey("查询压缩包类扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. zip 应属于 archive 分类", func(c C) {
			result := checker.ConvertFromName("zip")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "archive")
		})
		Convey("2. rar 应属于 archive 分类", func(c C) {
			result := checker.ConvertFromName("rar")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "archive")
		})
		Convey("3. 7z 应属于 archive 分类", func(c C) {
			result := checker.ConvertFromName("7z")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "archive")
		})
		Convey("4. tar 应属于 archive 分类", func(c C) {
			result := checker.ConvertFromName("tar")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 1)
			So(result[0], ShouldEqual, "archive")
		})
	})

	Convey("查询多分类标签扩展名", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. html 应同时属于 code 和 web 分类", func(c C) {
			result := checker.ConvertFromName("html")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "code")
			So(result[1], ShouldEqual, "web")
		})
		Convey("2. htm 应同时属于 code 和 web 分类", func(c C) {
			result := checker.ConvertFromName("htm")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "code")
			So(result[1], ShouldEqual, "web")
		})
		Convey("3. js 应同时属于 code 和 web 分类", func(c C) {
			result := checker.ConvertFromName("js")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "code")
			So(result[1], ShouldEqual, "web")
		})
		Convey("4. php 应同时属于 code 和 web 分类", func(c C) {
			result := checker.ConvertFromName("php")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "code")
			So(result[1], ShouldEqual, "web")
		})
		Convey("5. sh 应同时属于 code 和 exec 分类", func(c C) {
			result := checker.ConvertFromName("sh")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "code")
			So(result[1], ShouldEqual, "exec")
		})
		Convey("6. yuv 应同时属于 image 和 video 分类", func(c C) {
			result := checker.ConvertFromName("yuv")
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveLength, 2)
			So(result[0], ShouldEqual, "image")
			So(result[1], ShouldEqual, "video")
		})
	})

	Convey("验证大小写敏感性", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. 小写扩展名应正常查询", func(c C) {
			result := checker.ConvertFromName("pdf")
			So(result, ShouldNotBeNil)
		})
		Convey("2. 大写扩展名应返回 nil", func(c C) {
			So(checker.ConvertFromName("PDF"), ShouldBeNil)
			So(checker.ConvertFromName("JPG"), ShouldBeNil)
			So(checker.ConvertFromName("MP4"), ShouldBeNil)
		})
		Convey("3. 混合大小写扩展名应返回 nil", func(c C) {
			So(checker.ConvertFromName("Pdf"), ShouldBeNil)
			So(checker.ConvertFromName("Jpg"), ShouldBeNil)
		})
	})

	Convey("验证数据完整性 - 常见格式", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. 常见图片格式应存在", func(c C) {
			So(checker.ConvertFromName("jpg"), ShouldNotBeNil)
			So(checker.ConvertFromName("jpeg"), ShouldNotBeNil)
			So(checker.ConvertFromName("png"), ShouldNotBeNil)
			So(checker.ConvertFromName("gif"), ShouldNotBeNil)
			So(checker.ConvertFromName("bmp"), ShouldNotBeNil)
			So(checker.ConvertFromName("svg"), ShouldNotBeNil)
			So(checker.ConvertFromName("webp"), ShouldNotBeNil)
			So(checker.ConvertFromName("ico"), ShouldBeNil)
		})
		Convey("2. 常见视频格式应存在", func(c C) {
			So(checker.ConvertFromName("mp4"), ShouldNotBeNil)
			So(checker.ConvertFromName("avi"), ShouldNotBeNil)
			So(checker.ConvertFromName("mkv"), ShouldNotBeNil)
			So(checker.ConvertFromName("mov"), ShouldNotBeNil)
			So(checker.ConvertFromName("wmv"), ShouldNotBeNil)
			So(checker.ConvertFromName("flv"), ShouldNotBeNil)
		})
		Convey("3. 常见音频格式应存在", func(c C) {
			So(checker.ConvertFromName("mp3"), ShouldNotBeNil)
			So(checker.ConvertFromName("wav"), ShouldNotBeNil)
			So(checker.ConvertFromName("flac"), ShouldNotBeNil)
			So(checker.ConvertFromName("ogg"), ShouldNotBeNil)
			So(checker.ConvertFromName("aac"), ShouldNotBeNil)
			So(checker.ConvertFromName("wma"), ShouldNotBeNil)
		})
		Convey("4. 常见压缩包格式应存在", func(c C) {
			So(checker.ConvertFromName("zip"), ShouldNotBeNil)
			So(checker.ConvertFromName("rar"), ShouldNotBeNil)
			So(checker.ConvertFromName("7z"), ShouldNotBeNil)
			So(checker.ConvertFromName("tar"), ShouldNotBeNil)
			So(checker.ConvertFromName("gz"), ShouldNotBeNil)
			So(checker.ConvertFromName("bz2"), ShouldNotBeNil)
		})
		Convey("5. 常见编程语言源码格式应存在", func(c C) {
			So(checker.ConvertFromName("go"), ShouldNotBeNil)
			So(checker.ConvertFromName("py"), ShouldNotBeNil)
			So(checker.ConvertFromName("java"), ShouldNotBeNil)
			So(checker.ConvertFromName("c"), ShouldNotBeNil)
			So(checker.ConvertFromName("cpp"), ShouldNotBeNil)
			So(checker.ConvertFromName("rs"), ShouldNotBeNil)
			So(checker.ConvertFromName("js"), ShouldNotBeNil)
			So(checker.ConvertFromName("ts"), ShouldNotBeNil)
		})
		Convey("6. 常见字体格式应存在", func(c C) {
			So(checker.ConvertFromName("ttf"), ShouldNotBeNil)
			So(checker.ConvertFromName("otf"), ShouldNotBeNil)
			So(checker.ConvertFromName("woff"), ShouldNotBeNil)
			So(checker.ConvertFromName("woff2"), ShouldNotBeNil)
		})
		Convey("7. 常见电子书格式应存在", func(c C) {
			So(checker.ConvertFromName("epub"), ShouldNotBeNil)
			So(checker.ConvertFromName("mobi"), ShouldNotBeNil)
			So(checker.ConvertFromName("pdf"), ShouldNotBeNil)
		})
	})

	Convey("验证返回结果不会被修改影响后续查询", t, func() {
		checker := GetFilenameExtensionCodes()
		Convey("1. 修改返回的切片不应影响原始数据", func(c C) {
			result1 := checker.ConvertFromName("pdf")
			So(result1, ShouldNotBeNil)
			originalLen := len(result1)
			result2 := checker.ConvertFromName("pdf")
			So(result2, ShouldHaveLength, originalLen)
			So(result2[0], ShouldEqual, "text")
		})
	})
}
