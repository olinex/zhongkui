package capture

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	reporter_v1 "zhongkuisec.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// newTestCaptureClient 创建一个用于测试的 CaptureClient，使用临时目录作为 DumpDir
// s3 和 gRPC client 传 nil 即可，本测试不依赖 Run 流程
func newTestCaptureClient(t *testing.T, dumpDir string) *CaptureClient {
	t.Helper()
	logger, err := logging.NewProductionLogger(&logging.LoggerCfg{
		Level:            zap.NewAtomicLevelAt(zap.DebugLevel),
		Development:      true,
		Encoding:         "console",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	})
	So(err, ShouldBeNil)
	return &CaptureClient{
		cfg: &Config{
			Resource: reporter_v1.ResourceRaw{
				Namespace: "test",
				Name:      "test-capture",
				Version:   "v1.0.0",
			},
			Upload: UploadConfig{
				DumpDir: dumpDir,
			},
		},
		logger: logger,
	}
}

// createTempZKCN 在指定目录创建一个模拟的 zkcn 文件，返回文件路径
func createTempZKCN(t *testing.T, dir, filename string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte("mock zkcn content"), 0644)
	So(err, ShouldBeNil)
	return path
}

// syncMapLen 返回 sync.Map 中的元素个数
func syncMapLen(m *sync.Map) int {
	count := 0
	m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// TestCaptureClient_Shutdown 验证 CaptureClient 关闭时对已追踪临时文件的清理行为。
func TestCaptureClient_Shutdown(t *testing.T) {
	Convey("CaptureClient Shutdown 临时文件清理测试", t, func() {

		Convey("Shutdown 应清理所有已追踪的临时文件", func() {
			dumpDir := t.TempDir()
			client := newTestCaptureClient(t, dumpDir)

			// 模拟 dump 阶段创建了 3 个 zkcn 文件并注册到 pendingFiles
			file1 := createTempZKCN(t, dumpDir, "1000.session1.zkcn")
			file2 := createTempZKCN(t, dumpDir, "2000.session1.zkcn")
			file3 := createTempZKCN(t, dumpDir, "3000.session1.zkcn")

			client.trackFile(file1)
			client.trackFile(file2)
			client.trackFile(file3)

			// 执行 Shutdown
			err := client.Shutdown()
			So(err, ShouldBeNil)

			// 验证所有文件已被删除
			_, err = os.Stat(file1)
			So(os.IsNotExist(err), ShouldBeTrue)

			_, err = os.Stat(file2)
			So(os.IsNotExist(err), ShouldBeTrue)

			_, err = os.Stat(file3)
			So(os.IsNotExist(err), ShouldBeTrue)

			// 验证 pendingFiles 已清空
			So(syncMapLen(&client.pendingFiles), ShouldEqual, 0)
		})

		Convey("Shutdown 只清理未 untrack 的文件（模拟部分文件已上传成功）", func() {
			dumpDir := t.TempDir()
			client := newTestCaptureClient(t, dumpDir)

			// 模拟 2 个文件被创建
			uploadedFile := createTempZKCN(t, dumpDir, "1000.uploaded.zkcn")
			pendingFile := createTempZKCN(t, dumpDir, "2000.pending.zkcn")

			// 两个文件都先注册
			client.trackFile(uploadedFile)
			client.trackFile(pendingFile)

			// 模拟 uploadedFile 上传成功，调用 untrackFile
			client.untrackFile(uploadedFile)
			// 同时手动删除它（模拟 uploadOne 的 os.Remove）
			err := os.Remove(uploadedFile)
			So(err, ShouldBeNil)

			// 执行 Shutdown — 应该只清理 pendingFile
			err = client.Shutdown()
			So(err, ShouldBeNil)

			// pendingFile 应被 Shutdown 删除
			_, err = os.Stat(pendingFile)
			So(os.IsNotExist(err), ShouldBeTrue)

			// pendingFiles 已清空
			So(syncMapLen(&client.pendingFiles), ShouldEqual, 0)
		})

		Convey("Shutdown 对不存在的文件不应报错", func() {
			dumpDir := t.TempDir()
			client := newTestCaptureClient(t, dumpDir)

			nonExistentPath := filepath.Join(dumpDir, "9999.nonexistent.zkcn")
			client.trackFile(nonExistentPath)

			// Shutdown 即使文件不存在也应正常返回
			err := client.Shutdown()
			So(err, ShouldBeNil)
		})

		Convey("Shutdown 空 pendingFiles 不应报错", func() {
			dumpDir := t.TempDir()
			client := newTestCaptureClient(t, dumpDir)

			err := client.Shutdown()
			So(err, ShouldBeNil)
		})

		Convey("trackFile 和 untrackFile 并发安全", func() {
			dumpDir := t.TempDir()
			client := newTestCaptureClient(t, dumpDir)

			// 并发注册和取消注册
			done := make(chan struct{})
			go func() {
				for i := 0; i < 100; i++ {
					client.trackFile(filepath.Join(dumpDir, "concurrent.zkcn"))
				}
				close(done)
			}()

			for i := 0; i < 100; i++ {
				client.untrackFile(filepath.Join(dumpDir, "concurrent.zkcn"))
			}

			<-done

			// 不应 panic，pendingFiles 状态应一致
			err := client.Shutdown()
			So(err, ShouldBeNil)
		})
	})
}
