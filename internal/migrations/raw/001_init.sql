-- +goose Up
CREATE TABLE IF NOT EXISTS packet_ethernets (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, eth_src_mac, eth_dst_mac, eth_type, timestamp)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv4 (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, ipv4_src, ipv4_dst, ipv4_ip_protocol, eth_src_mac, eth_dst_mac, eth_type, timestamp)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv4 (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, ipv4_src, ipv4_dst, ipv4_ip_protocol, eth_src_mac, eth_dst_mac, eth_type, timestamp)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv6 (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, ipv6_src, ipv6_dst, ipv6_flow_label, ipv6_last_header, eth_src_mac, eth_dst_mac, eth_type, timestamp)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv6 (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, ipv6_src, ipv6_dst, ipv6_flow_label, ipv6_last_header, eth_src_mac, eth_dst_mac, eth_type, timestamp)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv4_tcps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3)),
	tcp_src_port UInt16 CODEC(ZSTD(3)),
	tcp_dst_port UInt16 CODEC(ZSTD(3)),
	tcp_seq UInt32 CODEC(ZSTD(3)),
	tcp_ack UInt32 CODEC(ZSTD(3)),
	tcp_data_offset UInt8 CODEC(ZSTD(3)),
	tcp_bit_fin Bool,
	tcp_bit_syn Bool,
	tcp_bit_rst Bool,
	tcp_bit_psh Bool,
	tcp_bit_ack Bool,
	tcp_bit_urg Bool,
	tcp_bit_ece Bool,
	tcp_bit_cwr Bool,
	tcp_bit_ns Bool,
	tcp_window UInt16 CODEC(ZSTD(3)),
	tcp_checksum UInt16 CODEC(ZSTD(3)),
	tcp_urgent UInt16 CODEC(ZSTD(3)),
	tcp_options String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	namespace,
	capture,
	version,
	session_id,
	LEAST(tcp_src_port, tcp_dst_port),
	GREATEST(tcp_src_port, tcp_dst_port),
	LEAST(ipv4_src, ipv4_dst),
	GREATEST(ipv4_src, ipv4_dst),
	ipv4_ip_protocol,
	LEAST(eth_src_mac, eth_dst_mac),
	GREATEST(eth_src_mac, eth_dst_mac),
	eth_type,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv4_tcps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3)),
	tcp_src_port UInt16 CODEC(ZSTD(3)),
	tcp_dst_port UInt16 CODEC(ZSTD(3)),
	tcp_seq UInt32 CODEC(ZSTD(3)),
	tcp_ack UInt32 CODEC(ZSTD(3)),
	tcp_data_offset UInt8 CODEC(ZSTD(3)),
	tcp_bit_fin Bool,
	tcp_bit_syn Bool,
	tcp_bit_rst Bool,
	tcp_bit_psh Bool,
	tcp_bit_ack Bool,
	tcp_bit_urg Bool,
	tcp_bit_ece Bool,
	tcp_bit_cwr Bool,
	tcp_bit_ns Bool,
	tcp_window UInt16 CODEC(ZSTD(3)),
	tcp_checksum UInt16 CODEC(ZSTD(3)),
	tcp_urgent UInt16 CODEC(ZSTD(3)),
	tcp_options String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	namespace,
	capture,
	version,
	session_id,
	LEAST(tcp_src_port, tcp_dst_port),
	GREATEST(tcp_src_port, tcp_dst_port),
	LEAST(ipv4_src, ipv4_dst),
	GREATEST(ipv4_src, ipv4_dst),
	ipv4_ip_protocol,
	LEAST(eth_src_mac, eth_dst_mac),
	GREATEST(eth_src_mac, eth_dst_mac),
	eth_type,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv6_tcps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3)),
	tcp_src_port UInt16 CODEC(ZSTD(3)),
	tcp_dst_port UInt16 CODEC(ZSTD(3)),
	tcp_seq UInt32 CODEC(ZSTD(3)),
	tcp_ack UInt32 CODEC(ZSTD(3)),
	tcp_data_offset UInt8 CODEC(ZSTD(3)),
	tcp_bit_fin Bool,
	tcp_bit_syn Bool,
	tcp_bit_rst Bool,
	tcp_bit_psh Bool,
	tcp_bit_ack Bool,
	tcp_bit_urg Bool,
	tcp_bit_ece Bool,
	tcp_bit_cwr Bool,
	tcp_bit_ns Bool,
	tcp_window UInt16 CODEC(ZSTD(3)),
	tcp_checksum UInt16 CODEC(ZSTD(3)),
	tcp_urgent UInt16 CODEC(ZSTD(3)),
	tcp_options String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	namespace,
	capture,
	version,
	session_id,
	LEAST(tcp_src_port, tcp_dst_port),
	GREATEST(tcp_src_port, tcp_dst_port),
	LEAST(ipv6_src, ipv6_dst),
	GREATEST(ipv6_src, ipv6_dst),
	ipv6_flow_label,
	ipv6_last_header,
	LEAST(eth_src_mac, eth_dst_mac),
	GREATEST(eth_src_mac, eth_dst_mac),
	eth_type,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv6_tcps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3)),
	tcp_src_port UInt16 CODEC(ZSTD(3)),
	tcp_dst_port UInt16 CODEC(ZSTD(3)),
	tcp_seq UInt32 CODEC(ZSTD(3)),
	tcp_ack UInt32 CODEC(ZSTD(3)),
	tcp_data_offset UInt8 CODEC(ZSTD(3)),
	tcp_bit_fin Bool,
	tcp_bit_syn Bool,
	tcp_bit_rst Bool,
	tcp_bit_psh Bool,
	tcp_bit_ack Bool,
	tcp_bit_urg Bool,
	tcp_bit_ece Bool,
	tcp_bit_cwr Bool,
	tcp_bit_ns Bool,
	tcp_window UInt16 CODEC(ZSTD(3)),
	tcp_checksum UInt16 CODEC(ZSTD(3)),
	tcp_urgent UInt16 CODEC(ZSTD(3)),
	tcp_options String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	namespace,
	capture,
	version,
	session_id,
	LEAST(tcp_src_port, tcp_dst_port),
	GREATEST(tcp_src_port, tcp_dst_port),
	LEAST(ipv6_src, ipv6_dst),
	GREATEST(ipv6_src, ipv6_dst),
	ipv6_flow_label,
	ipv6_last_header,
	LEAST(eth_src_mac, eth_dst_mac),
	GREATEST(eth_src_mac, eth_dst_mac),
	eth_type,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv4_udps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3)),
	udp_src_port UInt16 CODEC(ZSTD(3)),
	udp_dst_port UInt16 CODEC(ZSTD(3)),
	udp_length UInt16 CODEC(ZSTD(3)),
	udp_checksum UInt16 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	udp_src_port,
	udp_dst_port,
	ipv4_src,
	ipv4_dst,
	ipv4_ip_protocol,
	eth_src_mac,
	eth_dst_mac,
	eth_type,
	session_id,
	namespace,
	capture,
	version,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS packet_ipv6_udps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	truncated Bool,
	capture_length UInt32 CODEC(ZSTD(3)),
	layer_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3)),
	udp_src_port UInt16 CODEC(ZSTD(3)),
	udp_dst_port UInt16 CODEC(ZSTD(3)),
	udp_length UInt16 CODEC(ZSTD(3)),
	udp_checksum UInt16 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	udp_src_port,
	udp_dst_port,
	ipv6_src,
	ipv6_dst,
	ipv6_flow_label,
	ipv6_last_header,
	eth_src_mac,
	eth_dst_mac,
	eth_type,
	session_id,
	namespace,
	capture,
	version,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv4_udps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv4_version UInt8 CODEC(ZSTD(3)),
	ipv4_ihl UInt8 CODEC(ZSTD(3)),
	ipv4_tos UInt8 CODEC(ZSTD(3)),
	ipv4_length UInt16 CODEC(ZSTD(3)),
	ipv4_id UInt16 CODEC(ZSTD(3)),
	ipv4_flags UInt8 CODEC(ZSTD(3)),
	ipv4_frag_offset UInt16 CODEC(ZSTD(3)),
	ipv4_ttl UInt8 CODEC(ZSTD(3)),
	ipv4_ip_protocol UInt8 CODEC(ZSTD(3)),
	ipv4_checksum UInt16 CODEC(ZSTD(3)),
	ipv4_src FixedString(4),
	ipv4_dst FixedString(4),
	ipv4_options String CODEC(ZSTD(3)),
	ipv4_padding String CODEC(ZSTD(3)),
	udp_src_port UInt16 CODEC(ZSTD(3)),
	udp_dst_port UInt16 CODEC(ZSTD(3)),
	udp_length UInt16 CODEC(ZSTD(3)),
	udp_checksum UInt16 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	udp_src_port,
	udp_dst_port,
	ipv4_src,
	ipv4_dst,
	ipv4_ip_protocol,
	eth_src_mac,
	eth_dst_mac,
	eth_type,
	session_id,
	namespace,
	capture,
	version,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS fused_ipv6_udps (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	timestamp DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	payload String CODEC(ZSTD(3)),
	eth_dst_mac FixedString(6),
	eth_src_mac FixedString(6),
	eth_type UInt16 CODEC(ZSTD(3)),
	eth_length UInt16 CODEC(ZSTD(3)),
	ipv6_version UInt8 CODEC(ZSTD(3)),
	ipv6_traffic_class UInt8 CODEC(ZSTD(3)),
	ipv6_flow_label UInt32 CODEC(ZSTD(3)),
	ipv6_length UInt16 CODEC(ZSTD(3)),
	ipv6_next_header UInt8 CODEC(ZSTD(3)),
	ipv6_hop_limit UInt8 CODEC(ZSTD(3)),
	ipv6_src FixedString(16),
	ipv6_dst FixedString(16),
	ipv6_hop_by_hop String CODEC(ZSTD(3)),
	ipv6_last_header UInt8 CODEC(ZSTD(3)),
	udp_src_port UInt16 CODEC(ZSTD(3)),
	udp_dst_port UInt16 CODEC(ZSTD(3)),
	udp_length UInt16 CODEC(ZSTD(3)),
	udp_checksum UInt16 CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (
	udp_src_port,
	udp_dst_port,
	ipv6_src,
	ipv6_dst,
	ipv6_flow_label,
	ipv6_last_header,
	eth_src_mac,
	eth_dst_mac,
	eth_type,
	session_id,
	namespace,
	capture,
	version,
	timestamp
)
PARTITION BY toStartOfHour(timestamp)
TTL timestamp + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS http_transports (
	namespace LowCardinality(String) CODEC(ZSTD(3)),
	capture LowCardinality(String) CODEC(ZSTD(3)),
	version LowCardinality(String) CODEC(ZSTD(3)),
	session_id UUID CODEC(ZSTD(3)),
	frame_stack String CODEC(ZSTD(3)),
	client_ip FixedString(16),
	client_port UInt16 CODEC(ZSTD(3)),
	server_ip FixedString(16),
	server_port UInt16 CODEC(ZSTD(3)),
	ip_version UInt8 CODEC(ZSTD(3)),
	http_version LowCardinality(String) CODEC(ZSTD(3)),
	url String CODEC(ZSTD(3)),
	scheme LowCardinality(String) CODEC(ZSTD(3)),
	host String CODEC(ZSTD(3)),
	path String CODEC(ZSTD(3)),
	queries String CODEC(ZSTD(3)),
	fragment String CODEC(ZSTD(3)),
	method LowCardinality(String) CODEC(ZSTD(3)),
	user_agent String CODEC(ZSTD(3)),
	referer String CODEC(ZSTD(3)),
	accept LowCardinality(String) CODEC(ZSTD(3)),
	accept_encoding LowCardinality(String) CODEC(ZSTD(3)),
	accept_language LowCardinality(String) CODEC(ZSTD(3)),
	connection LowCardinality(String) CODEC(ZSTD(3)),
	cookie String CODEC(ZSTD(3)),
	authorization String CODEC(ZSTD(3)),
	req_content_type LowCardinality(String) CODEC(ZSTD(3)),
	req_content_length UInt32 CODEC(ZSTD(3)),
	req_headers String CODEC(ZSTD(3)),
	req_body String CODEC(ZSTD(3)),
	req_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	req_duration Int64 CODEC(ZSTD(3)),
	is_req_completed Bool,
	server LowCardinality(String) CODEC(ZSTD(3)),
	set_cookie String CODEC(ZSTD(3)),
	location String CODEC(ZSTD(3)),
	cache_control LowCardinality(String) CODEC(ZSTD(3)),
	content_encoding LowCardinality(String) CODEC(ZSTD(3)),
	rsp_content_type LowCardinality(String) CODEC(ZSTD(3)),
	rsp_content_length UInt32 CODEC(ZSTD(3)),
	rsp_headers String CODEC(ZSTD(3)),
	rsp_body String CODEC(ZSTD(3)),
	rsp_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	rsp_duration Int64 CODEC(ZSTD(3)),
	is_rsp_completed Bool
) ENGINE = MergeTree()
ORDER BY (namespace, capture, version, session_id, client_port, server_port, ip_version, client_ip, server_ip, req_time)
PARTITION BY toStartOfHour(req_time)
TTL req_time + INTERVAL 168 HOUR DELETE;

CREATE TABLE IF NOT EXISTS http_api_meta (
	batch UInt32 CODEC(ZSTD(3)),
	id FixedString(16) CODEC(ZSTD(3)),
	method LowCardinality(String) CODEC(ZSTD(3)),
	api String CODEC(ZSTD(3)),
	hits UInt64 CODEC(ZSTD(3)),
	status_codes Array(UInt64) CODEC(ZSTD(3)),
	create_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	last_hit_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (api, method, status_codes)
PARTITION BY batch
TTL toDateTime(batch * 3600) + INTERVAL 24 HOUR DELETE
SETTINGS allow_suspicious_fixed_string_types = 1;

CREATE TABLE IF NOT EXISTS http_api_nodes (
	batch UInt32 CODEC(ZSTD(3)),
	id FixedString(16) CODEC(ZSTD(3)),
	parent FixedString(16) CODEC(ZSTD(3)),
	sequence UInt32 CODEC(ZSTD(3)),
	n_type UInt64 CODEC(ZSTD(3)),
	v_type UInt64 CODEC(ZSTD(3)),
	key String CODEC(ZSTD(3)),
	depth UInt64 CODEC(ZSTD(3)),
	hits UInt64 CODEC(ZSTD(3)),
	variants String CODEC(ZSTD(3)),
	create_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	last_hit_time DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	childs_popularity_n String CODEC(ZSTD(3)),
	childs_popularity_line_sum String CODEC(ZSTD(3)),
	childs_popularity_square_sum String CODEC(ZSTD(3)),
	childs_dispersion_n String CODEC(ZSTD(3)),
	childs_dispersion_line_sum String CODEC(ZSTD(3)),
	childs_dispersion_square_sum String CODEC(ZSTD(3)),
	childs_concentration_n String CODEC(ZSTD(3)),
	childs_concentration_line_sum String CODEC(ZSTD(3)),
	childs_concentration_square_sum String CODEC(ZSTD(3))
) ENGINE = MergeTree()
ORDER BY (sequence)
PARTITION BY batch
TTL toDateTime(batch * 3600) + INTERVAL 24 HOUR DELETE;

CREATE TABLE IF NOT EXISTS tag_summings (
	minute DateTime64(6, 'UTC') CODEC(DoubleDelta, ZSTD(3)),
	api String CODEC(ZSTD(3)),
	tags Array(String),
	cnt UInt32
) ENGINE = SummingMergeTree()
ORDER BY (minute, api, tags)
PARTITION BY toStartOfHour(minute)
TTL minute + INTERVAL 72 HOUR DELETE;

-- +goose Down
DROP TABLE IF EXISTS packet_ethernets;
DROP TABLE IF EXISTS packet_ipv4;
DROP TABLE IF EXISTS fused_ipv4;
DROP TABLE IF EXISTS packet_ipv6;
DROP TABLE IF EXISTS fused_ipv6;
DROP TABLE IF EXISTS packet_ipv4_tcps;
DROP TABLE IF EXISTS fused_ipv4_tcps;
DROP TABLE IF EXISTS packet_ipv6_tcps;
DROP TABLE IF EXISTS fused_ipv6_tcps;
DROP TABLE IF EXISTS packet_ipv4_udps;
DROP TABLE IF EXISTS fused_ipv4_udps;
DROP TABLE IF EXISTS packet_ipv6_udps;
DROP TABLE IF EXISTS fused_ipv6_udps;
DROP TABLE IF EXISTS http_transports;
DROP TABLE IF EXISTS http_api_meta;
DROP TABLE IF EXISTS http_api_nodes;
DROP TABLE IF EXISTS tag_summings;
