package network

// LinkSlots 表示链路层的通用槽位
// 上层模型不再绑定具体链路协议，链路头信息以 link_1～link_16 承载；
// 槽位含义由 LinkMeta.LinkType 决定，未使用的槽位保持空串。
//
// 数据库映射:
//   - 所有槽位均为 String 列，使用 ZSTD(3) 压缩
//   - link_1～link_6 由迁移脚本建立 bloom_filter 索引，供精确筛选使用
type LinkSlots struct {
	// Link1 链路槽位 1
	Link1 string `gorm:"column:link_1;type:String CODEC(ZSTD(3))" json:"link1"`
	// Link2 链路槽位 2
	Link2 string `gorm:"column:link_2;type:String CODEC(ZSTD(3))" json:"link2"`
	// Link3 链路槽位 3
	Link3 string `gorm:"column:link_3;type:String CODEC(ZSTD(3))" json:"link3"`
	// Link4 链路槽位 4
	Link4 string `gorm:"column:link_4;type:String CODEC(ZSTD(3))" json:"link4"`
	// Link5 链路槽位 5
	Link5 string `gorm:"column:link_5;type:String CODEC(ZSTD(3))" json:"link5"`
	// Link6 链路槽位 6
	Link6 string `gorm:"column:link_6;type:String CODEC(ZSTD(3))" json:"link6"`
	// Link7 链路槽位 7（仅输出，不建索引）
	Link7 string `gorm:"column:link_7;type:String CODEC(ZSTD(3))" json:"link7"`
	// Link8 链路槽位 8（仅输出，不建索引）
	Link8 string `gorm:"column:link_8;type:String CODEC(ZSTD(3))" json:"link8"`
	// Link9 链路槽位 9（仅输出，不建索引）
	Link9 string `gorm:"column:link_9;type:String CODEC(ZSTD(3))" json:"link9"`
	// Link10 链路槽位 10（仅输出，不建索引）
	Link10 string `gorm:"column:link_10;type:String CODEC(ZSTD(3))" json:"link10"`
	// Link11 链路槽位 11（仅输出，不建索引）
	Link11 string `gorm:"column:link_11;type:String CODEC(ZSTD(3))" json:"link11"`
	// Link12 链路槽位 12（仅输出，不建索引）
	Link12 string `gorm:"column:link_12;type:String CODEC(ZSTD(3))" json:"link12"`
	// Link13 链路槽位 13（仅输出，不建索引）
	Link13 string `gorm:"column:link_13;type:String CODEC(ZSTD(3))" json:"link13"`
	// Link14 链路槽位 14（仅输出，不建索引）
	Link14 string `gorm:"column:link_14;type:String CODEC(ZSTD(3))" json:"link14"`
	// Link15 链路槽位 15（仅输出，不建索引）
	Link15 string `gorm:"column:link_15;type:String CODEC(ZSTD(3))" json:"link15"`
	// Link16 链路槽位 16（仅输出，不建索引）
	Link16 string `gorm:"column:link_16;type:String CODEC(ZSTD(3))" json:"link16"`
}

// NewLinkSlots 按顺序填充链路槽位
// values 依次对应 link_1～link_16，超出固定数量的值被忽略，未提供的槽位保持空串。
func NewLinkSlots(values ...string) LinkSlots {
	slots := LinkSlots{}
	refs := slots.slotRefs()
	for i := 0; i < len(values) && i < len(refs); i++ {
		*refs[i] = values[i]
	}
	return slots
}

// slotRefs 返回各槽位指针，按 link_1～link_16 顺序排列
func (s *LinkSlots) slotRefs() []*string {
	return []*string{
		&s.Link1, &s.Link2, &s.Link3, &s.Link4,
		&s.Link5, &s.Link6, &s.Link7, &s.Link8,
		&s.Link9, &s.Link10, &s.Link11, &s.Link12,
		&s.Link13, &s.Link14, &s.Link15, &s.Link16,
	}
}

// LinkSlotted 表示可提供链路槽位的链路层模型
// 上层模型通过该接口读取链路槽位，无需感知具体链路协议。
type LinkSlotted interface {
	// LinkSlotValues 返回链路模型按约定映射出的通用槽位
	LinkSlotValues() LinkSlots
}
