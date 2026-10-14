package enum

import (
	// 标准库
	"testing"

	// 第三方
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

// TestRegisterAndGet 测试枚举类的注册、按名查询、存在性判断与列表排序。
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

// TestRegisterValidation 测试枚举类注册时的各类非法输入均触发 panic。
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

// TestTypeResolve 测试枚举条目的 label 键生成与按 value/key 解析。
func TestTypeResolve(t *testing.T) {
	Convey("枚举类解析", t, func() {
		typ := &Type{
			Name: "tagRuleModel",
			Entries: []Entry{
				{Key: "httpTransport", Value: "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport"},
			},
		}

		Convey("LabelKey 生成共享前缀键", func() {
			So(typ.LabelKey("httpTransport"), ShouldEqual, "enum.tagRuleModel.httpTransport")
		})

		Convey("按 value 解析命中（数据库存储语义）", func() {
			e, ok := typ.Resolve("zhongkuisec.com/zhongkui/internal/models/network.HttpTransport")
			So(ok, ShouldBeTrue)
			So(e.Key, ShouldEqual, "httpTransport")
			So(e.Value, ShouldEqual, "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport")
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
			_, ok = typ.EntryByValue("zhongkuisec.com/zhongkui/internal/models/network.HttpTransport")
			So(ok, ShouldBeTrue)
			_, ok = typ.EntryByValue("nope")
			So(ok, ShouldBeFalse)
		})
	})
}

// TestRegisterSource 测试惰性条目来源的注册、读取派生与注册冲突校验。
func TestRegisterSource(t *testing.T) {
	Convey("惰性条目来源注册", t, func() {
		Reset(func() {
			registry = map[string]*Type{}
			sources = map[string]func() []Entry{}
		})

		Convey("条目在每次读取时从来源派生（注册时无快照）", func() {
			values := []string{"email"}
			RegisterSource("channelType", func() []Entry { return EntriesOf(values) })

			typ, ok := Get("channelType")
			So(ok, ShouldBeTrue)
			So(len(typ.Entries), ShouldEqual, 1)

			// 来源注册表随后扩展（同包内后续 init 填充），读取即随动——不依赖 init 顺序
			values = append(values, "webhook")
			typ, ok = Get("channelType")
			So(ok, ShouldBeTrue)
			So(len(typ.Entries), ShouldEqual, 2)

			types := List()
			So(len(types), ShouldEqual, 1)
			So(len(types[0].Entries), ShouldEqual, 2)
		})

		Convey("类名重复 panic", func() {
			RegisterSource("dup", func() []Entry { return nil })
			assertPanicWith(func() {
				RegisterSource("dup", func() []Entry { return nil })
			}, "duplicate type name")
			assertPanicWith(func() {
				Register(Type{Name: "dup2"})
				RegisterSource("dup2", func() []Entry { return nil })
			}, "duplicate type name")
		})

		Convey("空类名或 nil 来源 panic", func() {
			assertPanicWith(func() {
				RegisterSource("", func() []Entry { return nil })
			}, "empty type name")
			assertPanicWith(func() {
				RegisterSource("nilSrc", nil)
			}, "nil entries source")
		})

		Convey("来源条目违规时读取即 panic", func() {
			RegisterSource("bad", func() []Entry {
				return []Entry{{Key: "a", Value: "a"}, {Key: "a", Value: "b"}}
			})
			assertPanicWith(func() {
				Get("bad")
			}, "duplicate key")
		})
	})
}
