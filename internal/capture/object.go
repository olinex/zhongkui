package capture

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"zhongkui.com/zhongkui/internal/pkg/crypto/combine"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// BuildObjectKey 构建对象存储键名
// 根据给定的命名空间、名称、版本、时间戳和会话ID生成唯一的对象键名
// 键名格式为：namespace/name/version/timestamp_sessionID.zstd
//
// 参数:
//   - namespace: 命名空间，用于区分不同的业务或环境
//   - name: 对象名称，标识具体的对象类型
//   - version: 版本号，用于版本控制
//   - timestamp: 时间戳，表示对象创建的时间
//   - sessionID: 会话ID，用于唯一标识一次会话
//
// 返回:
//   - string: 构建的对象键名，格式为 namespace/name/version/timestamp_sessionID.zstd
//
// 示例:
//
//	BuildObjectKey("prod", "network-capture", "v1.0", time.Now(), sessionID)
//	返回: "prod/network-capture/v1.0/1745012345678_550e8400-e29b-41d4-a716-446655440000.zstd"
func BuildObjectKey(namespace, name, version string, timestamp time.Time, sessionID serializer.HexUUID) string {
	return fmt.Sprintf(
		`%s/%s/%s/%d_%s.%s`,
		namespace,
		name,
		version,
		timestamp.UnixMilli(),
		sessionID.String(),
		combine.FILE_EXT,
	)
}

// ParseObjectKey 解析对象存储键名
// 将对象键名解析为其组成部分：命名空间、名称、版本、时间戳和会话ID
// 键名格式必须为：namespace/name/version/timestamp_sessionID.zstd
//
// 参数:
//   - objectKey: 对象键名，格式为 namespace/name/version/timestamp_sessionID.zstd
//
// 返回:
//   - namespace: 命名空间，从键名中解析出的命名空间部分
//   - name: 对象名称，从键名中解析出的对象名称部分
//   - version: 版本号，从键名中解析出的版本号部分
//   - timestamp: 时间戳，从键名中解析出的时间戳（Unix毫秒时间戳）
//   - sessionID: 会话ID，从键名中解析出的会话ID
//   - ok: 解析是否成功，true表示成功解析，false表示键名格式无效
//
// 解析规则:
//  1. 键名必须包含4个部分，由"/"分隔
//  2. 第四部分必须以".zstd"结尾
//  3. 第四部分去掉".zstd"后缀后，必须包含一个"_"分隔符
//  4. "_"分隔符前为Unix毫秒时间戳，后为会话ID字符串
//
// 示例:
//
//	ParseObjectKey("prod/network-capture/v1.0/1745012345678_550e8400-e29b-41d4-a716-446655440000.zstd")
//	返回: "prod", "network-capture", "v1.0", time.UnixMilli(1745012345678), sessionID, true
func ParseObjectKey(objectKey string) (namespace, name, version string, timestamp time.Time, sessionID serializer.HexUUID, ok bool) {
	sourceParts := strings.Split(objectKey, "/")
	if len(sourceParts) != 4 {
		return
	}
	tmp := sourceParts[3]
	if !strings.HasSuffix(tmp, combine.FILE_DOT_EXT) {
		return
	}
	tmp = strings.TrimRight(tmp, combine.FILE_DOT_EXT)
	nameParts := strings.Split(tmp, "_")
	if len(nameParts) != 2 {
		return
	}
	// 解析时间戳部分
	timestampMillis, err := strconv.ParseInt(nameParts[0], 10, 64)
	if err != nil {
		return
	}
	timestamp = time.UnixMilli(timestampMillis)
	namespace = sourceParts[0]
	name = sourceParts[1]
	version = sourceParts[2]
	// 解析会话ID - 使用安全的UUID解析
	if sessionID, err = serializer.HexUUIDv4FromString(nameParts[1]); err == nil {
		ok = true
	}
	return
}
