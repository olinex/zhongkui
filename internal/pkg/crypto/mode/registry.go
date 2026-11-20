// 本文件实现加密模式的注册表：标签中的 mode 字符串经注册表解析为具体实现，
// 新增加密模式只需实现 Mode 接口并调用 Register，无需修改标签解析链路。
package mode

import (
	"sync"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// Constructor 加密模式构造函数：依据派生密钥与可选附加认证数据（AAD）创建模式实例。
type Constructor func(key, additionalData []byte) (Mode, error)

var (
	// registryMu 保护 registry 的并发读写；注册通常发生在 init 阶段，读取发生在每次解密。
	registryMu sync.RWMutex
	// registry 已注册的加密模式，键为 crypto 标签中 mode 属性的取值。
	registry = make(map[string]Constructor)
)

// Register 注册加密模式构造函数，name 为 crypto 标签中 mode 属性的取值（如 "aes-gcm"）。
// 重复注册、空名称或空构造函数属于编程错误，直接 panic 以在注册期暴露。
func Register(name string, ctor Constructor) {
	if name == "" {
		panic("mode: cannot register an encryption mode with an empty name")
	}
	if ctor == nil {
		panic("mode: cannot register a nil constructor for " + name)
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic("mode: encryption mode " + name + " already registered")
	}
	registry[name] = ctor
}

// New 按 name 查找并构造加密模式实例；name 未注册时返回错误。
func New(name string, key, additionalData []byte) (Mode, error) {
	registryMu.RLock()
	ctor, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, errors.Newf("unknown crypto mode: %s", name)
	}
	return ctor(key, additionalData)
}
