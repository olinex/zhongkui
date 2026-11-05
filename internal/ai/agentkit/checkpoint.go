package agentkit

import (
	"context"
	"sync"

	"github.com/cloudwego/eino/adk"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// 编译期确认实现满足 eino 的 checkpoint 存储契约。
var (
	_ adk.CheckPointStore   = (*MemoryCheckPointStore)(nil)
	_ adk.CheckPointDeleter = (*MemoryCheckPointStore)(nil)
	_ adk.CheckPointStore   = (*DBCheckPointStore)(nil)
	_ adk.CheckPointDeleter = (*DBCheckPointStore)(nil)
)

// MemoryCheckPointStore 是 eino checkpoint 存储的进程内实现：按 checkpoint ID 保存运行现场
// 字节。供测试与无数据库场景使用；生产运行使用 DBCheckPointStore 持久化到 core。
type MemoryCheckPointStore struct {
	// mu 保护 data（运行可能并发写同一 store）
	mu sync.Mutex
	// data checkpoint ID → 序列化后的现场字节
	data map[string][]byte
}

// NewMemoryCheckPointStore 构造一个空的进程内 checkpoint 存储。
func NewMemoryCheckPointStore() *MemoryCheckPointStore {
	return &MemoryCheckPointStore{data: make(map[string][]byte)}
}

// Get 读取指定 ID 的 checkpoint：不存在时返回 (nil, false, nil)。返回副本，调用方修改不影响存储。
func (s *MemoryCheckPointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.data[id]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), raw...), true, nil
}

// Set 写入指定 ID 的 checkpoint；入参字节被复制，调用方后续修改不影响存储。
func (s *MemoryCheckPointStore) Set(_ context.Context, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = append([]byte(nil), data...)
	return nil
}

// Delete 删除指定 ID 的 checkpoint（幂等）。
func (s *MemoryCheckPointStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
	return nil
}

// DBCheckPointStore 是 eino checkpoint 存储的 core（PostgreSQL）实现：按 checkpoint ID 把
// 运行现场加密落库（agent_checkpoints），使挂起现场跨进程 / 进程重启存活。
type DBCheckPointStore struct {
	// db core 库连接（agent_checkpoints 读写）
	db *database.DB
}

// NewDBCheckPointStore 构造 core 库 checkpoint 存储；db 为空时返回英文错误。
func NewDBCheckPointStore(db *database.DB) (*DBCheckPointStore, error) {
	if db == nil {
		return nil, errors.New("agentkit: nil database for the checkpoint store")
	}
	return &DBCheckPointStore{db: db}, nil
}

// Get 读取指定 ID 的 checkpoint：不存在时返回 (nil, false, nil)。返回独立副本。
func (s *DBCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error) {
	var row modelsai.AgentCheckpoint
	if err := s.db.WithContext(ctx).Where("checkpoint_id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, errors.WithStack(err)
	}
	return append([]byte(nil), row.Payload.Value()...), true, nil
}

// Set 写入指定 ID 的 checkpoint：同一 ID 重复写入按 checkpoint_id 幂等覆盖现场字节，
// payload 经 crypto 序列化器加密。
func (s *DBCheckPointStore) Set(ctx context.Context, id string, data []byte) error {
	row := &modelsai.AgentCheckpoint{CheckpointID: id, Payload: serializer.NewCrypto(data)}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "checkpoint_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
	}).Create(row).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Delete 删除指定 ID 的 checkpoint（幂等）。
func (s *DBCheckPointStore) Delete(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Where("checkpoint_id = ?", id).Delete(&modelsai.AgentCheckpoint{}).Error; err != nil {
		return errors.WithStack(err)
	}
	return nil
}
