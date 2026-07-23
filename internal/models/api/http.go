package api

import (
	"time"

	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

const (
	API_NODE_TTL_HOURS = 24
)

type HttpApiMeta struct {
	Batch       uint32             `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	ID          serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	Method      string             `gorm:"type:LowCardinality(String) CODEC(ZSTD(3))"`
	Api         string             `gorm:"type:String CODEC(ZSTD(3))"`
	Hits        int64              `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	StatusCodes []uint64           `gorm:"type:Array(UInt64) CODEC(ZSTD(3))"`
	CreateTime  time.Time          `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	LastHitTime time.Time          `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
}

type HttpApiNode struct {
	Batch                        uint32             `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	ID                           serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	Parent                       serializer.HexUUID `gorm:"type:FixedString(16) CODEC(ZSTD(3))"`
	Sequence                     uint32             `gorm:"type:UInt32 CODEC(ZSTD(3))"`
	NType                        int64              `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	VType                        int64              `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	Key                          string             `gorm:"type:String CODEC(ZSTD(3))"`
	Depth                        int64              `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	Hits                         int64              `gorm:"type:UInt64 CODEC(ZSTD(3))"`
	Variants                     string             `gorm:"type:String CODEC(ZSTD(3))"`
	CreateTime                   time.Time          `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	LastHitTime                  time.Time          `gorm:"type:DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))"`
	ChildsPopularityN            string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsPopularityLineSum      string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsPopularitySquareSum    string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsDispersionN            string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsDispersionLineSum      string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsDispersionSquareSum    string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsConcentrationN         string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsConcentrationLineSum   string             `gorm:"type:String CODEC(ZSTD(3))"`
	ChildsConcentrationSquareSum string             `gorm:"type:String CODEC(ZSTD(3))"`
}
