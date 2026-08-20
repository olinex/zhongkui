// The vee-validate field state passed by `defineField` is not fully typed by the library.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const vuetifyCfg = (state: any) => ({
  props: {
    label: state.label,
    "error-messages": state.errors[0] || "",
  },
});
