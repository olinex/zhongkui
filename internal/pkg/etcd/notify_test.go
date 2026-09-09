package etcd

import (
	// stdlib
	"context"
	"testing"

	// third-party
	. "github.com/smartystreets/goconvey/convey"
)

// TestBroadcastChangeNoEtcd 未绑定 etcd 客户端时广播快速失败（写路径告警降级）。
func TestBroadcastChangeNoEtcd(t *testing.T) {
	Convey("Test BroadcastChange without etcd client", t, func() {
		err := BroadcastChange(context.Background(), nil, "test/change", struct{ Name string }{Name: "x"})
		So(err, ShouldNotBeNil)
		So(err, ShouldEqual, ErrChangeUnavailable)
	})
}
