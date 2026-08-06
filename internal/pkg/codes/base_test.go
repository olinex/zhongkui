package codes

import (
	"io/fs"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBaseConstants(t *testing.T) {
	Convey("验证 ISO 标准常量值", t, func() {
		Convey("1. 验证 LANGUAGE 常量", func(c C) {
			So(Language, ShouldEqual, "standards/iso_639-2.json")
		})
		Convey("2. 验证 CountryArea 常量", func(c C) {
			So(CountryArea, ShouldEqual, "standards/iso_3166-1.json")
		})
		Convey("3. 验证 CURRENCY 常量", func(c C) {
			So(Currency, ShouldEqual, "standards/iso_4217.json")
		})
		Convey("4. 验证 ScriptLanguage 常量", func(c C) {
			So(ScriptLanguage, ShouldEqual, "standards/iso_15924.json")
		})
	})

	Convey("验证集合类常量值", t, func() {
		Convey("1. 验证 FilenameExtension 常量", func(c C) {
			So(FilenameExtension, ShouldEqual, "collections/filename_extensions.json")
		})
	})
}

func TestBaseConstantsPathFormat(t *testing.T) {
	Convey("验证常量路径前缀格式", t, func() {
		Convey("1. 标准类常量路径应以 standards/ 开头", func(c C) {
			So(strings.HasPrefix(Language, "standards/"), ShouldBeTrue)
			So(strings.HasPrefix(CountryArea, "standards/"), ShouldBeTrue)
			So(strings.HasPrefix(Currency, "standards/"), ShouldBeTrue)
			So(strings.HasPrefix(ScriptLanguage, "standards/"), ShouldBeTrue)
		})
		Convey("2. 集合类常量路径应以 collections/ 开头", func(c C) {
			So(strings.HasPrefix(FilenameExtension, "collections/"), ShouldBeTrue)
		})
	})

	Convey("验证常量路径后缀格式", t, func() {
		Convey("1. 所有常量路径应以 .json 结尾", func(c C) {
			So(strings.HasSuffix(Language, ".json"), ShouldBeTrue)
			So(strings.HasSuffix(CountryArea, ".json"), ShouldBeTrue)
			So(strings.HasSuffix(Currency, ".json"), ShouldBeTrue)
			So(strings.HasSuffix(ScriptLanguage, ".json"), ShouldBeTrue)
			So(strings.HasSuffix(FilenameExtension, ".json"), ShouldBeTrue)
		})
	})

	Convey("验证常量路径中包含对应的 ISO 编号", t, func() {
		Convey("1. LANGUAGE 路径应包含 639-2", func(c C) {
			So(strings.Contains(Language, "639-2"), ShouldBeTrue)
		})
		Convey("2. CountryArea 路径应包含 3166-1", func(c C) {
			So(strings.Contains(CountryArea, "3166-1"), ShouldBeTrue)
		})
		Convey("3. CURRENCY 路径应包含 4217", func(c C) {
			So(strings.Contains(Currency, "4217"), ShouldBeTrue)
		})
		Convey("4. ScriptLanguage 路径应包含 15924", func(c C) {
			So(strings.Contains(ScriptLanguage, "15924"), ShouldBeTrue)
		})
	})

	Convey("验证各常量值互不相同", t, func() {
		all := []string{Language, CountryArea, Currency, ScriptLanguage, FilenameExtension}
		seen := make(map[string]bool, len(all))
		for _, v := range all {
			So(seen[v], ShouldBeFalse)
			seen[v] = true
		}
	})
}

func TestEmbeddedStandards(t *testing.T) {
	Convey("验证嵌入的标准文件可访问", t, func() {
		Convey("1. LANGUAGE 文件可打开", func(c C) {
			f, err := embeddedStandards.Open(Language)
			So(err, ShouldBeNil)
			So(f, ShouldNotBeNil)
			f.Close()
		})
		Convey("2. CountryArea 文件可打开", func(c C) {
			f, err := embeddedStandards.Open(CountryArea)
			So(err, ShouldBeNil)
			So(f, ShouldNotBeNil)
			f.Close()
		})
		Convey("3. CURRENCY 文件可打开", func(c C) {
			f, err := embeddedStandards.Open(Currency)
			So(err, ShouldBeNil)
			So(f, ShouldNotBeNil)
			f.Close()
		})
		Convey("4. ScriptLanguage 文件可打开", func(c C) {
			f, err := embeddedStandards.Open(ScriptLanguage)
			So(err, ShouldBeNil)
			So(f, ShouldNotBeNil)
			f.Close()
		})
	})

	Convey("验证嵌入的标准文件非空且为合法 JSON", t, func() {
		Convey("1. LANGUAGE 文件内容非空且以 { 开头", func(c C) {
			f, err := embeddedStandards.Open(Language)
			So(err, ShouldBeNil)
			defer f.Close()
			info, err := f.Stat()
			So(err, ShouldBeNil)
			So(info.Size(), ShouldBeGreaterThan, 0)
			buf := make([]byte, 1)
			_, err = f.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "{")
		})
		Convey("2. CountryArea 文件内容非空且以 { 开头", func(c C) {
			f, err := embeddedStandards.Open(CountryArea)
			So(err, ShouldBeNil)
			defer f.Close()
			info, err := f.Stat()
			So(err, ShouldBeNil)
			So(info.Size(), ShouldBeGreaterThan, 0)
			buf := make([]byte, 1)
			_, err = f.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "{")
		})
		Convey("3. CURRENCY 文件内容非空且以 { 开头", func(c C) {
			f, err := embeddedStandards.Open(Currency)
			So(err, ShouldBeNil)
			defer f.Close()
			info, err := f.Stat()
			So(err, ShouldBeNil)
			So(info.Size(), ShouldBeGreaterThan, 0)
			buf := make([]byte, 1)
			_, err = f.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "{")
		})
		Convey("4. ScriptLanguage 文件内容非空且以 { 开头", func(c C) {
			f, err := embeddedStandards.Open(ScriptLanguage)
			So(err, ShouldBeNil)
			defer f.Close()
			info, err := f.Stat()
			So(err, ShouldBeNil)
			So(info.Size(), ShouldBeGreaterThan, 0)
			buf := make([]byte, 1)
			_, err = f.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "{")
		})
	})

	Convey("验证嵌入的标准文件不包含集合类路径", t, func() {
		Convey("1. embeddedStandards 不应包含 collections 路径", func(c C) {
			_, err := embeddedStandards.Open("collections/filename_extensions.json")
			So(err, ShouldNotBeNil)
		})
	})

	Convey("验证打开不存在的标准文件应返回错误", t, func() {
		Convey("1. 不存在的文件路径", func(c C) {
			_, err := embeddedStandards.Open("standards/nonexistent.json")
			So(err, ShouldNotBeNil)
		})
		Convey("2. 空路径", func(c C) {
			_, err := embeddedStandards.Open("")
			So(err, ShouldNotBeNil)
		})
		Convey("3. 目录路径", func(c C) {
			_, err := embeddedStandards.Open("standards/")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEmbeddedCollections(t *testing.T) {
	Convey("验证嵌入的集合文件可访问", t, func() {
		Convey("1. FilenameExtension 文件可打开", func(c C) {
			f, err := embeddedCollections.Open(FilenameExtension)
			So(err, ShouldBeNil)
			So(f, ShouldNotBeNil)
			f.Close()
		})
	})

	Convey("验证嵌入的集合文件非空且为合法 JSON", t, func() {
		Convey("1. FilenameExtension 文件内容非空且以 { 开头", func(c C) {
			f, err := embeddedCollections.Open(FilenameExtension)
			So(err, ShouldBeNil)
			defer f.Close()
			info, err := f.Stat()
			So(err, ShouldBeNil)
			So(info.Size(), ShouldBeGreaterThan, 0)
			buf := make([]byte, 1)
			_, err = f.Read(buf)
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, "{")
		})
	})

	Convey("验证嵌入的集合文件不包含标准类路径", t, func() {
		Convey("1. embeddedCollections 不应包含 standards 路径", func(c C) {
			_, err := embeddedCollections.Open("standards/iso_639-2.json")
			So(err, ShouldNotBeNil)
		})
	})

	Convey("验证打开不存在的集合文件应返回错误", t, func() {
		Convey("1. 不存在的文件路径", func(c C) {
			_, err := embeddedCollections.Open("collections/nonexistent.json")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEmbeddedFSIsolation(t *testing.T) {
	Convey("验证两个嵌入文件系统相互隔离", t, func() {
		Convey("1. embeddedStandards 不能访问 collections 目录中的文件", func(c C) {
			_, err := embeddedStandards.Open(FilenameExtension)
			So(err, ShouldNotBeNil)
		})
		Convey("2. embeddedCollections 不能访问 standards 目录中的文件", func(c C) {
			_, err := embeddedCollections.Open(Language)
			So(err, ShouldNotBeNil)
			_, err = embeddedCollections.Open(CountryArea)
			So(err, ShouldNotBeNil)
			_, err = embeddedCollections.Open(Currency)
			So(err, ShouldNotBeNil)
			_, err = embeddedCollections.Open(ScriptLanguage)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEmbeddedFSReadDir(t *testing.T) {
	Convey("验证嵌入文件系统的目录结构", t, func() {
		Convey("1. embeddedStandards 的 standards 目录应包含 4 个文件", func(c C) {
			entries, err := embeddedStandards.ReadDir("standards")
			So(err, ShouldBeNil)
			So(len(entries), ShouldEqual, 4)
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				So(e.IsDir(), ShouldBeFalse)
				names = append(names, e.Name())
			}
			So(names, ShouldContain, "iso_639-2.json")
			So(names, ShouldContain, "iso_3166-1.json")
			So(names, ShouldContain, "iso_4217.json")
			So(names, ShouldContain, "iso_15924.json")
		})
		Convey("2. embeddedCollections 的 collections 目录应包含 1 个文件", func(c C) {
			entries, err := embeddedCollections.ReadDir("collections")
			So(err, ShouldBeNil)
			So(len(entries), ShouldEqual, 1)
			So(entries[0].Name(), ShouldEqual, "filename_extensions.json")
			So(entries[0].IsDir(), ShouldBeFalse)
		})
	})

	Convey("验证读取不存在的目录应返回错误", t, func() {
		Convey("1. embeddedStandards 读取不存在的目录", func(c C) {
			_, err := embeddedStandards.ReadDir("nonexistent")
			So(err, ShouldNotBeNil)
		})
		Convey("2. embeddedCollections 读取不存在的目录", func(c C) {
			_, err := embeddedCollections.ReadDir("nonexistent")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEmbeddedFSStat(t *testing.T) {
	Convey("验证通过 Stat 获取文件信息", t, func() {
		Convey("1. 获取 LANGUAGE 文件信息", func(c C) {
			info, err := fs.Stat(embeddedStandards, Language)
			So(err, ShouldBeNil)
			So(info.Name(), ShouldEqual, "iso_639-2.json")
			So(info.Size(), ShouldBeGreaterThan, 0)
			So(info.IsDir(), ShouldBeFalse)
		})
		Convey("2. 获取 FilenameExtension 文件信息", func(c C) {
			info, err := fs.Stat(embeddedCollections, FilenameExtension)
			So(err, ShouldBeNil)
			So(info.Name(), ShouldEqual, "filename_extensions.json")
			So(info.Size(), ShouldBeGreaterThan, 0)
			So(info.IsDir(), ShouldBeFalse)
		})
	})

	Convey("验证 Stat 不存在的文件应返回错误", t, func() {
		Convey("1. embeddedStandards 中不存在的文件", func(c C) {
			_, err := fs.Stat(embeddedStandards, "standards/nonexistent.json")
			So(err, ShouldNotBeNil)
		})
		Convey("2. embeddedCollections 中不存在的文件", func(c C) {
			_, err := fs.Stat(embeddedCollections, "collections/nonexistent.json")
			So(err, ShouldNotBeNil)
		})
	})
}
