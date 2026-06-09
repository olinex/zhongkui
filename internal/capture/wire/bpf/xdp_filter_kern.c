// bpf/xdp_filter_kern.c
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

struct {
    __uint(type, BPF_MAP_TYPE_XSKMAP);
    __uint(max_entries, 64);
    __uint(key_size, sizeof(int));
    __uint(value_size, sizeof(int));
} xsk_map SEC(".maps");

// __FILTER_FUNCTION_PLACEHOLDER__
// 上面这个占位符会被自动生成的 BPF 过滤函数替换

SEC("xdp")
int xdp_filter_prog(struct xdp_md *ctx) {
    void *data_end = (void *)(long)ctx->data_end;
    void *data = (void *)(long)ctx->data;

    __u64 data_len = (__u64)data_end - (__u64)data;

    if (data_len < sizeof(struct ethhdr)) {
        return XDP_PASS;
    }

    // 调用自动生成的 BPF 过滤函数
    // cbpf_filter 返回非 0 表示符合过滤条件
    if (!cbpf_filter((const __u8 *)data, (const __u8 *)data_end)) {
        return XDP_PASS;
    }

    // 符合过滤条件, 重定向到 AF_XDP 套接字
    return bpf_redirect_map(&xsk_map, 0, 0);
}

char _license[] SEC("license") = "GPL";
