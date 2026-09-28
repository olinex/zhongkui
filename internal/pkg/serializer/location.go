package serializer

import (
	"time"
)

// Location 表示 IANA 时区名称的字符串类型。
type Location string

// ToSTD 根据时区名称加载并返回对应的 time.Location。
func (l Location) ToSTD() (*time.Location, error) {
	return time.LoadLocation(string(l))
}
