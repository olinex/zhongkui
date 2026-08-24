/**
 * Slash-trigger helpers shared by the Rego editor and the tag expression
 * editor: "/" opens an in-editor field/tag picker, a second "/" while the
 * picker is open acts as a close signal (the slash itself is swallowed),
 * and a "/" after "/" with the picker closed is typed as a literal.
 */

import type { Completion } from "@codemirror/autocomplete";
import { closeCompletion } from "@codemirror/autocomplete";
import type { EditorState } from "@codemirror/state";
import type { EditorView, KeyBinding } from "@codemirror/view";

// 斜杠前一字符为这些时视为非触发位置（标识符 / 字符串尾 / 括号闭合处）
const SLASH_PREV_BLOCK_RE = /[a-zA-Z0-9_)\]"'`]/;

/**
 * Finds the position of the last eligible "/" trigger in the text before the
 * cursor, or null. 从行尾向前扫描：触发斜杠前一字符不能是 "/"（即非 "//"），
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

/** Whether the completion dropdown is currently open (.cm-tooltip-autocomplete in DOM). */
export function isCompletionOpen(view: EditorView): boolean {
  return view.dom.querySelector(".cm-tooltip-autocomplete") !== null;
}

/**
 * "/" key binding implementing the slash input rules:
 * - prev char is "/" and the dropdown is open: the slash is only a close
 *   signal, swallowed without writing to the document;
 * - otherwise fall through to normal input (opening the picker is handled
 *   by the completion source; a "/" after "/" with the picker closed is
 *   typed literally).
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
 * Wraps completion options so that picking one replaces the range starting at
 * the "/" trigger (from 已包含斜杠，无需再前移):
 * - function applies get `from` passed through (已覆盖斜杠);
 * - label-only applies become functions inserting the label from `from`,
 *   with the cursor placed after the inserted text (mirrors CM's default).
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
