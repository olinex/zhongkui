// vuetifyCfg 由 vee-validate 字段状态生成 Vuetify 字段的展示配置（标签与错误信息）。
// defineField 传入的字段状态未被该库完整类型化，故此处以 any 接收。
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const vuetifyCfg = (state: any) => ({
  props: {
    label: state.label,
    "error-messages": state.errors[0] || "",
  },
});
