// Third-party
import { defaultHighlightStyle, HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { oneDarkHighlightStyle } from "@codemirror/theme-one-dark";
import { tags } from "@lezer/highlight";

// Types
import type { Extension } from "@codemirror/state";

// Plugins / Utilities
import { DARK_THEME } from "@/stores/local";

// 项目统一的编辑器语法高亮语义配色方案：同一语义在任意编辑器（go 模板 / 标签表达式 / rego
// 策略）内使用同一种颜色，且相邻语义的色相明确区隔，避免混淆。
// 核心色相约定（浅色 / 深色各一套）：
//   - 关键字 range/if/else/end/AND/OR/NOT：红（#cf222e）/ 粉紫（#c678dd）
//   - 变量与字段引用 .Field、$var：蓝（#0550ae）/ 浅蓝（#61afef）  ← 与关键字明显区分
//   - 字符串：绿（#0a7d22）/ 绿（#98c379）
//   - 数字：橙棕（#953800）/ 橙（#d19a66）
//   - 函数调用：紫（#6f42c1）/ 黄（#e5c07b）
//   - 运算符/括号：近黑 / 灰色
//   - 边界标记 {{ }}（meta）：灰蓝（#57606a）/ 灰（#5c6370）——区别于普通黑色字面量，
//     但不与关键字/变量抢色
//   - 注释：灰，斜体
const lightHighlight = HighlightStyle.define([
  { tag: tags.comment, color: "#6a737d", fontStyle: "italic" },
  { tag: tags.keyword, color: "#cf222e" },
  { tag: tags.bool, color: "#cf222e" },
  { tag: tags.string, color: "#0a7d22" },
  { tag: tags.number, color: "#953800" },
  { tag: tags.variableName, color: "#0550ae" },
  { tag: tags.propertyName, color: "#0550ae" },
  { tag: tags.atom, color: "#0550ae" },
  { tag: tags.function(tags.variableName), color: "#6f42c1" },
  { tag: tags.operator, color: "#1f2328" },
  { tag: tags.bracket, color: "#6f42c1" },
  { tag: tags.meta, color: "#57606a" },
]);

const darkHighlight = HighlightStyle.define([
  { tag: tags.comment, color: "#7f848e", fontStyle: "italic" },
  { tag: tags.keyword, color: "#c678dd" },
  { tag: tags.bool, color: "#c678dd" },
  { tag: tags.string, color: "#98c379" },
  { tag: tags.number, color: "#d19a66" },
  { tag: tags.variableName, color: "#61afef" },
  { tag: tags.propertyName, color: "#61afef" },
  { tag: tags.atom, color: "#61afef" },
  { tag: tags.function(tags.variableName), color: "#e5c07b" },
  { tag: tags.operator, color: "#abb2bf" },
  { tag: tags.bracket, color: "#56b6c2" },
  { tag: tags.meta, color: "#5c6370" },
]);

// highlightFor 生成编辑器语法高亮扩展：以基础主题为底（补齐未定制语义），叠加本项目统一
// 语义配色，供所有 CodeMirror 编辑器（go 模板 / 标签表达式 / rego）复用。
export function highlightFor(theme: string): Extension {
  const isDark = theme === DARK_THEME;
  return [
    syntaxHighlighting(isDark ? oneDarkHighlightStyle : defaultHighlightStyle),
    syntaxHighlighting(isDark ? darkHighlight : lightHighlight),
  ];
}
