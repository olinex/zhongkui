// 第三方
import { defaultHighlightStyle, HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { oneDarkHighlightStyle } from "@codemirror/theme-one-dark";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";

// 类型
import type { Extension } from "@codemirror/state";

// 插件 / 工具
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

// 调色板：亮 / 暗两套语义配色，是本文件（及所有编辑器语义高亮）的唯一色值来源。
// HighlightStyle 与 SQL 语义高亮主题（sqlHighlightFor）都引用同一常量，禁止色值双写。
interface HighlightPalette {
  comment: string;
  keyword: string;
  string: string;
  number: string;
  variable: string;
  function: string;
  operator: string;
  bracket: string;
  meta: string;
}

const LIGHT_PALETTE: HighlightPalette = {
  comment: "#6a737d",
  keyword: "#cf222e",
  string: "#0a7d22",
  number: "#953800",
  variable: "#0550ae",
  function: "#6f42c1",
  operator: "#1f2328",
  bracket: "#6f42c1",
  meta: "#57606a",
};

const DARK_PALETTE: HighlightPalette = {
  comment: "#7f848e",
  keyword: "#c678dd",
  string: "#98c379",
  number: "#d19a66",
  variable: "#61afef",
  function: "#e5c07b",
  operator: "#abb2bf",
  bracket: "#56b6c2",
  meta: "#5c6370",
};

const lightHighlight = HighlightStyle.define([
  { tag: tags.comment, color: LIGHT_PALETTE.comment, fontStyle: "italic" },
  { tag: tags.keyword, color: LIGHT_PALETTE.keyword },
  { tag: tags.bool, color: LIGHT_PALETTE.keyword },
  { tag: tags.string, color: LIGHT_PALETTE.string },
  { tag: tags.number, color: LIGHT_PALETTE.number },
  { tag: tags.variableName, color: LIGHT_PALETTE.variable },
  { tag: tags.propertyName, color: LIGHT_PALETTE.variable },
  { tag: tags.atom, color: LIGHT_PALETTE.variable },
  { tag: tags.function(tags.variableName), color: LIGHT_PALETTE.function },
  { tag: tags.operator, color: LIGHT_PALETTE.operator },
  { tag: tags.bracket, color: LIGHT_PALETTE.bracket },
  { tag: tags.meta, color: LIGHT_PALETTE.meta },
]);

const darkHighlight = HighlightStyle.define([
  { tag: tags.comment, color: DARK_PALETTE.comment, fontStyle: "italic" },
  { tag: tags.keyword, color: DARK_PALETTE.keyword },
  { tag: tags.bool, color: DARK_PALETTE.keyword },
  { tag: tags.string, color: DARK_PALETTE.string },
  { tag: tags.number, color: DARK_PALETTE.number },
  { tag: tags.variableName, color: DARK_PALETTE.variable },
  { tag: tags.propertyName, color: DARK_PALETTE.variable },
  { tag: tags.atom, color: DARK_PALETTE.variable },
  { tag: tags.function(tags.variableName), color: DARK_PALETTE.function },
  { tag: tags.operator, color: DARK_PALETTE.operator },
  { tag: tags.bracket, color: DARK_PALETTE.bracket },
  { tag: tags.meta, color: DARK_PALETTE.meta },
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

// sqlHighlightFor 生成 sqlite.query("...") 字符串内 SQL 语义高亮的主题：关键词 / 表名 /
// 单引号字符串字面量三个装饰类（.cm-sql-keyword / .cm-sql-table / .cm-sql-string），
// 色值取自与 Rego 语法高亮同源的调色板，随主题 reconfigure 切换。
export function sqlHighlightFor(theme: string): Extension {
  const palette = theme === DARK_THEME ? DARK_PALETTE : LIGHT_PALETTE;
  return EditorView.theme({
    ".cm-sql-keyword": {
      color: palette.keyword,
      fontWeight: "var(--zk-font-weight-semibold)",
    },
    ".cm-sql-table": { color: palette.variable },
    ".cm-sql-string": { color: palette.string },
  });
}
