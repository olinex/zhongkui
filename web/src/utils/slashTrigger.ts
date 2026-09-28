/**
 * Rego 编辑器与标签表达式编辑器共用的斜杠触发辅助函数：
 * "/" 打开编辑器内的字段/标签选择器，选择器打开时再次输入 "/" 作为关闭信号
 * （该斜杠本身被吞掉），选择器关闭时输入 "/" 后面的 "/" 按字面量处理。
 */

import type { Completion } from "@codemirror/autocomplete";
import { closeCompletion } from "@codemirror/autocomplete";
import type { EditorState } from "@codemirror/state";
import type { EditorView, KeyBinding } from "@codemirror/view";

// 斜杠前一字符为这些时视为非触发位置（标识符 / 字符串尾 / 括号闭合处）
const SLASH_PREV_BLOCK_RE = /[a-zA-Z0-9_)\]"'`]/;

/**
 * 在光标之前的文本中查找最后一个可触发的 "/" 位置，找不到返回 null。
 * 从行尾向前扫描：触发斜杠前一字符不能是 "/"（即非 "//"），
 * 也不能是词字符等封闭字符。可选的 `inLiteral` 用于排除字符串/注释等
 * 字面量内部的斜杠（由各编辑器的语法树判定）。
 */
export function findSlashPos(
  state: EditorState,
  pos: number,
  inLiteral?: (state: EditorState, pos: number) => boolean,
): number | null {
  const line = state.doc.lineAt(pos);
  const textBefore = line.text.slice(0, pos - line.from);
  let slashIdx = textBefore.lastIndexOf("/");
  while (slashIdx >= 0) {
    const prev = slashIdx > 0 ? textBefore[slashIdx - 1] : "";
    if (prev !== "/" && !SLASH_PREV_BLOCK_RE.test(prev)) {
      break;
    }
    slashIdx = textBefore.lastIndexOf("/", slashIdx - 1);
  }
  if (slashIdx < 0 || inLiteral?.(state, pos)) {
    return null;
  }
  return line.from + slashIdx;
}

/** 补全下拉框当前是否打开（DOM 中存在 .cm-tooltip-autocomplete）。 */
export function isCompletionOpen(view: EditorView): boolean {
  return view.dom.querySelector(".cm-tooltip-autocomplete") !== null;
}

/**
 * 实现斜杠输入规则的 "/" 键绑定：
 * - 前一字符为 "/" 且下拉框已打开：该斜杠仅作为关闭信号，被吞掉且不写入文档；
 * - 否则交由常规输入处理（打开选择器由补全源负责；选择器关闭时，
 *   "/" 之后的 "/" 按字面量输入）。
 */
export const slashKeyBinding: KeyBinding = {
  key: "/",
  run: (view) => {
    const head = view.state.selection.main.head;
    const prev = head > 0 ? view.state.doc.sliceString(head - 1, head) : "";
    if (prev === "/" && isCompletionOpen(view)) {
      closeCompletion(view);
      return true;
    }
    return false;
  },
};

/**
 * 包装补全选项，使选中项替换从 "/" 触发处开始的区间（已包含斜杠，无需再前移）：
 * - 函数形式的 apply 直接透传 `from`（已覆盖斜杠）；
 * - 仅含 label 的 apply 变为从 `from` 插入 label 的函数，
 *   光标置于插入文本之后（与 CM 默认行为一致）。
 */
export function withSlashApply(options: Completion[]): Completion[] {
  return options.map((c) => {
    const apply = c.apply;
    if (typeof apply === "function") {
      return {
        ...c,
        apply: (v: EditorView, comp: Completion, from: number, to: number) =>
          apply(v, comp, from, to),
      };
    }
    return {
      ...c,
      apply: (v: EditorView, comp: Completion, from: number, to: number) => {
        v.dispatch({
          changes: { from, to, insert: comp.label },
          selection: { anchor: from + comp.label.length },
          userEvent: "input.complete",
        });
      },
    };
  });
}
