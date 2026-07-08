package capture

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// TestBuildObjectKey 测试 BuildObjectKey 函数
func TestBuildObjectKey(t *testing.T) {
	Convey("Test BuildObjectKey function", t, func() {
		Convey("当提供有效的参数时", func() {
			namespace := "prod"
			name := "network-capture"
			version := "v1.0"
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			sessionID, err := serializer.HexUUIDv4FromString("550e8400-e29b-41d4-a716-446655440000")
			So(err, ShouldBeNil)

			expectedKey := "prod/network-capture/v1.0/1776508200000_550e8400-e29b-41d4-a716-446655440000.zkcn"
			actualKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)

			So(actualKey, ShouldEqual, expectedKey)
		})

		Convey("当命名空间包含特殊字符时", func() {
			namespace := "test-namespace"
			name := "test-capture"
			version := "v2.1-beta"
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			sessionID, err := serializer.HexUUIDv4FromString("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
			So(err, ShouldBeNil)

			expectedKey := "test-namespace/test-capture/v2.1-beta/1776508200000_6ba7b810-9dad-11d1-80b4-00c04fd430c8.zkcn"
			actualKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)

			So(actualKey, ShouldEqual, expectedKey)
		})

		Convey("当版本号包含点号时", func() {
			namespace := "dev"
			name := "packet-capture"
			version := "1.2.3"
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			sessionID, err := serializer.HexUUIDv4FromString("123e4567-e89b-12d3-a456-426614174000")
			So(err, ShouldBeNil)

			expectedKey := "dev/packet-capture/1.2.3/1776508200000_123e4567-e89b-12d3-a456-426614174000.zkcn"
			actualKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)

			So(actualKey, ShouldEqual, expectedKey)
		})

		Convey("当时间戳为 Unix 纪元开始时", func() {
			namespace := "test"
			name := "test"
			version := "v1"
			timestamp := time.Unix(0, 0).UTC()
			sessionID, err := serializer.HexUUIDv4FromString("00000000-0000-0000-0000-000000000000")
			So(err, ShouldBeNil)

			expectedKey := "test/test/v1/0_00000000-0000-0000-0000-000000000000.zkcn"
			actualKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)

			So(actualKey, ShouldEqual, expectedKey)
		})

		Convey("当会话ID为最大UUID值时", func() {
			namespace := "max"
			name := "max-capture"
			version := "v1"
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			sessionID, err := serializer.HexUUIDv4FromString("ffffffff-ffff-ffff-ffff-ffffffffffff")
			So(err, ShouldBeNil)

			expectedKey := "max/max-capture/v1/1776508200000_ffffffff-ffff-ffff-ffff-ffffffffffff.zkcn"
			actualKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)

			So(actualKey, ShouldEqual, expectedKey)
		})
	})
}

// TestParseObjectKey 测试 ParseObjectKey 函数
func TestParseObjectKey(t *testing.T) {
	Convey("Test ParseObjectKey function", t, func() {
		Convey("当解析有效的对象键名时", func() {
			// 使用与 BuildObjectKey 测试中相同的时间戳
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			objectKey := "prod/network-capture/v1.0/1776508200000_550e8400-e29b-41d4-a716-446655440000.zkcn"

			namespace, name, version, parsedTimestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeTrue)
			So(namespace, ShouldEqual, "prod")
			So(name, ShouldEqual, "network-capture")
			So(version, ShouldEqual, "v1.0")
			So(parsedTimestamp.UnixMilli(), ShouldEqual, timestamp.UnixMilli())
			So(sessionID.String(), ShouldEqual, "550e8400-e29b-41d4-a716-446655440000")
		})

		Convey("当解析包含特殊字符的对象键名时", func() {
			// 使用与 BuildObjectKey 测试中相同的时间戳
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			objectKey := "test-namespace/test-capture/v2.1-beta/1776508200000_6ba7b810-9dad-11d1-80b4-00c04fd430c8.zkcn"

			namespace, name, version, parsedTimestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeTrue)
			So(namespace, ShouldEqual, "test-namespace")
			So(name, ShouldEqual, "test-capture")
			So(version, ShouldEqual, "v2.1-beta")
			So(parsedTimestamp.UnixMilli(), ShouldEqual, timestamp.UnixMilli())
			So(sessionID.String(), ShouldEqual, "6ba7b810-9dad-11d1-80b4-00c04fd430c8")
		})

		Convey("当解析包含点号版本号的对象键名时", func() {
			// 使用与 BuildObjectKey 测试中相同的时间戳
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			objectKey := "dev/packet-capture/1.2.3/1776508200000_123e4567-e89b-12d3-a456-426614174000.zkcn"

			namespace, name, version, parsedTimestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeTrue)
			So(namespace, ShouldEqual, "dev")
			So(name, ShouldEqual, "packet-capture")
			So(version, ShouldEqual, "1.2.3")
			So(parsedTimestamp.UnixMilli(), ShouldEqual, timestamp.UnixMilli())
			So(sessionID.String(), ShouldEqual, "123e4567-e89b-12d3-a456-426614174000")
		})

		Convey("当对象键名缺少部分时", func() {
			objectKey := "prod/network-capture/1776508200000_550e8400-e29b-41d4-a716-446655440000.zkcn"

			namespace, name, version, timestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			So(namespace, ShouldBeEmpty)
			So(name, ShouldBeEmpty)
			So(version, ShouldBeEmpty)
			So(timestamp.IsZero(), ShouldBeTrue)
			So(sessionID.IsNil(), ShouldBeTrue)
		})

		Convey("当对象键名缺少 .zkcn 后缀时", func() {
			objectKey := "prod/network-capture/v1.0/1776508200000_550e8400-e29b-41d4-a716-446655440000"

			namespace, name, version, timestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			So(namespace, ShouldBeEmpty)
			So(name, ShouldBeEmpty)
			So(version, ShouldBeEmpty)
			So(timestamp.IsZero(), ShouldBeTrue)
			So(sessionID.IsNil(), ShouldBeTrue)
		})

		Convey("当对象键名缺少下划线分隔符时", func() {
			objectKey := "prod/network-capture/v1.0/1776508200000550e8400-e29b-41d4-a716-446655440000.zkcn"

			namespace, name, version, timestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			So(namespace, ShouldBeEmpty)
			So(name, ShouldBeEmpty)
			So(version, ShouldBeEmpty)
			So(timestamp.IsZero(), ShouldBeTrue)
			So(sessionID.IsNil(), ShouldBeTrue)
		})

		Convey("当时间戳部分不是数字时", func() {
			objectKey := "prod/network-capture/v1.0/not-a-number_550e8400-e29b-41d4-a716-446655440000.zkcn"

			namespace, name, version, timestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			So(namespace, ShouldBeEmpty)
			So(name, ShouldBeEmpty)
			So(version, ShouldBeEmpty)
			So(timestamp.IsZero(), ShouldBeTrue)
			So(sessionID.IsNil(), ShouldBeTrue)
		})

		Convey("当会话ID部分不是有效的UUID时", func() {
			objectKey := "prod/network-capture/v1.0/1776508200000_not-a-valid-uuid.zkcn"

			_, _, _, _, _, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			// 当 ok 为 false 时，其他返回值可能不是零值
			// 我们只检查 ok 是否为 false，不检查其他返回值
		})

		Convey("当对象键名包含额外的斜杠时", func() {
			objectKey := "prod/network-capture/v1.0/extra/1776508200000_550e8400-e29b-41d4-a716-446655440000.zkcn"

			namespace, name, version, timestamp, sessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeFalse)
			So(namespace, ShouldBeEmpty)
			So(name, ShouldBeEmpty)
			So(version, ShouldBeEmpty)
			So(timestamp.IsZero(), ShouldBeTrue)
			So(sessionID.IsNil(), ShouldBeTrue)
		})
	})
}

