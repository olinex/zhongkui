package network

// 本文件集中声明模型对应的数据库表名，供 GORM 与文档生成统一读取。

// TableName 返回 HttpTransport 对应的数据库表名。
func (m *HttpTransport) TableName() string {
	return "http_transports"
}

// TableName 返回 NetworkStack 对应的数据库表名。
func (m *NetworkStack) TableName() string {
	return "network_stacks"
}

// TableName 返回 PacketEthernet 对应的数据库表名。
func (m *PacketEthernet) TableName() string {
	return "packet_ethernets"
}

// TableName 返回 PacketLinuxSLL 对应的数据库表名。
func (m *PacketLinuxSLL) TableName() string {
	return "packet_linux_slls"
}

// TableName 返回 PacketLinuxSLL2 对应的数据库表名。
func (m *PacketLinuxSLL2) TableName() string {
	return "packet_linux_sll2s"
}

// TableName 返回 PacketPPP 对应的数据库表名。
func (m *PacketPPP) TableName() string {
	return "packet_ppps"
}

// TableName 返回 PacketDot11 对应的数据库表名。
func (m *PacketDot11) TableName() string {
	return "packet_dot11s"
}

// TableName 返回 PacketIPv4 对应的数据库表名。
func (m *PacketIPv4) TableName() string {
	return "packet_ipv4"
}

// TableName 返回 PacketIPv4TCP 对应的数据库表名。
func (m *PacketIPv4TCP) TableName() string {
	return "packet_ipv4_tcps"
}

// TableName 返回 PacketIPv4UDP 对应的数据库表名。
func (m *PacketIPv4UDP) TableName() string {
	return "packet_ipv4_udps"
}

// TableName 返回 PacketIPv6 对应的数据库表名。
func (m *PacketIPv6) TableName() string {
	return "packet_ipv6"
}

// TableName 返回 PacketIPv6TCP 对应的数据库表名。
func (m *PacketIPv6TCP) TableName() string {
	return "packet_ipv6_tcps"
}

// TableName 返回 PacketIPv6UDP 对应的数据库表名。
func (m *PacketIPv6UDP) TableName() string {
	return "packet_ipv6_udps"
}
