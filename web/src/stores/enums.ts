// 统一枚举注册表缓存 store。
// 后端枚举值空间在运行生命周期内固定（仅后端进程重启才可能更新），故按（类, 语言）永久缓存、
// 在途请求按（类, 语言）去重、全量预取按语言去重；语言切换后惰性获取对应语言条目。
//
// 用法：
//   const enumsStore = useEnumsStore();
//   await enumsStore.ensure("httpMethod", "zh-Hans"); // 单类按语言确保已缓存
//   await enumsStore.prefetch("zh-Hans");             // 全量按语言预取
//   enumsStore.labelOf("httpMethod", "GET", "zh-Hans"); // 按 value/key 解析 label
import { defineStore } from "pinia";

// Utilities
import { getResponse } from "@/apis/base";
import { enumGet, enumList } from "@/apis/enums";

// Types
import type { EnumEntryOut, EnumTypeOut } from "@/apis/enums";

export const useEnumsStore = defineStore("enums", {
  state: () => ({
    // cached 按 `${type}:${lang}` 缓存各枚举类条目（label 已按 lang 翻译）
    cached: {} as Record<string, EnumTypeOut>,
    // inflight 单类在途请求去重（键为 `${type}:${lang}`）
    inflight: {} as Record<string, Promise<void>>,
    // listInflight 全量预取在途去重（键为 lang）
    listInflight: {} as Record<string, Promise<void>>,
    // listLoaded 某语言全量预取是否成功（失败保持 false 以便重试）
    listLoaded: {} as Record<string, boolean>,
  }),
  actions: {
    // cacheKeyOf 组装缓存键：`${type}:${lang}`
    cacheKeyOf(type: string, lang: string): string {
      return `${type}:${lang}`;
    },
    // ensure 确保某枚举类按语言已缓存：缓存命中直接返回，在途请求复用同一 Promise；
    // 成功写缓存，失败不写（允许重试），成功/失败后均清理在途标记（getResponse 不抛错，
    // 返回的 Promise 对调用方恒 resolve）。
    async ensure(type: string, lang: string): Promise<void> {
      const key = this.cacheKeyOf(type, lang);
      if (this.cached[key]) {
        return;
      }
      if (key in this.inflight) {
        return this.inflight[key];
      }
      const p = getResponse<EnumTypeOut>(enumGet({ type, lang }), async (rsp) => {
        this.cached[key] = rsp.data;
      }).then(
        () => {
          delete this.inflight[key];
        },
        () => {
          delete this.inflight[key];
        },
      );
      this.inflight[key] = p;
      return p;
    },
    // prefetch 全量预取某语言全部枚举类条目并逐个写入缓存；已预取成功则跳过，
    // 在途请求复用同一 Promise，失败不置 listLoaded（允许重试），成功/失败后清理在途标记。
    async prefetch(lang: string): Promise<void> {
      if (this.listLoaded[lang]) {
        return;
      }
      if (lang in this.listInflight) {
        return this.listInflight[lang];
      }
      const p = getResponse<{ types: EnumTypeOut[] }>(enumList({ lang }), async (rsp) => {
        for (const t of rsp.data.types ?? []) {
          this.cached[this.cacheKeyOf(t.type, lang)] = t;
        }
        this.listLoaded[lang] = true;
      }).then(
        () => {
          delete this.listInflight[lang];
        },
        () => {
          delete this.listInflight[lang];
        },
      );
      this.listInflight[lang] = p;
      return p;
    },
    // entries 返回某枚举类按语言的缓存条目；未缓存返回 undefined
    entries(type: string, lang: string): EnumEntryOut[] | undefined {
      return this.cached[this.cacheKeyOf(type, lang)]?.entries;
    },
    // labelOf 按 value 优先、key 次之匹配条目并返回 label；均未命中回退原始值字符串
    labelOf(type: string, value: string | number | null | undefined, lang: string): string {
      const str = String(value ?? "");
      const entries = this.cached[this.cacheKeyOf(type, lang)]?.entries;
      const entry = entries?.find((e) => e.value === str) ?? entries?.find((e) => e.key === str);
      return entry ? entry.label : str;
    },
    // clear 清空全部缓存与预取状态
    clear(): void {
      this.cached = {};
      this.listLoaded = {};
    },
  },
});
