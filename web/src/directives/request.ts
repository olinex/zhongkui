/**
 * src/directives/request.ts
 *
 * 自定义指令 `v-request`：为"发起后端请求"的主流程按钮统一提供
 * "点击后禁用 + 加载中动画"能力，防止接口较慢时重复点击造成重复请求。
 *
 * 用法：
 * ```html
 * <v-btn v-request="handleDelete">删除</v-btn>
 * ```
 * 其中 `handleDelete` 必须返回 Promise（`handleSubmit` 的返回值、
 * `getResponse(...)` 包装函数、async 函数均可），指令在 handler 结束后复位加载状态。
 *
 * 实现基于纯 DOM：
 *  - 对普通按钮：直接接管 click，执行 handler。
 *  - 对 `type="submit"` 提交按钮：在 click 中 `preventDefault()` 阻止原生表单提交，
 *    再执行 handler，从而避免与 `<v-form @submit="handler">` 重复触发。
 * 加载期间：
 *  - 通过内部 `pending` 标志忽略后续点击（防重复）。
 *  - 为根元素添加 `zk-request-loading` 类（配合全局 SCSS 展示居中旋转 spinner）、
 *    原生 `disabled` 属性与 `aria-busy`，提升可访问性。
 * 任何情况下（成功/抛错）都在 `finally` 中复位，避免 loading 卡死。
 *
 * 指令不吞 handler 的错误，也不干预各页面自身的错误提示逻辑。
 */
import type { Directive, DirectiveBinding } from "vue";

/** 指令绑定值：必须是返回 Promise 的异步处理函数 */
export type RequestHandler = (event: Event) => Promise<void>;

const LOADING_CLASS = "zk-request-loading";
const atrHolder = "_zkRequestLoading";

/** 向按钮注入居中的加载中 spinner 覆盖层（若不存在） */
function ensureSpinner(el: HTMLElement): void {
  if (el.querySelector(`[${atrHolder}]`)) {
    return;
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const hasVuetifyContent = Boolean((el as any).querySelector?.(".v-btn__content"));
  // icon 按钮（无 .v-btn__content，如 mdi 图标）直接使用背景覆盖层；带内容按钮同理居中覆盖
  const layer = document.createElement("span");
  layer.setAttribute(atrHolder, "");
  layer.className = "zk-request-spinner";
  const icon = document.createElement("i");
  icon.className = "mdi mdi-loading mdi-spin";
  layer.appendChild(icon);
  if (hasVuetifyContent) {
    // 让 v-btn 内容容器居中承载 spinner，保证文字/图标按钮都居中
    const content = el.querySelector(".v-btn__content") as HTMLElement | null;
    if (content) {
      // 临时闭包内追加到 content 顶部，由全局 SCSS 定位
      content.prepend(layer);
      return;
    }
  }
  el.appendChild(layer);
}

/** 移除注入的 spinner 覆盖层 */
function removeSpinner(el: HTMLElement): void {
  el.querySelectorAll(`[${atrHolder}]`).forEach((node) => node.remove());
}

/** 进入加载态：禁用 + 展示 spinner */
function enterLoading(el: HTMLElement): void {
  el.classList.add(LOADING_CLASS);
  el.setAttribute("disabled", "");
  el.setAttribute("aria-busy", "true");
  ensureSpinner(el);
}

/** 退出加载态：复位禁用与 spinner */
function exitLoading(el: HTMLElement): void {
  el.classList.remove(LOADING_CLASS);
  el.removeAttribute("disabled");
  el.setAttribute("aria-busy", "false");
  removeSpinner(el);
}

/** 判断元素是否为提交按钮（原生 type=submit 或位于表单内且未指定其他 type） */
function isSubmitButton(el: HTMLElement): boolean {
  const tag = el.tagName.toLowerCase();
  if (tag !== "button") {
    return false;
  }
  const type = el.getAttribute("type");
  if (type === "submit") {
    return true;
  }
  // v-btn 默认不带 type；若位于表单内则浏览器按 submit 处理点击的默认行为
  if (type === null && el.closest("form")) {
    return true;
  }
  return false;
}

function onRequestClick(
  event: Event,
  el: HTMLElement,
  binding: DirectiveBinding<RequestHandler>,
): void {
  // 忽略非主键点击，避免辅助技术重复触发
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  if ((event as any).button !== undefined && (event as any).button !== 0) {
    return;
  }
  const holder = el as HTMLElement & { _zkPending?: boolean };
  // 防重复：pending 期间忽略后续点击
  if (holder._zkPending) {
    event.preventDefault();
    event.stopImmediatePropagation();
    return;
  }
  holder._zkPending = true;
  // 提交按钮需阻止原生表单提交，避免与 v-form 的 @submit 双重触发
  if (isSubmitButton(el)) {
    event.preventDefault();
  }
  enterLoading(el);
  const handler = binding.value;
  // handler 的错误由其自身处理（如 getResponse 的 failed/notify），
  // 这里仅用 catch 吞掉以阻止 "unhandled rejection"，并在 finally 复位加载态。
  Promise.resolve()
    .then(() => handler(event))
    .catch(() => undefined)
    .finally(() => {
      exitLoading(el);
      holder._zkPending = false;
    });
}

/**
 * v-request 自定义指令定义。
 * 挂载时接管按钮点击，卸载时移除监听，避免内存泄漏。
 */
export const requestDirective: Directive<HTMLElement, RequestHandler> = {
  beforeMount(el, binding) {
    el.addEventListener("click", (event) => onRequestClick(event, el, binding));
  },
  unmounted(el) {
    removeSpinner(el);
    el.classList.remove(LOADING_CLASS);
    el.removeAttribute("aria-busy");
  },
};
