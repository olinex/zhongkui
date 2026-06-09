export const vuetifyCfg = (state: any) => ({
  props: {
    "label": state.label,
    "error-messages": state.errors[0] || "",
  },
});
