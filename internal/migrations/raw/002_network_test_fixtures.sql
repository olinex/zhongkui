-- +goose Up

INSERT INTO packet_ethernets (
    namespace, capture, version, session_id, timestamp, truncated,
    capture_length, layer_stack, payload,
    eth_dst_mac, eth_src_mac, eth_type, eth_length
) VALUES (
    'test', 'test', 'test', unhex('a0eebc999c0b4ef8bb6d6bb9bd380a11'), now(), false,
    0,
    '[{"level":2,"proto":1,"headers":{"dst":"00:00:5e:00:53:01","src":"00:00:5e:00:53:02","type":2048}}]',
    '',
    unhex('00005E005301'), unhex('00005E005302'), 2048, 0
);

-- +goose Down
ALTER TABLE packet_ethernets DELETE WHERE namespace = 'test' AND capture = 'test' AND version = 'test';
