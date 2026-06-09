package capture

import (
	"sync"
	"time"

	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
)

// captureSnapshot 表示捕获状态的快照
// 该结构体用于在需要时获取捕获状态的只读副本，避免并发访问问题
type captureSnapshot struct {
	status        reporter_v1.StatusType // 当前捕获状态（运行中、已停止等）
	config        *reporter_v1.Config    // 当前配置信息
	configVersion int64                  // 配置版本号，用于检测配置变更
	globalVersion int64                  // 全局版本号，用于同步状态
	prevHeartbeat time.Time              // 上一次心跳时间
}

// captureMutex 是带有互斥锁的捕获状态管理器
// 该结构体提供了线程安全的捕获状态管理，并支持状态变更通知
type captureMutex struct {
	lock          sync.RWMutex           // 读写锁，保证并发安全
	status        reporter_v1.StatusType // 当前捕获状态
	config        *reporter_v1.Config    // 当前配置
	configVersion int64                  // 配置版本号
	globalVersion int64                  // 全局版本号
	prevHeartbeat time.Time              // 上一次心跳时间
	notifyChan    chan *captureSnapshot  // 状态变更通知通道
}

// newCaptureMutex 创建并初始化一个新的 captureMutex 实例
// notifyChan: 状态变更时发送通知的通道，当状态发生变化时会发送快照到该通道
// 返回值: 初始化后的 captureMutex 指针，初始状态为 STOPPED
func newCaptureMutex() *captureMutex {
	return &captureMutex{
		lock:          sync.RWMutex{},
		status:        reporter_v1.StatusType_STOPPED,
		config:        nil,
		configVersion: 0,
		globalVersion: 0,
		prevHeartbeat: time.Time{},
		notifyChan:    make(chan *captureSnapshot, 1),
	}
}

func (cm *captureMutex) snapshots() <-chan *captureSnapshot {
	return cm.notifyChan
}

// update 更新捕获配置和版本信息
// 该方法会原子性地更新配置、配置版本和全局版本，并在更新后发送状态变更通知
// config: 新的捕获配置，可以为 nil 表示清除配置
// configVersion: 新的配置版本号，通常由配置管理系统提供
// globalVersion: 新的全局版本号，用于跨节点状态同步
func (cm *captureMutex) update(
	config *reporter_v1.Config,
	configVersion int64,
	globalVersion int64,
) {
	cm.lock.Lock()
	cm.config = config
	cm.configVersion = configVersion
	cm.globalVersion = globalVersion
	cm.lock.Unlock()
	cm.notifyChan <- cm.snapshot()
}

// reset 重置捕获状态到初始值
// 该方法会清除所有配置信息、重置版本号和时间戳，并将状态变更通知发送到通知通道
// 通常用于捕获任务结束或发生错误时清理状态
func (cm *captureMutex) reset() {
	cm.lock.Lock()
	cm.config = nil
	cm.configVersion = 0
	cm.globalVersion = 0
	cm.prevHeartbeat = time.Time{}
	cm.lock.Unlock()
	cm.notifyChan <- cm.snapshot()
}

// asRunning 将捕获状态设置为运行中
// 该方法会原子性地更新状态为 RUNNING，表示捕获任务正在执行
// 注意：该方法不会发送状态变更通知，通常与配置更新一起使用
func (cm *captureMutex) asRunning() {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.status = reporter_v1.StatusType_RUNNING
}

// asStopped 将捕获状态设置为已停止
// 该方法会原子性地更新状态为 STOPPED，表示捕获任务已停止
// 注意：该方法不会发送状态变更通知，通常与配置更新一起使用
func (cm *captureMutex) asStopped() {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.status = reporter_v1.StatusType_STOPPED
}

// heartbeat 更新心跳时间戳
// 该方法用于记录捕获任务的最新活动时间，用于健康检查和超时检测
// timestamp: 心跳发生的时间戳，通常使用 time.Now() 获取当前时间
func (cm *captureMutex) heartbeat(timestamp time.Time) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.prevHeartbeat = timestamp
}

// snapshot 创建当前捕获状态的快照
// 该方法返回一个只读的状态副本，包含深拷贝的配置信息，确保快照的不可变性
// 返回值: 当前状态的 captureSnapshot 指针，可以安全地在并发环境中使用
func (cm *captureMutex) snapshot() *captureSnapshot {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	var cfg *reporter_v1.Config
	if cm.config != nil {
		// 深拷贝配置，确保快照的不可变性
		cfg = &reporter_v1.Config{
			Device:  cm.config.Device,
			SnapLen: cm.config.SnapLen,
			Promisc: cm.config.Promisc,
			Timeout: cm.config.Timeout,
			BpfExpr: cm.config.BpfExpr,
			Enabled: cm.config.Enabled,
		}
	} else {
		cfg = nil
	}
	return &captureSnapshot{
		status:        cm.status,
		config:        cfg,
		configVersion: cm.configVersion,
		globalVersion: cm.globalVersion,
		prevHeartbeat: cm.prevHeartbeat,
	}
}