// TestBuildAndParseRoundTrip 测试 BuildObjectKey 和 ParseObjectKey 的往返一致性
func TestBuildAndParseRoundTrip(t *testing.T) {
	Convey("Test BuildObjectKey and ParseObjectKey round-trip consistency", t, func() {
		Convey("当构建后立即解析时", func() {
			namespace := "prod"
			name := "network-capture"
			version := "v1.0"
			timestamp := time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC)
			sessionID, err := serializer.HexUUIDv4FromString("550e8400-e29b-41d4-a716-446655440000")
			So(err, ShouldBeNil)

			objectKey := BuildObjectKey(namespace, name, version, timestamp, sessionID)
			parsedNamespace, parsedName, parsedVersion, parsedTimestamp, parsedSessionID, ok := ParseObjectKey(objectKey)

			So(ok, ShouldBeTrue)
			So(parsedNamespace, ShouldEqual, namespace)
			So(parsedName, ShouldEqual, name)
			So(parsedVersion, ShouldEqual, version)
			So(parsedTimestamp.UnixMilli(), ShouldEqual, timestamp.UnixMilli())
			So(parsedSessionID.String(), ShouldEqual, sessionID.String())
		})

		Convey("当使用不同的参数进行多次往返测试时", func() {
			testCases := []struct {
				testName  string
				namespace string
				name      string
				version   string
				timestamp time.Time
				sessionID string
			}{
				{
					testName:  "基本测试",
					namespace: "prod",
					name:      "capture",
					version:   "v1",
					timestamp: time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC),
					sessionID: "550e8400-e29b-41d4-a716-446655440000",
				},
				{
					testName:  "开发环境测试",
					namespace: "dev",
					name:      "test-capture",
					version:   "v2.1-beta",
					timestamp: time.Date(2026, 4, 19, 14, 45, 30, 500000000, time.UTC),
					sessionID: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				},
				{
					testName:  "特殊字符测试",
					namespace: "test-namespace",
					name:      "test_name",
					version:   "1.2.3.4",
					timestamp: time.Date(2026, 4, 20, 23, 59, 59, 999999999, time.UTC),
					sessionID: "123e4567-e89b-12d3-a456-426614174000",
				},
			}

			for _, tc := range testCases {
				Convey(tc.testName, func() {
					sessionID, err := serializer.HexUUIDv4FromString(tc.sessionID)
					So(err, ShouldBeNil)
					objectKey := BuildObjectKey(tc.namespace, tc.name, tc.version, tc.timestamp, sessionID)
					parsedNamespace, parsedName, parsedVersion, parsedTimestamp, parsedSessionID, ok := ParseObjectKey(objectKey)

					So(ok, ShouldBeTrue)
					So(parsedNamespace, ShouldEqual, tc.namespace)
					So(parsedName, ShouldEqual, tc.name)
					So(parsedVersion, ShouldEqual, tc.version)
					So(parsedTimestamp.UnixMilli(), ShouldEqual, tc.timestamp.UnixMilli())
					So(parsedSessionID.String(), ShouldEqual, sessionID.String())
				})
			}
		})
	})
}
