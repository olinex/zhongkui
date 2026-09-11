// 接口元信息（OPTIONS）缓存 store。
// 考虑到元信息自省接口几乎不可能更新，前端仅在首次打开页面或强制刷新时才重新请求，
// 后续直接使用缓存值；枚举字段可选项统一从缓存 meta 的 FieldMeta.enumOptions 读取。
//
// 用法：
//   const metaStore = useMetaStore();
//   // 首次打开页面时确保某端点元信息已加载（传入该端点的 OPTIONS fetcher）
//   await metaStore.ensure("/notification/channel/list", notificationChannelListOptions);
//   // 取某方法某一槽位结构体中某字段的可选枚举值（无缓存或字段非枚举时返回 undefined）
//   const opts = metaStore.fieldOptions("/notification/channel/list", "POST", "bodyInput", "type");
import { defineStore } from "pinia";

// Utilities
import { getResponse } from "@/apis/base";

// Types
import type { ApiMeta, EnumOption, MetaOut, StructSlot } from "@/components/@types/meta";
import type { AxiosPromise } from "axios";

// ensures 缓存无界增长的风险可控：端点集合有限（仅 OPTIONS 注册过的列表/详情接口）。
export const useMetaStore = defineStore("meta", {
  state: () => ({
    // cached 按端点 OPTIONS 路径缓存其元信息；路径即各页面 optionsRequest 对应的 URL。
    cached: {} as Record<string, MetaOut>,
  }),
  actions: {
    // ensure 首次访问才请求并写缓存；失败保持未缓存态以允许重试，不抛错中断调用方。
    async ensure(path: string, fetcher: () => AxiosPromise): Promise<void> {
      if (this.cached[path]) {
        return;
      }
      await getResponse<MetaOut>(fetcher(), async (rsp) => {
        this.cached[path] = rsp.data;
      });
    },
    // invalidate 清除指定端点的缓存（强制刷新 meta 使用）。
    invalidate(path: string): void {
      delete this.cached[path];
    },
    // clear 清空全部缓存。
    clear(): void {
      this.cached = {};
    },
    // apiMeta 取指定端点的全部方法元信息；未缓存时返回空对象。
    apiMeta(path: string): ApiMeta {
      return this.cached[path]?.meta ?? {};
    },
    // structMeta 取指定端点某 HTTP 方法某槽位下的结构体元信息；未命中时返回 undefined。
    structMeta(
      path: string,
      method: string,
      slot: StructSlot,
    ): { name: string; fields: { name: string; enumOptions?: EnumOption[] }[] } | undefined {
      return this.apiMeta(path)[method]?.[slot] ?? undefined;
    },
    // fieldOptions 取指定结构体某字段的可选枚举值；缓存未命中或字段非枚举时返回 undefined。
    fieldOptions(
      path: string,
      method: string,
      slot: StructSlot,
      field: string,
    ): EnumOption[] | undefined {
      const struct = this.structMeta(path, method, slot);
      if (!struct) {
        return undefined;
      }
      return struct.fields.find((f) => f.name === field)?.enumOptions;
    },
  },
});
