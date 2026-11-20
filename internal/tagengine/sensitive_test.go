package tagengine_test

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/models/tag"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// revealTestModel 敏感字段解密测试用的目标模型。
// ID 字段满足注册表的 UUID 锚点约束；Auth 为列内加密字段，Body 为 S3 对象字段。
type revealTestModel struct {
	ID        serializer.HexUUID  `json:"id"`
	Namespace string              `json:"namespace"`
	Method    string              `json:"method"`
	Auth      *serializer.Crypto  `json:"auth" crypto:"mode=\"aes-gcm\",offset=12000,length=32"`
	Body      serializer.S3Crypto `json:"body" s3crypto:"offset=12001,length=32"`
}

// revealTestModelKey 目标模型类型键。
var revealTestModelKey = func() string {
	t, err := tagengine.TypeKey(&revealTestModel{})
	if err != nil {
		panic("failed to compute reveal test model: " + err.Error())
	}
	return t
}()

// init 注册测试目标模型（注册表为包级全局状态，仅注册一次）。
func init() {
	tagengine.Register(tagengine.Target{Type: &revealTestModel{}})
}

// fakeObjectLoader 可计数的 S3 对象读取器，按对象键返回预置明文。
type fakeObjectLoader struct {
	mu       sync.Mutex
	calls    int
	contents map[string]string
}

// load 实现 tagengine.S3ObjectLoader。
func (l *fakeObjectLoader) load(_ context.Context, objectKey string, _ []byte) (io.ReadCloser, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	content, ok := l.contents[objectKey]
	if !ok {
		return nil, nil
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

// count 返回累计调用次数。
func (l *fakeObjectLoader) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// reset 清零调用次数。
func (l *fakeObjectLoader) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = 0
}

// newRevealEngine 创建绑定敏感字段运行时的测试引擎。
func newRevealEngine(t *testing.T, loader tagengine.S3ObjectLoader, maxBytes int64) (*tagengine.Engine, *database.DB) {
	t.Helper()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Core(t)
	engine := tagengine.NewEngine(db, logger, nil, tagengine.WithSensitiveRuntime(loader, maxBytes))
	return engine, db
}

// insertRevealRule 插入一条针对测试目标模型的启用规则。
func insertRevealRule(t *testing.T, db *database.DB, name, rego string) {
	t.Helper()
	rule := &tag.TagRule{
		NamespaceName: "default",
		Name:          name,
		Model:         revealTestModelKey,
		Rego:          rego,
		Enabled:       true,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
}

// TestBuildInputSensitive 验证敏感字段的输入文档形态与 crypto.reveal 端到端解密。
func TestBuildInputSensitive(t *testing.T) {
	Convey("Test BuildInput sensitive fields", t, func() {
		Convey("敏感字段不出现在输入文档、普通字段保留", func() {
			model := &revealTestModel{
				Namespace: "default",
				Method:    "GET",
				Auth:      serializer.NewCrypto([]byte("Bearer secret")),
				Body:      serializer.NewS3Crypto("body-key"),
			}
			input, err := tagengine.BuildInput(model)
			So(err, ShouldBeNil)
			So(input["method"], ShouldEqual, "GET")
			So(input, ShouldNotContainKey, "auth")
			So(input, ShouldNotContainKey, "body")
			// WithoutSensitiveFields 返回副本，不修改调用方传入的输入文档
			So(model.Auth.Value(), ShouldResemble, []byte("Bearer secret"))
			So(model.Body.Key(), ShouldEqual, "body-key")
		})

		Convey("规则经 crypto.reveal 读取 Crypto 明文", func() {
			loader := &fakeObjectLoader{contents: map[string]string{}}
			engine, db := newRevealEngine(t, loader.load, 1024)
			insertRevealRule(t, db, "bearer", `package zki

tags contains "bearer-auth" if {
	startswith(crypto.reveal("auth"), "Bearer ")
}`)
			tags, err := engine.Recognize(context.Background(), &revealTestModel{
				Namespace: "default",
				Auth:      serializer.NewCrypto([]byte("Bearer demo-token")),
			})
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"bearer-auth"})
			// Crypto 解密不触发 S3 读取
			So(loader.count(), ShouldEqual, 0)
		})
	})
}

