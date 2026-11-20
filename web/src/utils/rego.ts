/**
 * Rego 辅助函数，供标签规则管理页面与 RegoEditor 组件共用：用于启发式展开的
 * tag/字段字面量提取、由元信息驱动的自动补全输入字段片段、Rego 骨架生成，
 * 以及轻量的本地预校验。
 */

export interface RegoValidationError {
  line: number;
  column: number;
  message: string;
  /** lint 严重级键，如 "warning" / "error" */
  type?: string;
  /** 可选提示，与消息一同渲染 */
  hint?: string;
}

/** HTTP 传输输入文档所注册的模型类型键。 */
export const HTTP_TRANSPORT_MODEL_KEY =
  "zhongkuisec.com/zhongkui/internal/models/network.HttpTransport";

/** 编辑器自动补全提供的常用 Rego 关键字。 */
export const REGO_KEYWORDS = [
  "package",
  "import",
  "as",
  "default",
  "contains",
  "if",
  "else",
  "not",
  "some",
  "every",
  "with",
  "input",
  "data",
  "true",
  "false",
  "null",
];

/**
 * Rego 编辑器自动补全的补全位置分类。
 * 编辑器会抑制或过滤在光标处非法的候选项，例如在已完成的表达式
 * `input.file_ext == ""` 之后不再提示 `and`/`abs`/`any`。
 */
export type RegoCompletionKind = "block" | "term" | "allowAll";

/** 只能作为语句/表达式开头、不能作为项（term）的关键字。 */
const EXPRESSION_START_KEYWORDS = new Set([
  "package",
  "import",
  "default",
  "some",
  "every",
  "if",
  "else",
  "contains",
  "with",
  "in",
  "not",
  "as",
]);

