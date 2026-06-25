package flow

import "time"

type Param struct {
	SecretKey             string        `mapstructure:"secretKey" binding:"required,min=32,max=32"`
	MaxReassemblerSize    int           `mapstructure:"maxReassemblerSize" binding:"min=1,max=65536" default:"1024"`
	MaxOutOrderPacketSize int           `mapstructure:"maxOutOrderPacketSize" binding:"min=1,max=65536" default:"256"`
	NoticeTimeout         time.Duration `mapstructure:"noticeTimeout" binding:"min=1s,max=10s" default:"5s"`
	WindowRange           time.Duration `mapstructure:"windowRange" binding:"oneof=1m 1h 1d" default:"1h"`
}
