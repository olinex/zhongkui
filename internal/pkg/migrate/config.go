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
