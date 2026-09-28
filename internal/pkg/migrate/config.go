// Package migrate（本文件）定义数据库迁移的配置项及其链式配置方法。
package migrate

import (
	"embed"
	"time"
)

// Config 描述数据库迁移所需的连接、迁移脚本与执行行为配置。
type Config struct {
	// Name 数据库名称（core / raw），用于 CLI 打印时标识目标数据库。
	Name string
	// Driver 数据库驱动类型，如 postgres 或 clickhouse。
	Driver string
	// DSN 数据库连接串。
	DSN string
	// MigrationsFS 内嵌的迁移脚本文件系统。
	MigrationsFS embed.FS
	// MigrationsDir 迁移脚本在文件系统中的目录。
	MigrationsDir string
	// Timeout 迁移操作超时时间。
	Timeout time.Duration
	// Verbose 是否输出详细日志。
	Verbose bool
	// KeepTemp 是否保留临时数据库。
	KeepTemp bool
	// AllowMissing 是否允许缺少迁移文件。
	AllowMissing bool
}

// NewConfig 创建迁移配置，设置驱动、连接串与默认 5 分钟超时。
func NewConfig(driver, dsn string) *Config {
	return &Config{
		Driver:  driver,
		DSN:     dsn,
		Timeout: 5 * time.Minute,
	}
}

// WithName 设置数据库名称（core / raw），用于 CLI 打印时标识目标数据库
func (c *Config) WithName(name string) *Config {
	c.Name = name
	return c
}

// WithMigrations 设置内嵌的迁移脚本文件系统及其目录。
func (c *Config) WithMigrations(fs embed.FS, dir string) *Config {
	c.MigrationsFS = fs
	c.MigrationsDir = dir
	return c
}

// WithTimeout 设置迁移操作超时时间。
func (c *Config) WithTimeout(timeout time.Duration) *Config {
	c.Timeout = timeout
	return c
}

// WithVerbose 设置是否输出详细日志。
func (c *Config) WithVerbose(verbose bool) *Config {
	c.Verbose = verbose
	return c
}

// WithKeepTemp 设置是否保留临时数据库。
func (c *Config) WithKeepTemp(keep bool) *Config {
	c.KeepTemp = keep
	return c
}
