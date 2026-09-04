package message

import (
	// stdlib
	"encoding/json"
	"testing"

	// third-party
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/pkg/resourcepool"
)

// TestMakeKey_Canonicalization 语义等价的渠道级配置（字段顺序不同）应映射到同一资源池 Key，
// 从而复用同一连接池；配置不同则映射到不同 Key。
// TestMakeKey_Canonicalization verifies semantically equivalent channel-level configs
// (differing field order) map to the same pool Key (thus sharing a pool), while
// different configs map to different keys.
func TestMakeKey_Canonicalization(t *testing.T) {
	Convey("规范化渠道级配置映射资源池 Key", t, func() {
		Convey("字段顺序不同的等价配置共享同一 Key", func() {
			a := makeKey(TypeEmail, json.RawMessage(`{"host":"a.com","port":465}`))
			b := makeKey(TypeEmail, json.RawMessage(`{"port":465,"host":"a.com"}`))
			So(a, ShouldEqual, b)
		})

		Convey("配置不同则 Key 不同", func() {
			a := makeKey(TypeEmail, json.RawMessage(`{"host":"a.com","port":465}`))
			b := makeKey(TypeEmail, json.RawMessage(`{"host":"b.com","port":465}`))
			So(a, ShouldNotEqual, b)
		})

		Convey("渠道类型不同则 Key 不同", func() {
			e := makeKey(TypeEmail, json.RawMessage(`{}`))
			w := makeKey(TypeWebhook, json.RawMessage(`{}`))
			So(e, ShouldNotEqual, w)
		})

		Convey("Key 属于 resourcepool 类型", func() {
			So(makeKey(TypeEmail, json.RawMessage(`{}`)), ShouldHaveSameTypeAs, resourcepool.Key(""))
		})
	})
}

// TestDecodeKey_Roundtrip makeKey 与 decodeKey 应为一对互逆运算：decodeKey 还原渠道类型，
// 并还原 makeKey 规范化（map 键升序）后的配置原文。
// TestDecodeKey_Roundtrip verifies makeKey/decodeKey are inverses: decodeKey recovers the
// channel type and the canonicalized (key-sorted) config stored by makeKey.
func TestDecodeKey_Roundtrip(t *testing.T) {
	Convey("makeKey 与 decodeKey 互逆", t, func() {
		typ := TypeWebhook
		config := json.RawMessage(`{"url":"https://x.example/hook","headers":{"X-Token":"s"}}`)
		gotTyp, gotConfig := decodeKey(makeKey(typ, config))
		So(gotTyp, ShouldEqual, typ)
		// makeKey 会规范化配置（map 键升序），故解码结果应与规范化后的字节序一致
		So(string(gotConfig), ShouldEqual, `{"headers":{"X-Token":"s"},"url":"https://x.example/hook"}`)
	})

	Convey("空配置解码回空对象", t, func() {
		_, gotConfig := decodeKey(makeKey(TypeEmail, nil))
		So(string(gotConfig), ShouldEqual, `{}`)
	})
}
