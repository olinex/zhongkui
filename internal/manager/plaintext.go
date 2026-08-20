package manager

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/network/detectors"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/crypto/mode"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/tag"
)

// 明文查看模型注册表：
// 注册名（name）全局唯一，既是权限点 object 的 model 段（见 internal/pkg/base/consts.go 格式规范），
// 也是明文接口请求体 model 字段的值。新增模型的加密/S3 字段只需注册模型 + 补模板种子，无需新增接口。
var (
	plaintextModels   = make(map[string]any)
)

// registerPlaintextModel 注册支持明文查看的模型
// name 为全局唯一模型注册名（如 "ipv4-tcp"、"http-transport"）；
// 重复注册同名模型属编程错误，直接 panic 以在注册期暴露。
func registerPlaintextModel(name string, model any) {
	if _, ok := plaintextModels[name]; ok {
		panic(fmt.Sprintf("plaintext: model %q already registered", name))
	}
	plaintextModels[name] = model
}

// CryptoPlaintextIn 加密字段明文查看请求输入
// Model 为模型注册名，Field 为字段 json 名（二者构成权限点 object），Ciphertext 为字段密文的 base64 编码
type CryptoPlaintextIn struct {
	Model      string `json:"model" binding:"required"`
	Field      string `json:"field" binding:"required"`
	Ciphertext string `json:"ciphertext" binding:"required"`
}

// S3PlaintextIn S3 body 明文查看请求输入
// Model/Field 构成权限点 object，ObjectKey 为 body 内容在 S3 中的对象键
type S3PlaintextIn struct {
	Model     string `json:"model" binding:"required"`
	Field     string `json:"field" binding:"required"`
	ObjectKey string `json:"object_key" binding:"required"`
}

// CryptoPlaintextGet 处理加密字段明文查看请求（全局通用接口）
// 请求携带模型注册名 + 字段 + base64 密文；先按字段构成权限点 object 做 RBAC 静态授权，
// 再按模型字段的 secure 标签派生密钥解密，以 application/octet-stream 返回明文。
// 新增模型的加密字段仅需 RegisterPlaintextModel 注册模型 + 补模板种子，无需新增接口。
func CryptoPlaintextGet(c *gin.Context) {
	var in CryptoPlaintextIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := authorizePlaintext(c, base.PlaintextCrypto, in.Model, in.Field); err != nil {
		err.AbortWithGinH(c)
		return
	}
	model, ok := plaintextModels[in.Model]
	if !ok {
		CreateBadBodyError(errors.Newf("plaintext: model %q not registered", in.Model)).AbortWithGinH(c)
		return
	}
	spec, err := extractCryptoFieldTag(model, in.Field)
	if err != nil {
		CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	raw, err := base64.StdEncoding.DecodeString(in.Ciphertext)
	if err != nil {
		CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	kd := key.GetKeyDerivation()
	if kd == nil {
		CreateAccidentError(errors.New("crypto: KeyDerivation not initialized")).AbortWithGinH(c)
		return
	}
	cryptoKey, err := kd.DeriveChildKey(spec.Offset, spec.Length)
	if err != nil {
		CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	m, err := mode.NewAESGCMMode(cryptoKey, nil)
	if err != nil {
		CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	plain, err := m.Decrypt(raw)
	if err != nil {
		CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", plain)
}

// S3PlaintextGet 处理 S3 body 明文查看请求（全局通用接口）
// body 内容以 Zkcn（zstd + AES-GCM）加密存储在 S3，请求体 {object_key} 指定对象键；
// 先按 model + field 构成权限点 object 做 RBAC 静态授权，再读取并解密解压后以
// application/octet-stream 返回原始字节流。
// 新增模型的 S3 字段仅需 RegisterPlaintextModel 注册模型 + 补模板种子，无需新增接口。
func S3PlaintextGet(c *gin.Context) {
	var in S3PlaintextIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := authorizePlaintext(c, base.PlaintextS3, in.Model, in.Field); err != nil {
		err.AbortWithGinH(c)
		return
	}
	if _, ok := plaintextModels[in.Model]; !ok {
		CreateBadBodyError(errors.Newf("plaintext: model %q not registered", in.Model)).AbortWithGinH(c)
		return
	}
	kd := key.GetKeyDerivation()
	if kd == nil {
		CreateAccidentError(errors.New("crypto: KeyDerivation not initialized")).AbortWithGinH(c)
		return
	}
	cryptoKey, err := kd.DeriveChildKey(detectors.CryptoKeyOffset, detectors.CryptoKeyLength)
	if err != nil {
		CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	rc, err := s3.NewZkcn(GetS3Client(c), in.ObjectKey, cryptoKey, 0).Load(c.Request.Context())
	if err != nil {
		if s3.IsNotExists(err) {
			CreateCodeError(http.StatusNotFound, http.StatusNotFound, nil).AbortWithGinH(c)
			return
		}
		CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// plaintextObject 构造明文查看权限点 object
// 格式规范见 internal/pkg/base/consts.go：{prefix}:{subtype}:{model}:{field}
func plaintextObject(scope, model, field string) string {
	return base.PlaintextPrefix + ":" + scope + ":" + model + ":" + field
}

// authorizePlaintext 明文查看权限点静态授权
// object = plaintext:{crypto|s3}:{model}:{field}，action = READ
func authorizePlaintext(c *gin.Context, scope, model, field string) *CodeError {
	namespace, err := GetNamespace(c)
	if err != nil {
		return err
	}
	_, span := StartHandlerSpan(c, "plaintext-authorize")
	defer span.End()
	return StaticAuthorize(c, span, namespace, plaintextObject(scope, model, field), base.ActionRead)
}

// extractCryptoFieldTag 从模型中解析指定字段的 secure 标签（加密参数）
// 通过 GORM schema 解析，与 GORM 列解析结果完全一致（含匿名嵌入字段）
func extractCryptoFieldTag(model any, field string) (serializer.CryptoTag, error) {
	s, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		return serializer.CryptoTag{}, errors.WithStack(err)
	}
	for _, f := range s.Fields {
		jsonName, _, _ := strings.Cut(f.StructField.Tag.Get("json"), ",")
		if jsonName != field {
			continue
		}
		secure := f.StructField.Tag.Get(serializer.SecurityTagKey)
		if secure == "" {
			return serializer.CryptoTag{}, errors.Newf("field %s is not a secure field", field)
		}
		var spec serializer.CryptoTag
		if err := tag.Unmarshal(secure, &spec); err != nil {
			return serializer.CryptoTag{}, errors.WithStack(err)
		}
		return spec, nil
	}
	return serializer.CryptoTag{}, errors.Newf("field %s not found in model %T", field, model)
}

func init() {
	// 注册支持明文查看的流量模型（注册名即权限点 object 的 model 段，与前端 type slug 一致）
	registerPlaintextModel("ipv4-tcp", &network.PacketIPv4TCP{})
	registerPlaintextModel("ipv6-tcp", &network.PacketIPv6TCP{})
	registerPlaintextModel("http-transport", &network.HttpTransport{})
}
