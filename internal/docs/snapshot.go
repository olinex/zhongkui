package docs

// 本文件定义 RAG 知识快照：发布二进制不携带源码树与 Go 工具链，zki sync docs / sync schema
// 无法在运行期渲染包注释与数据表知识，因此由 docsnap 在源码树内烘焙快照并编译期嵌入二进制。

import (
	// 标准库
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ragSnapshotVersion 是快照结构版本：字段语义变化时递增，运行期据此拒绝不兼容快照。
const ragSnapshotVersion = 1

// ragSnapshotRelPath 是快照文件相对项目根的路径，生成器与守卫测试共用。
const ragSnapshotRelPath = "internal/docs/ragdata/snapshot.json"

// embeddedRAGSnapshot 是构建期烘焙的 RAG 知识快照，随二进制发布。
//
//go:embed ragdata/snapshot.json
var embeddedRAGSnapshot []byte

// RAGDocument 是一篇可入 RAG 的知识文档。
type RAGDocument struct {
	// Key 稳定去重键。
	Key string `json:"key"`
	// Title 文档标题，作为入库与同步判定的稳定键。
	Title string `json:"title"`
	// Content Markdown 正文。
	Content string `json:"content"`
}

// RAGSnapshot 是编译期嵌入的 RAG 知识快照：两个分区与 sync docs / sync schema 一一对应。
type RAGSnapshot struct {
	// Version 快照结构版本。
	Version int `json:"version"`
	// Narratives 叙述页分区：由包注释渲染而来。
	Narratives []RAGDocument `json:"narratives"`
	// Schemas 数据表分区：每张已迁移表一篇。
	Schemas []RAGDocument `json:"schemas"`
}

// BuildRAGSnapshot 在源码树内渲染 RAG 知识快照，供生成器烘焙到快照文件。
// projectRoot 为项目根目录；该操作需要源码与 Go 工具链，只能在构建期执行。
func BuildRAGSnapshot(projectRoot string) (*RAGSnapshot, error) {
	narratives, err := GeneratedNarrativeDocuments(projectRoot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	schemas, err := SchemaKnowledgeDocuments(projectRoot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	snapshot := &RAGSnapshot{
		Version:    ragSnapshotVersion,
		Narratives: make([]RAGDocument, 0, len(narratives)),
		Schemas:    make([]RAGDocument, 0, len(schemas)),
	}
	for _, n := range narratives {
		snapshot.Narratives = append(snapshot.Narratives, RAGDocument{Key: n.Key, Title: n.Title, Content: n.Content})
	}
	for _, s := range schemas {
		snapshot.Schemas = append(snapshot.Schemas, RAGDocument{Key: s.Key, Title: s.Title, Content: s.Content})
	}
	return snapshot, nil
}

// EmbeddedRAGSnapshot 返回编译期嵌入的 RAG 知识快照。
// 返回值只读，调用方不得修改其中的切片与字符串。
func EmbeddedRAGSnapshot() (*RAGSnapshot, error) {
	var snapshot RAGSnapshot
	if err := json.Unmarshal(embeddedRAGSnapshot, &snapshot); err != nil {
		return nil, errors.WithStack(err)
	}
	if snapshot.Version != ragSnapshotVersion {
		return nil, errors.Newf("docs: embedded RAG snapshot version %d is incompatible with %d", snapshot.Version, ragSnapshotVersion)
	}
	return &snapshot, nil
}

// MarshalRAGSnapshot 以稳定格式序列化快照：生成器写盘与守卫测试比对共用本函数，保证零漂移。
func MarshalRAGSnapshot(snapshot *RAGSnapshot) ([]byte, error) {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return append(data, '\n'), nil
}

// RAGSnapshotPath 返回快照文件在源码树中的路径。
func RAGSnapshotPath(projectRoot string) string {
	return filepath.Join(projectRoot, filepath.FromSlash(ragSnapshotRelPath))
}

// WriteRAGSnapshotFile 在源码树内渲染快照并写入快照文件，供生成器调用。
func WriteRAGSnapshotFile(projectRoot string) error {
	snapshot, err := BuildRAGSnapshot(projectRoot)
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := MarshalRAGSnapshot(snapshot)
	if err != nil {
		return errors.WithStack(err)
	}
	path := RAGSnapshotPath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.WithStack(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