// TestRevealS3CryptoCache 验证 S3Crypto 解密成功，且同一数据内多条规则只下载一次。
func TestRevealS3CryptoCache(t *testing.T) {
	Convey("Test crypto.reveal S3Crypto cache", t, func() {
		loader := &fakeObjectLoader{contents: map[string]string{"body-key": "xx-top-secret-yy"}}
		engine, db := newRevealEngine(t, loader.load, 1024)
		insertRevealRule(t, db, "body-a", `package zki

tags contains "body-a" if {
	contains(crypto.reveal("body"), "top-secret")
}`)
		insertRevealRule(t, db, "body-b", `package zki

tags contains "body-b" if {
	crypto.reveal("body") == "xx-top-secret-yy"
}`)
		model := &revealTestModel{Namespace: "default", Body: serializer.NewS3Crypto("body-key")}

		Convey("组级识别：两条规则共享一次下载", func() {
			tags, err := engine.Recognize(context.Background(), model)
			So(err, ShouldBeNil)
			So(tags, ShouldResemble, []string{"body-a", "body-b"})
			So(loader.count(), ShouldEqual, 1)
		})

		Convey("逐规则明细评估：组级查询与各规则共享一次下载", func() {
			result, err := engine.RecognizeWithDetail(context.Background(), model)
			So(err, ShouldBeNil)
			So(result.Tags, ShouldResemble, []string{"body-a", "body-b"})
			So(loader.count(), ShouldEqual, 1)
		})
	})
}

// TestRevealS3CryptoLimit 验证超过正文上限时内置函数报错，规则不命中。
func TestRevealS3CryptoLimit(t *testing.T) {
	Convey("Test crypto.reveal S3Crypto limit", t, func() {
		loader := &fakeObjectLoader{contents: map[string]string{"body-key": "0123456789"}}
		engine, db := newRevealEngine(t, loader.load, 4)
		insertRevealRule(t, db, "body-large", `package zki

tags contains "body-large" if {
	contains(crypto.reveal("body"), "0123")
}`)
		tags, err := engine.Recognize(context.Background(), &revealTestModel{
			Namespace: "default",
			Body:      serializer.NewS3Crypto("body-key"),
		})
		So(err, ShouldBeNil)
		So(tags, ShouldBeEmpty)
	})
}

// TestRevealNonDecryptableField 验证不可解密字段与未绑定运行时均令规则不命中。
func TestRevealNonDecryptableField(t *testing.T) {
	Convey("Test crypto.reveal non-decryptable field", t, func() {
		loader := &fakeObjectLoader{contents: map[string]string{"body-key": "content"}}
		engine, db := newRevealEngine(t, loader.load, 1024)
		insertRevealRule(t, db, "plain-field", `package zki

tags contains "plain-field" if {
	crypto.reveal("method") == "GET"
}`)
		insertRevealRule(t, db, "no-runtime", `package zki

tags contains "no-runtime" if {
	contains(crypto.reveal("body"), "content")
}`)
		tags, err := engine.Recognize(context.Background(), &revealTestModel{
			Namespace: "default",
			Method:    "GET",
			Body:      serializer.NewS3Crypto("body-key"),
		})
		So(err, ShouldBeNil)
		So(tags, ShouldBeEmpty)
		So(loader.count(), ShouldEqual, 0)
	})
}

// TestRevealRuntimeMissing 验证未注入 S3 运行时（引擎无 loader）时 S3Crypto 规则不命中。
func TestRevealRuntimeMissing(t *testing.T) {
	Convey("Test crypto.reveal runtime missing", t, func() {
		logger, err := logging.NewDefaultLogger()
		So(err, ShouldBeNil)
		db := testdb.Core(t)
		engine := tagengine.NewEngine(db, logger, nil)
		insertRevealRule(t, db, "no-runtime", `package zki

tags contains "no-runtime" if {
	contains(crypto.reveal("body"), "content")
}`)
		tags, err := engine.Recognize(context.Background(), &revealTestModel{
			Namespace: "default",
			Body:      serializer.NewS3Crypto("body-key"),
		})
		So(err, ShouldBeNil)
		So(tags, ShouldBeEmpty)
	})
}
