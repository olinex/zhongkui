package enum

import (
	// stdlib
	"testing"

	// third-party
	. "github.com/smartystreets/goconvey/convey"
)

// assertPanicWith 断言 fn 触发 panic 且消息包含 want 子串。
func assertPanicWith(fn func(), want string) {
	defer func() {
		r := recover()
		So(r, ShouldNotBeNil)
		So(r, ShouldContainSubstring, want)
	}()
	fn()
}

func TestRegisterAndGet(t *testing.T) {
	Convey("枚举类注册与查询", t, func() {
		Reset(func() {
			registry = map[string]*Type{}
		})

		Convey("注册正常类后可按名查询", func() {
			Register(Type{
				Name: "httpMethod",
				Entries: []Entry{
					{Key: "GET", Value: "GET"},
					{Key: "POST", Value: "POST"},
				},
			})
			typ, ok := Get("httpMethod")
			So(ok, ShouldBeTrue)
			So(typ.Name, ShouldEqual, "httpMethod")
			So(len(typ.Entries), ShouldEqual, 2)
			So(Exists("httpMethod"), ShouldBeTrue)
			So(Exists("missing"), ShouldBeFalse)
		})

		Convey("未注册类查询返回 false", func() {
			_, ok := Get("notRegistered")
			So(ok, ShouldBeFalse)
		})

		Convey("List 按类名排序返回全部", func() {
			Register(Type{Name: "b", Entries: []Entry{{Key: "x", Value: "x"}}})
			Register(Type{Name: "a", Entries: []Entry{{Key: "x", Value: "x"}}})
			types := List()
			So(len(types), ShouldEqual, 2)
			So(types[0].Name, ShouldEqual, "a")
			So(types[1].Name, ShouldEqual, "b")
		})

		Convey("空类注册后查询返回空条目", func() {
			Register(Type{Name: "empty", Entries: nil})
			typ, ok := Get("empty")
			So(ok, ShouldBeTrue)
			So(typ.Entries, ShouldBeEmpty)
		})
	})
}

func TestRegisterValidation(t *testing.T) {
	Convey("枚举类注册校验", t, func() {
		Reset(func() {
			registry = map[string]*Type{}
		})

		Convey("类名重复 panic", func() {
			Register(Type{Name: "t", Entries: []Entry{{Key: "a", Value: "a"}}})
			assertPanicWith(func() {
				Register(Type{Name: "t", Entries: []Entry{{Key: "b", Value: "b"}}})
			}, "duplicate type name")
		})

		Convey("空类名 panic", func() {
			assertPanicWith(func() {
				Register(Type{Name: "", Entries: []Entry{{Key: "a", Value: "a"}}})
			}, "empty type name")
		})

		Convey("类内重复 key panic", func() {
			assertPanicWith(func() {
				Register(Type{
					Name: "t",
					Entries: []Entry{
						{Key: "a", Value: "1"},
						{Key: "a", Value: "2"},
					},
				})
			}, "duplicate key")
		})

		Convey("类内重复 value panic", func() {
			assertPanicWith(func() {
				Register(Type{
					Name: "t",
					Entries: []Entry{
						{Key: "a", Value: "x"},
						{Key: "b", Value: "x"},
					},
				})
			}, "duplicate value")
		})

		Convey("空 key panic", func() {
			assertPanicWith(func() {
				Register(Type{Name: "t", Entries: []Entry{{Key: "", Value: "a"}}})
			}, "empty key")
		})

		Convey("空 value panic", func() {
			assertPanicWith(func() {
				Register(Type{Name: "t", Entries: []Entry{{Key: "a", Value: ""}}})
			}, "empty value")
		})
	})
}

func TestTypeResolve(t *testing.T) {
	Convey("枚举类解析", t, func() {
		typ := &Type{
			Name: "tagRuleModel",
			Entries: []Entry{
				{Key: "httpTransport", Value: "zhongkui.com/zhongkui/internal/models/network.HttpTransport"},
			},
		}

		Convey("LabelKey 生成共享前缀键", func() {
			So(typ.LabelKey("httpTransport"), ShouldEqual, "enum.tagRuleModel.httpTransport")
		})

		Convey("按 value 解析命中（数据库存储语义）", func() {
			e, ok := typ.Resolve("zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			So(ok, ShouldBeTrue)
			So(e.Key, ShouldEqual, "httpTransport")
			So(e.Value, ShouldEqual, "zhongkui.com/zhongkui/internal/models/network.HttpTransport")
		})

		Convey("按 key 解析命中（展示语义）", func() {
			e, ok := typ.Resolve("httpTransport")
			So(ok, ShouldBeTrue)
			So(e.Key, ShouldEqual, "httpTransport")
		})

		Convey("未命中返回 false", func() {
			_, ok := typ.Resolve("unknown")
			So(ok, ShouldBeFalse)
		})

		Convey("EntryByKey / EntryByValue", func() {
			_, ok := typ.EntryByKey("httpTransport")
			So(ok, ShouldBeTrue)
			_, ok = typ.EntryByKey("nope")
			So(ok, ShouldBeFalse)
			_, ok = typ.EntryByValue("zhongkui.com/zhongkui/internal/models/network.HttpTransport")
			So(ok, ShouldBeTrue)
			_, ok = typ.EntryByValue("nope")
			So(ok, ShouldBeFalse)
		})
	})
}
