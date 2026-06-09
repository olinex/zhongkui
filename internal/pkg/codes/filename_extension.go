package codes

import (
	"encoding/json"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// extCodes 是文件扩展名代码的全局单例实例，通过 sync.Once 保证仅初始化一次。
	extCodes *FilenameExtensionCodes
	// extOnce 用于控制 extCodes 的延迟初始化，确保并发安全。
	extOnce sync.Once
)

// FilenameExtensionCodes 提供文件扩展名到分类标签的查询能力。
//
// 数据来源于 dyne/file-extension-list 项目，通过嵌入的 JSON 文件加载。
// Extensions 是一个从文件扩展名（小写，如 "jpg"、"mp4"）到分类标签列表
// （如 ["image"]、["video"]）的映射。部分扩展名可能对应多个分类标签，
// 例如 "html" 同时属于 ["code", "web"] 两个分类。
type FilenameExtensionCodes struct {
	Extensions map[string][]string `json:"extensions"`
}

// GetFilenameExtensionCodes 获取文件扩展名代码查询器的全局单例。
//
// 首次调用时会从嵌入的 JSON 数据文件中加载并解析所有扩展名数据。
// 后续调用直接返回已缓存的实例，不再重复加载。
// 如果加载过程中发生错误，将触发 panic，因为这是不可恢复的初始化错误。
func GetFilenameExtensionCodes() *FilenameExtensionCodes {
	extOnce.Do(func() {
		codes, err := newFilenameExtensionCodes()
		if err != nil {
			panic(err)
		}
		extCodes = codes
	})
	return extCodes
}

// newFilenameExtensionCodes 从嵌入的集合数据文件中创建并初始化 FilenameExtensionCodes 实例。
//
// 该函数读取由 FILENAME_EXTENSION 常量指定的 JSON 文件，
// 将其反序列化为 FilenameExtensionCodes 结构体并返回。
func newFilenameExtensionCodes() (*FilenameExtensionCodes, error) {
	file, err := embeddedCollections.Open(FILENAME_EXTENSION)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	raw := make([]byte, info.Size())
	_, err = file.Read(raw)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var data FilenameExtensionCodes
	err = json.Unmarshal(raw, &data)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &data, nil
}

// ConvertFromName 根据文件扩展名查询其所属的分类标签列表。
//
// 参数 name 为文件扩展名（小写形式，如 "jpg"、"mp4"）。
// 返回该扩展名对应的分类标签切片（如 ["image"]、["code", "web"]）。
// 如果指定的扩展名不存在，返回 nil。
//
// 注意：该方法区分大小写，仅支持小写扩展名查询。
func (c *FilenameExtensionCodes) ConvertFromName(name string) []string {
	return c.Extensions[name]
}
