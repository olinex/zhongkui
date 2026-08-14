package migrate

import (
	"embed"
	"time"
)

type Config struct {
	Name          string
	Driver        string
	DSN           string
	MigrationsFS  embed.FS
	MigrationsDir string
	Timeout       time.Duration
	Verbose       bool
	KeepTemp      bool
	AllowMissing  bool
}

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

func (c *Config) WithMigrations(fs embed.FS, dir string) *Config {
	c.MigrationsFS = fs
	c.MigrationsDir = dir
	return c
}

func (c *Config) WithTimeout(timeout time.Duration) *Config {
	c.Timeout = timeout
	return c
}

func (c *Config) WithVerbose(verbose bool) *Config {
	c.Verbose = verbose
	return c
}

func (c *Config) WithKeepTemp(keep bool) *Config {
	c.KeepTemp = keep
	return c
}