/** 匹配行尾完整闭合的字符串字面量：`""`、`"..."` 或 `` `...` ``。 */
const CLOSED_LITERAL_TAIL_RE =
  /(?:""|"(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`|\d+(?:\.\d+)?|true|false|null)$/;

/** 比较/赋值运算符，在整行任意位置查找。 */
const COMPARISON_OP_RE = /(?:==|!=|<=|>=|<|>|:=|=)/;

/** 期望项（term）值的行尾：运算符、逗号或左括号。 */
const TERM_POSITION_RE = /(?:==|!=|<=|>=|<|>|:=|=|,|\[|\{|\()\s*$/;

/** 期望同一行紧随一个项（term）的关键字。 */
const TERM_KEYWORD_RE = /(?:\bif\b|\bin\b|\bnot\b|\bsome\b|\bevery\b|\bcontains\b)\s*$/;

/**
 * 根据光标之前的行文本（含正在输入的部分单词）对光标处的补全上下文分类：
 * - "block"：光标前是已完成表达式/字面量，此处输入任何单词都是语法错误，
 *   不应提供补全。
 * - "term"：此处只期望一个项（值），需从补全列表中过滤掉语句开头的关键字。
 * - "allowAll"：无位置约束，保留默认补全。
 */
export function classifyRegoCompletionContext(lineTextBefore: string): RegoCompletionKind {
  // 去掉正在输入的部分单词，以便对位置本身进行分类。
  const before = lineTextBefore.replace(/[a-zA-Z_][a-zA-Z0-9_]*$/, "");

  // 比较/赋值后紧跟闭合字面量即构成完整表达式：此行再出现任何单词都是语法错误。
  if (CLOSED_LITERAL_TAIL_RE.test(before.trimEnd()) && COMPARISON_OP_RE.test(before)) {
    return "block";
  }
  if (TERM_POSITION_RE.test(before) || TERM_KEYWORD_RE.test(before)) {
    return "term";
  }
  return "allowAll";
}

/** 当补全标签为语句开头关键字时返回 true。 */
export function isRegoExpressionKeyword(label: string): boolean {
  return EXPRESSION_START_KEYWORDS.has(label);
}

/** 数组类型字段的元素元信息，如 `items` 的元素类型。 */
export interface InputItemMeta {
  type: string;
  goType: string;
}

/**
 * 由 `/tag-event/tag-rule/model/metadata` 接口返回的输入模型文档字段元信息。
 */
export interface InputFieldMeta {
  /** 输入字段名，如 "method" */
  field: string;
  /** 语义类型键，如 "string" | "number" | "boolean" | "object" | "array" */
  type: string;
  /** 后端模型字段的 Go 类型名 */
  goType: string;
  /** 在自动补全弹层中显示的简要描述 */
  description?: string;
  /** 该字段是否携带敏感数据 */
  sensitive?: boolean;
  /** 数组类型字段的元素元信息 */
  items?: InputItemMeta;
}

/** 供试运行预览使用的样例输入文档。 */
export interface InputSample {
  name: string;
  description?: string;
  input: Record<string, unknown>;
}

/** 可选择的模型选项，如渲染为 `<v-select>` 的选项项。 */
export interface ModelOption {
  value: string;
  label: string;
}

// ---- 自定义 OPA 内置函数与 sqlite 虚拟表元信息 ----

/**
 * 由 `/tag-event/tag-rule/functions` 接口返回、rego 规则内可调用的自定义 OPA
 * 内置函数元信息。驱动 Rego 编辑器的函数关键字高亮、悬停签名提示与自动补全。
 */
export interface RegoFunctionMeta {
  /** 完整函数名，如 "sqlite.query" */
  name: string;
  /** 在自动补全弹层/悬停提示中显示的本地化描述 */
  description: string;
  /** 人类可读的调用签名，如 "sqlite.query(sql, params)" */
  signature: string;
  /** 可选的使用示例文本 */
  example?: string;
}

/** sqlite 虚拟表数据源的单个列元信息。 */
export interface SqliteColumnMeta {
  name: string;
  /** 值类型名，如 "text" | "integer" | "real" | "blob" | "any" */
  type: string;
}

/**
 * 由 `/tag-event/tag-rule/sqlite/meta` 接口返回、注册为 sqlite 虚拟表的第三方
 * 数据源元信息。驱动 `sqlite.query(...)` 字符串内的 SQL 表名/列名补全。
 */
export interface SqliteDatasourceMeta {
  /** sqlite.query 引用的裸表名，如 "ip_geo" */
  name: string;
  columns: SqliteColumnMeta[];
}

/**
 * 编辑器在 `sqlite.query("...")` 字符串内提供的关键字高亮与自动补全所用的 SQL
 * 关键字，为 SQLite 只读 SELECT 子集。
 */
export const SQL_KEYWORDS = [
  "select",
  "from",
  "where",
  "and",
  "or",
  "not",
  "null",
  "as",
  "join",
  "left",
  "right",
  "inner",
  "outer",
  "cross",
  "on",
  "order",
  "by",
  "group",
  "having",
  "limit",
  "offset",
  "distinct",
  "count",
  "sum",
  "avg",
  "min",
  "max",
  "case",
  "when",
  "then",
  "else",
  "end",
  "between",
  "like",
  "in",
  "is",
  "true",
  "false",
  "asc",
  "desc",
];

/**
 * 依据语义类型为输入字段构建可插入的 Rego 片段。可与字面量比较的字段生成
 * `input.<field> == <literal>`（字符串/数字/布尔）；对象、数组与敏感字段无法
 * 做相等比较，只生成裸引用 `input.<field>`。未知类型同样回退为裸引用。
 * 可解密的敏感字段（Crypto / S3Crypto）生成 `crypto.reveal("<field>")`，
 * 直接取明文参与判定。
 */
export function fieldSnippet(field: InputFieldMeta): string {
  const ref = `input.${field.field}`;
  if (
    field.sensitive &&
    (field.goType === "*serializer.Crypto" ||
      field.goType === "serializer.Crypto" ||
      field.goType === "serializer.S3Crypto")
  ) {
    return `crypto.reveal("${field.field}")`;
  }
  switch (field.type) {
    case "string":
      return `${ref} == ""`;
    case "number":
      return `${ref} == 0`;
    case "boolean":
      return `${ref} == true`;
    case "object":
    case "array":
    case "sensitive":
      return ref;
    default:
      return ref;
  }
}

const STRING_LITERAL_RE = /("(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`)/g;

function unquoteLiteral(raw: string): string {
  const inner = raw.slice(1, -1);
  return raw.startsWith("`") ? inner : inner.replace(/\\(.)/g, "$1");
}

/**
 * 提取 Rego 规则中声明的 tag 字符串字面量：
 * `tags contains "x"` 与 `tags := { "a", "b" }` 两种形式。
 */
export function extractTagLiterals(rego: string): string[] {
  const tags = new Set<string>();
  const containsRe = /tags\s+contains\s+("(?:[^"\\]|\\.)*"|`(?:[^`]|\\.)*`)/g;
  let m: RegExpExecArray | null;
  while ((m = containsRe.exec(rego)) !== null) {
    const value = unquoteLiteral(m[1]);
    if (value) {
      tags.add(value);
    }
  }
  const setRe = /tags\s*:=\s*\{([\s\S]*?)\}/g;
  while ((m = setRe.exec(rego)) !== null) {
    const body = m[1];
    let lit: RegExpExecArray | null;
    while ((lit = STRING_LITERAL_RE.exec(body)) !== null) {
      const value = unquoteLiteral(lit[1]);
      if (value) {
        tags.add(value);
      }
    }
  }
  return [...tags];
}

/**
 * 提取 Rego 规则中以 `input.<field>` 引用的首段字段名，按首次出现顺序去重。
 */
export function extractInputFields(rego: string): string[] {
  const fields: string[] = [];
  const seen = new Set<string>();
  const re = /input\.([a-zA-Z_][a-zA-Z0-9_]*)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(rego)) !== null) {
    if (!seen.has(m[1])) {
      seen.add(m[1]);
      fields.push(m[1]);
    }
  }
  return fields;
}

/**
 * 按 `package zki` + `tags contains` 契约构建 Rego 骨架，使用给定规则名作为
 * 产出的 tag。
 */
export function buildRegoSkeleton(name: string): string {
  const tag = name.trim() || "tag";
  return [
    "package zki",
    "",
    `tags contains "${tag}" if {`,
    "\t# TODO: add tag conditions",
    "}",
    "",
  ].join("\n");
}

/** 一个开箱场景样例：标题/说明的 i18n 键 + 可插入编辑器的完整 rego 片段。 */
export interface DatasourceSample {
  /** 样例标题的 i18n 键 */
  titleKey: string;
  /** 样例一句话说明的 i18n 键 */
  descKey: string;
  /** 插入编辑器的完整 rego 片段（package zki + tags contains 契约） */
  snippet: string;
}

/**
 * buildDatasourceSamples 基于当前命中的数据源 meta 组装开箱场景样例，演示
 * sqlite.query 的存在性判断 / 取单值 / 聚合统计三种用法（含 `?` 参数绑定与
 * 行对象字段访问）。生成的片段以 `package zki` + `tags contains` 契约包裹，
 * 可直接试运行。table 为数据源裸名（sqlite.query 中 FROM 引用的表名）。
 */
export function buildDatasourceSamples(
  ds: SqliteDatasourceMeta,
  tag = "tag",
  field = "",
): DatasourceSample[] {
  const table = ds.name;
  const c1 = ds.columns[0]?.name ?? "col";
  const c2 = ds.columns[1]?.name ?? c1;
  const target = field ? `input.${field}` : "input.<字段>";
  const task = tag.trim() || "tag";

  const exists = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 存在性判断：命中任一匹配行即成立（COUNT(*) > 0）`,
    `\trows := sqlite.query("SELECT COUNT(*) AS n FROM ${table} WHERE ${c1} = ?", [${target}])`,
    "\trows[0].n > 0",
    "}",
    "",
  ].join("\n");

  const value = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 取单值：SQL 结果每行是一个对象，以 rows[0].<列> 取字段（参数 \`?\` 与数组一一对应）`,
    `\trows := sqlite.query("SELECT ${c2} FROM ${table} WHERE ${c1} = ? LIMIT 1", [${target}])`,
    "\tcount(rows) > 0",
    `\trows[0].${c2} == "期望值"`,
    "}",
    "",
  ].join("\n");

  const aggregate = [
    "package zki",
    "",
    `tags contains "${task}" if {`,
    `\t# 聚合统计：COUNT(DISTINCT ...) / SUM / AVG，结果行列名为别名 n`,
    `\trows := sqlite.query("SELECT COUNT(DISTINCT ${c1}) AS n FROM ${table}", [])`,
    "\trows[0].n >= 1",
    "}",
    "",
  ].join("\n");

  return [
    { titleKey: "guide.sample.exists", descKey: "guide.sample.existsDesc", snippet: exists },
    { titleKey: "guide.sample.value", descKey: "guide.sample.valueDesc", snippet: value },
    {
      titleKey: "guide.sample.aggregate",
      descKey: "guide.sample.aggregateDesc",
      snippet: aggregate,
    },
  ];
}

/**
 * 轻量的本地 Rego 预检，用于输入时即时反馈。检查括号/引号配对以及必需的
 * `package zki` / `tags` 声明。不能替代后端权威校验。
 */
export function preflightRego(src: string): RegoValidationError[] {
  const errors: RegoValidationError[] = [];
  const stack: { ch: string; line: number; column: number }[] = [];
  let line = 1;
  let column = 1;
  let inComment = false;
  let inString: string | null = null;
  let escaped = false;

  const push = (message: string, l: number, c: number) =>
    errors.push({ line: l, column: c, message });

  for (let i = 0; i < src.length; i++) {
    const ch = src[i];
    if (inComment) {
      if (ch === "\n") {
        inComment = false;
        line += 1;
        column = 1;
      } else {
        column += 1;
      }
      continue;
    }
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === "\\") {
        escaped = true;
      } else if (ch === inString) {
        inString = null;
      }
      column += 1;
      continue;
    }
    if (ch === "#") {
      inComment = true;
      continue;
    }
    if (ch === '"' || ch === "`") {
      inString = ch;
      column += 1;
      continue;
    }
    if (ch === "{" || ch === "(") {
      stack.push({ ch, line, column });
      column += 1;
      continue;
    }
    if (ch === "}" || ch === ")") {
      const open = stack.pop();
      const expected = ch === "}" ? "{" : "(";
      if (!open || open.ch !== expected) {
        push(`unmatched '${ch}'`, line, column);
      }
      column += 1;
      continue;
    }
    if (ch === "\n") {
      line += 1;
      column = 1;
      continue;
    }
    column += 1;
  }

  if (inString) {
    push("unterminated string", line, column);
  }
  for (const open of stack) {
    push(`unmatched '${open.ch}'`, open.line, open.column);
  }
  if (!/\bpackage\s+zki\b/.test(src)) {
    push("missing `package zki`", 1, 1);
  }
  if (!/tags\s+(contains|:=)/.test(src)) {
    push("missing `tags` output declaration", 1, 1);
  }
  // 与后端 TagRuleCreateIn.Rego 的 max=65536（64KiB）保持一致
  if (src.length > 65536) {
    push("rego exceeds the maximum length of 65536 characters", 1, 1);
  }

  errors.sort((a, b) => a.line - b.line || a.column - b.column);
  return errors;
}
