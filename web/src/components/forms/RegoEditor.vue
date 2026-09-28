<template>
  <!-- 全屏时 Teleport 到 body，脱离对话框 .v-overlay__content 的 transform 祖先，
       否则 position:fixed 的相对包含块仍是对话框容器，无法真正铺满视口 -->
  <teleport to="body" :disabled="!fullscreen">
    <div
      class="rego-editor-wrap"
      :class="{ 'is-fullscreen': fullscreen, 'bg-surface': fullscreen }"
    >
      <div class="rego-toolbar">
        <div class="sample-selector">
          <v-select
            v-if="samples.length > 0"
            v-model="selectedSampleName"
            :items="samples.map((s) => ({ title: s.name, value: s.name }))"
            :label="$t('action.selectSample')"
            hide-details
            clearable
          >
            <template #append-inner>
              <v-btn
                variant="text"
                size="small"
                icon="mdi-eye-outline"
                :disabled="!selectedSampleName"
                :title="$t('action.viewInput')"
                :aria-label="$t('action.viewInput')"
                @click.stop="emit('viewInput')"
              />
            </template>
          </v-select>
        </div>
        <v-btn
          class="toolbar-btn"
          :icon="fullscreen ? 'mdi-fullscreen-exit' : 'mdi-fullscreen'"
          :variant="fullscreen ? 'tonal' : 'text'"
          :disabled="disabled"
          :title="$t(fullscreen ? 'action.exitFullscreen' : 'action.fullscreen')"
          :aria-label="$t(fullscreen ? 'action.exitFullscreen' : 'action.fullscreen')"
          @click="toggleFullscreen"
        />
        <v-btn
          class="toolbar-btn"
          icon="mdi-database-search-outline"
          :variant="guideOpen ? 'tonal' : 'text'"
          color="primary"
          :disabled="disabled"
          :title="$t('guide.title')"
          :aria-label="$t('guide.title')"
          @click="openGuide"
        />
      </div>
      <div class="rego-body">
        <!-- minHeight 为公开 prop（tag-rule.vue / tag-event-rule.vue / notification/template.vue
             等调用方按场景传入自定义最小高度），属运行时计算值，故保留 :style 绑定 -->
        <div ref="hostRef" class="rego-editor zk-editor" :style="{ minHeight }" />
        <transition name="guide">
          <div v-if="fullscreen && guideOpen" class="fullscreen-guide zk-panel">
            <datasource-guide-panel
              :datasources="sqliteDatasources"
              :fields="fields"
              @insert="handleGuideInsert"
            />
          </div>
        </transition>
      </div>
    </div>
  </teleport>
</template>

<script setup lang="ts" name="rego-editor">
import {
  autocompletion,
  completeFromList,
  type Completion,
  type CompletionContext,
  type CompletionResult,
  type CompletionSource,
} from "@codemirror/autocomplete";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { bracketMatching, syntaxTree } from "@codemirror/language";
import { linter, setDiagnostics, type Diagnostic } from "@codemirror/lint";
import {
  Compartment,
  EditorState,
  StateField,
  type Extension,
  type Range,
} from "@codemirror/state";
import {
  Decoration,
  EditorView,
  highlightActiveLine,
  hoverTooltip,
  keymap,
  lineNumbers,
  placeholder,
  type DecorationSet,
} from "@codemirror/view";
import { rego } from "codemirror-lang-rego";

import { useAppStore } from "@/stores/app";
import { highlightFor, sqlHighlightFor } from "@/utils/editorHighlight";
import i18n from "@/utils/i18n";
import {
  classifyRegoCompletionContext,
  fieldSnippet,
  isRegoExpressionKeyword,
  SQL_KEYWORDS,
} from "@/utils/rego";
import type {
  InputFieldMeta,
  RegoFunctionMeta,
  RegoValidationError,
  SqliteDatasourceMeta,
} from "@/utils/rego";
import { findSlashPos, slashKeyBinding, withSlashApply } from "@/utils/slashTrigger";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    errors?: RegoValidationError[];
    /** 额外的 `input.<field>` 补全项，如既有规则引用到的字段 */
    extraCompletions?: string[];
    /** 来自后端的输入文档字段元信息，驱动 `input.` 与 "/" 补全 */
    fields?: InputFieldMeta[];
    /** 来自后端的自定义 OPA 内置函数元信息，驱动函数补全与悬停提示 */
    functions?: RegoFunctionMeta[];
    /** sqlite 虚拟表数据源 schema，驱动 `sqlite.query` 字符串内的 SQL 补全 */
    sqliteDatasources?: SqliteDatasourceMeta[];
    /** 当前模型可用的样例，驱动样例选择器 */
    samples?: { name: string; input: Record<string, unknown> }[];
    /** 禁用编辑（只读），组件仍保持可交互 */
    disabled?: boolean;
    minHeight?: string;
  }>(),
  {
    errors: () => [],
    extraCompletions: () => [],
    fields: () => [],
    functions: () => [],
    sqliteDatasources: () => [],
    samples: () => [],
    disabled: false,
    minHeight: "240px",
  },
);

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
  (e: "update:sampleInput", value: Record<string, unknown> | null): void;
  (e: "viewInput"): void;
}>();

const appStore = useAppStore();

const hostRef = useTemplateRef<HTMLDivElement>("hostRef");
const view = shallowRef<EditorView>();

// ---- 全屏编辑模式 与 参考侧边栏（数据源速查 + 场景样例）----
// 第三方数据源参考栏仅在全屏模式下以右侧并排分栏呈现（充足宽度，与编辑器同框）；
// 点击「第三方数据源查询」若未全屏则强制进入全屏并打开参考栏。只读（disabled）时禁用。
const fullscreen = ref(false);
const guideOpen = ref(false);

/** 切换全屏；退出全屏时一并收起参考栏（参考栏只存在于全屏模式）。 */
function toggleFullscreen(): void {
  fullscreen.value = !fullscreen.value;
  if (!fullscreen.value) {
    guideOpen.value = false;
  }
}

/** 打开第三方数据源参考：未全屏则先进入全屏；已全屏则在参考栏显隐间切换。 */
function openGuide(): void {
  if (!fullscreen.value) {
    fullscreen.value = true;
    guideOpen.value = true;
    return;
  }
  guideOpen.value = !guideOpen.value;
}

// 全屏布局切换后重新测量，保证 CodeMirror 行宽度/滚动区正确适配（lineWrapping 需要）
watch(fullscreen, async (open) => {
  if (open) {
    await nextTick();
    view.value?.requestMeasure();
  }
});

/** 在光标处插入文本并置于插入内容末尾（速查/样例一键写入），随后把焦点还给编辑器。 */
function insertAtCursor(text: string): void {
  const editor = view.value;
  if (!editor) {
    return;
  }
  const head = editor.state.selection.main.head;
  editor.dispatch({
    changes: { from: head, insert: text },
    selection: { anchor: head + text.length },
  });
  editor.focus();
}

/** 抽屉内 DatasourceGuidePanel 的一键插入，插入后保持抽屉展开以便连续取用。 */
const handleGuideInsert = (text: string): void => insertAtCursor(text);

// ---- 样例选择器状态 ----
// 试运行样例与编辑器联动：用户预先选择一个样例后，编辑器对 `input.<field>`
// 的补全与悬停提示会展示该字段在样例中的实际值，便于快速判断规则合法性。
const selectedSampleName = ref("");
const selectedSampleInput = shallowRef<Record<string, unknown> | null>(null);

/** 格式化样例值，供补全/悬停提示展示。 */
function formatSampleValue(value: unknown): string {
  if (value === null) {
    return "null";
  }
  if (value === undefined) {
    return "undefined";
  }
  if (typeof value === "string") {
    return JSON.stringify(value);
  }
  if (typeof value === "object") {
    return JSON.stringify(value);
  }
  return String(value);
}

/** 查找 `input.<field>` 引用对应的样例值（若存在）。 */
function sampleValueFor(field: string): unknown {
  return selectedSampleInput.value?.[field];
}

/** 查找覆盖给定文档位置的 `input.<field>` 词元。 */
function inputFieldAt(
  editorView: EditorView,
  pos: number,
): { field: string; from: number; to: number } | null {
  const line = editorView.state.doc.lineAt(pos);
  const rel = pos - line.from;
  const re = /input\.([a-zA-Z_][a-zA-Z0-9_]*)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(line.text)) !== null) {
    const start = m.index;
    const end = m.index + m[0].length;
    if (rel >= start && rel <= end) {
      return { field: m[1], from: line.from + start, to: line.from + end };
    }
  }
  return null;
}

// 语法高亮随主题切换，配色统一走 utils/editorHighlight 的共享方案。
const highlightCompartment = new Compartment();

/** 插入完整片段并把光标置于其引号内部。 */
function applySnippet(
  editorView: EditorView,
  _completion: Completion,
  from: number,
  to: number,
  snippet: string,
): void {
  const cursor = Math.max(0, snippet.indexOf('"')) + 1;
  editorView.dispatch({
    changes: { from, to, insert: snippet },
    selection: { anchor: from + cursor },
  });
}

/** 本地化语义输入类型键，无对应翻译时回退为原始类型文本。 */
function inputTypeLabel(type: string): string {
  const key = `inputType.${type}`;
  return i18n.global.te(key) ? i18n.global.t(key) : type;
}

/**
 * 由 `fields` 元信息合并 `extraCompletions` 构建 `input.<field>` 补全
 * （按字段名去重，元信息优先）。敏感字段在标签中加标记、在详情文本中加提示。
 * 选中样例后，会把字段的实际值附加到详情，便于用户边输入边判断规则正确性。
 * `prefix` 用于 "/" 触发的「输入即筛选」：斜杠补全走 filter:false 由本函数
 * 按前缀过滤（与 CM 内置前缀匹配一致，大小写不敏感）；语言补全等场景传 ""。
 */
function inputFieldCompletions(prefix: string): Completion[] {
  const items: Completion[] = [];
  const seen = new Set<string>();
  const matches = (label: string): boolean => label.toLowerCase().startsWith(prefix.toLowerCase());
  for (const field of props.fields) {
    seen.add(field.field);
    const parts = [inputTypeLabel(field.type)];
    if (field.description) {
      parts.push(field.description);
    }
    if (field.sensitive) {
      parts.push("sensitive");
    }
    const sampleValue = sampleValueFor(field.field);
    if (sampleValue !== undefined) {
      parts.push(`= ${formatSampleValue(sampleValue)}`);
    }
    const label = field.sensitive ? `${field.field} (sensitive)` : field.field;
    if (!matches(label)) {
      continue;
    }
    items.push({
      label,
      detail: parts.join(" · "),
      type: "variable",
      apply: (v, c, from, to) => applySnippet(v, c, from, to, fieldSnippet(field)),
    });
  }
  for (const field of props.extraCompletions) {
    if (seen.has(field)) {
      continue;
    }
    seen.add(field);
    if (!matches(field)) {
      continue;
    }
    const parts = ["existing rule field"];
    const sampleValue = sampleValueFor(field);
    if (sampleValue !== undefined) {
      parts.push(`= ${formatSampleValue(sampleValue)}`);
    }
    items.push({
      label: field,
      detail: parts.join(" · "),
      type: "variable",
      apply: (v, c, from, to) => applySnippet(v, c, from, to, `input.${field} == ""`),
    });
  }
  return items;
}

// 判断光标是否位于 Rego 字符串 / 原始字符串 / 注释内（"/" 触发在这些字面量
// 内部一律不生效，避免 "http://..." 等误弹字段下拉）
function inRegoLiteral(state: EditorState, pos: number): boolean {
  const innermost = syntaxTree(state).resolveInner(pos, -1);
  let node: typeof innermost | null = innermost;
  while (node) {
    const name = node.name;
    if (name === "String" || name === "RawString" || name === "LineComment") {
      return true;
    }
    node = node.parent;
  }
  return false;
}

// ---- 自定义 OPA 内置函数与嵌套 sqlite.query 支持 ----

/**
 * 最小语法节点结构：只声明本组件用到的字段，避免依赖 @lezer/common 类型
 * （pnpm 严格模式下不可直接 import 传递依赖）。
 */
interface NodeLike {
  from: number;
  to: number;
  name: string;
  parent: NodeLike | null;
  firstChild: NodeLike | null;
}

/** 判断语法节点是否为 sqlite.query 调用的第一参数字符串（String/RawString）。 */
function isSqliteQueryFirstArg(state: EditorState, node: NodeLike): boolean {
  let parent = node.parent;
  while (parent && parent.name !== "ArgList") {
    parent = parent.parent;
  }
  if (!parent) {
    return false;
  }
  const call = parent.parent;
  if (!call || call.name !== "CallExpression") {
    return false;
  }
  const callee = call.firstChild;
  if (!callee) {
    return false;
  }
  return state.doc.sliceString(callee.from, callee.to) === "sqlite.query";
}

/**
 * 定位覆盖给定位置的 sqlite.query(...) 第一参数字符串，返回字符串节点边界与
 * 内部 SQL 文本。非 sqlite.query 调用或位置在引号外时返回 null。
 */
function sqliteQueryStringAt(
  state: EditorState,
  pos: number,
): { from: number; to: number; sql: string } | null {
  const inner = syntaxTree(state).resolveInner(pos, -1);
  let node: typeof inner | null = inner;
  while (node && node.name !== "String" && node.name !== "RawString") {
    node = node.parent;
  }
  if (!node) {
    return null;
  }
  // 光标须位于字符串引号内部（空字符串 "" 时两引号之间同样合法）
  if (pos < node.from + 1 || pos > node.to - 1) {
    return null;
  }
  if (!isSqliteQueryFirstArg(state, node)) {
    return null;
  }
  return {
    from: node.from,
    to: node.to,
    sql: state.doc.sliceString(node.from + 1, node.to - 1),
  };
}

/** 定位覆盖给定位置的 sqlite.query 调用，返回函数元信息与调用名范围。 */
function customFunctionAt(
  state: EditorState,
  pos: number,
): { fn: RegoFunctionMeta; from: number; to: number } | null {
  const inner = syntaxTree(state).resolveInner(pos, -1);
  let node: typeof inner | null = inner;
  while (node && node.name !== "CallExpression") {
    node = node.parent;
  }
  if (!node) {
    return null;
  }
  const callee = node.firstChild;
  if (!callee) {
    return null;
  }
  const text = state.doc.sliceString(callee.from, callee.to);
  const fn = props.functions.find((f) => f.name === text);
  if (!fn) {
    return null;
  }
  return { fn, from: callee.from, to: callee.to };
}

/** 构造单个自定义函数的补全项（label 可为全名或局部名，slash 模式用全名）。 */
function functionCompletionOption(fn: RegoFunctionMeta, label: string): Completion {
  const detailParts = [fn.signature];
  if (fn.description) {
    detailParts.push(fn.description);
  }
  return {
    label,
    type: "function",
    // boost 置顶：斜杠菜单中字段项众多，sqlite.query 若排在末尾会被淹没
    boost: 1,
    detail: detailParts.join(" · "),
    info: fn.example
      ? () => {
          const dom = document.createElement("pre");
          dom.className = "cm-function-info";
          dom.textContent = fn.example ?? "";
          return dom;
        }
      : undefined,
    apply: (v, _c, from, to) => {
      const insert = `${fn.name}(`;
      v.dispatch({
        changes: { from, to, insert },
        selection: { anchor: from + insert.length },
      });
    },
  };
}

/**
 * 自定义 OPA 内置函数补全：输入 `sqlite` / `sqlite.` 时给出完整函数名或局部名，
 * 选中后补出调用括号并把光标定位到括号后等待输入参数。
 */
function customFunctionCompletion(context: CompletionContext): CompletionResult | null {
  if (props.functions.length === 0) {
    return null;
  }
  const line = context.state.doc.lineAt(context.pos);
  const textBefore = line.text.slice(0, context.pos - line.from);
  const word = /[a-zA-Z_][a-zA-Z0-9_.]*$/.exec(textBefore)?.[0] ?? "";
  if (!word) {
    return null;
  }
  const options: Completion[] = [];
  for (const fn of props.functions) {
    const dot = fn.name.lastIndexOf(".");
    const ns = fn.name.slice(0, dot);
    const local = fn.name.slice(dot + 1);
    let label: string | null = null;
    if (word.startsWith(`${ns}.`)) {
      // 已输入命名空间点号（sqlite.）：补局部名（匹配局部名前缀）
      if (local.startsWith(word.slice(ns.length + 1))) {
        label = local;
      }
    } else if (fn.name.startsWith(word)) {
      label = fn.name;
    }
    if (label === null) {
      continue;
    }
    options.push(functionCompletionOption(fn, label));
  }
  if (options.length === 0) {
    return null;
  }
  return { from: context.pos - word.length, options };
}

/** "/" 触发模式下按查询前缀过滤的自定义函数补全项（与字段项合并展示）。 */
function functionSlashOptions(query: string): Completion[] {
  const options: Completion[] = [];
  for (const fn of props.functions) {
    if (!fn.name.toLowerCase().startsWith(query.toLowerCase())) {
      continue;
    }
    options.push(functionCompletionOption(fn, fn.name));
  }
  return options;
}

/** SQL 关键词的补全项，全小写关键字。 */
function sqliteKeywordOptions(): Completion[] {
  return SQL_KEYWORDS.map((kw) => ({ label: kw, type: "keyword" }));
}

/** 数据源列补全项（不含表名前缀）。 */
function sqliteColumnOptions(ds: SqliteDatasourceMeta): Completion[] {
  return ds.columns.map((col) => ({
    label: col.name,
    type: "property",
    detail: col.type,
  }));
}

/** 数据源表名补全项。 */
function sqliteTableOptions(): Completion[] {
  return props.sqliteDatasources.map((ds) => ({
    label: ds.name,
    type: "table",
  }));
}

/**
 * sqlite.query("...") 字符串内的 SQL 补全：FROM/JOIN 后补表名、`table.` 前缀补该表
 * 列名、已出现 FROM 表时补列名 + 关键词，其余位置补关键词 + 表名。SQLite 只读
 * SELECT 子集，补全内容全部来自后端元信息与内置关键词。
 */
function sqliteCompletion(context: CompletionContext): CompletionResult | null {
  const hit = sqliteQueryStringAt(context.state, context.pos);
  if (!hit) {
    return null;
  }
  const before = context.state.doc.sliceString(hit.from + 1, context.pos);
  const word = /[a-zA-Z_][a-zA-Z0-9_]*$/.exec(before)?.[0] ?? "";
  const wordFrom = context.pos - word.length;
  const beforeWord = before.slice(0, -word.length);

  // 1. `table.col` 列引用 → 该表列名
  const dotMatch = /([a-zA-Z_][a-zA-Z0-9_]*)\.[a-zA-Z_][a-zA-Z0-9_]*$/.exec(before);
  if (dotMatch) {
    const ds = props.sqliteDatasources.find((d) => d.name === dotMatch[1]);
    if (ds) {
      return { from: wordFrom, options: sqliteColumnOptions(ds) };
    }
  }

  // 2. FROM / JOIN 等关键词后 → 表名
  if (/(?:^|[\s,(])(?:from|join|into|update|table)\s+[a-zA-Z_][a-zA-Z0-9_]*$/i.test(beforeWord)) {
    return { from: wordFrom, options: sqliteTableOptions() };
  }

  // 3. 已出现 FROM 表 → 该表列名 + 关键词
  const fromMatch = /\bfrom\s+([a-zA-Z_][a-zA-Z0-9_]*)/i.exec(before);
  if (fromMatch) {
    const ds = props.sqliteDatasources.find((d) => d.name === fromMatch[1]);
    if (ds) {
      return {
        from: wordFrom,
        options: [...sqliteColumnOptions(ds), ...sqliteKeywordOptions()],
      };
    }
  }

  // 4. 兜底：关键词 + 表名
  return { from: wordFrom, options: [...sqliteKeywordOptions(), ...sqliteTableOptions()] };
}

// SQL 关键词/表名/字符串字面量高亮：仅在 sqlite.query("...") 参数字符串内部叠加
// 装饰器，与 Rego 语法高亮共存，随文档编辑增量重建。
const sqliteHighlightField = StateField.define<DecorationSet>({
  create(state) {
    return computeSqliteHighlights(state);
  },
  update(deco, tr) {
    if (!tr.docChanged) {
      return deco.map(tr.changes);
    }
    return computeSqliteHighlights(tr.state);
  },
  provide: (field) => EditorView.decorations.from(field),
});

/**
 * 为 sqlite.query("...") 参数字符串内部的 SQL 关键字、单引号字符串字面量与
 * FROM/JOIN 后的已注册表名叠加高亮装饰，随文档编辑增量重建。
 */
function computeSqliteHighlights(state: EditorState): DecorationSet {
  const decos: Range<Decoration>[] = [];
  const cursor = syntaxTree(state).cursor();
  while (cursor.next()) {
    if (cursor.name !== "String" && cursor.name !== "RawString") {
      continue;
    }
    const node = cursor.node;
    if (!isSqliteQueryFirstArg(state, node)) {
      continue;
    }
    const innerFrom = node.from + 1;
    const innerTo = node.to - 1;
    if (innerTo <= innerFrom) {
      continue;
    }
    const sql = state.doc.sliceString(innerFrom, innerTo);

    // SQL 关键词
    const keywordRe = new RegExp(`\\b(?:${SQL_KEYWORDS.join("|")})\\b`, "gi");
    let m: RegExpExecArray | null;
    while ((m = keywordRe.exec(sql)) !== null) {
      decos.push(
        Decoration.mark({ class: "cm-sql-keyword" }).range(
          innerFrom + m.index,
          innerFrom + m.index + m[0].length,
        ),
      );
    }
    // 单引号字符串字面量（'' 转义）
    const strRe = /'(?:[^']|'')*'/g;
    let sm: RegExpExecArray | null;
    while ((sm = strRe.exec(sql)) !== null) {
      decos.push(
        Decoration.mark({ class: "cm-sql-string" }).range(
          innerFrom + sm.index,
          innerFrom + sm.index + sm[0].length,
        ),
      );
    }
    // FROM / JOIN 后已注册的表名
    const tableRe = /\b(?:from|join)\s+([a-zA-Z_][a-zA-Z0-9_]*)/gi;
    let tm: RegExpExecArray | null;
    while ((tm = tableRe.exec(sql)) !== null) {
      const name = tm[1];
      if (!props.sqliteDatasources.some((d) => d.name === name)) {
        continue;
      }
      const relFrom = tm.index + tm[0].lastIndexOf(name);
      decos.push(
        Decoration.mark({ class: "cm-sql-table" }).range(
          innerFrom + relFrom,
          innerFrom + relFrom + name.length,
        ),
      );
    }
  }
  // Decoration.set 的 sort=true 要求按 from（同 from 按 to）升序排列
  decos.sort((a, b) => a.from - b.from || a.to - b.to);
  return Decoration.set(decos, true);
}

/**
 * sqlite.query("...") 内 SQL 的轻量语法提示：仅允许只读 SELECT、引用的表必须已注册、
 * 括号与单引号字符串必须配对闭合。与后端校验诊断合并后统一经 lint 渲染。
 */
function sqliteSqlDiagnostics(state: EditorState): Diagnostic[] {
  const diagnostics: Diagnostic[] = [];
  const cursor = syntaxTree(state).cursor();
  while (cursor.next()) {
    if (cursor.name !== "String" && cursor.name !== "RawString") {
      continue;
    }
    const node = cursor.node;
    if (!isSqliteQueryFirstArg(state, node)) {
      continue;
    }
    const innerFrom = node.from + 1;
    const innerTo = node.to - 1;
    if (innerTo <= innerFrom) {
      continue;
    }
    const sql = state.doc.sliceString(innerFrom, innerTo);
    const trimmed = sql.trim();

    if (trimmed && !/^select\b/i.test(trimmed)) {
      diagnostics.push({
        from: innerFrom,
        to: innerTo,
        severity: "warning",
        message: i18n.global.t("action.regoSqlMustStartSelect"),
      });
    }

    // 单引号字符串闭合检查
    let strIdx = sql.indexOf("'");
    while (strIdx !== -1) {
      let j = strIdx + 1;
      while (j < sql.length) {
        if (sql[j] === "'") {
          if (sql[j + 1] === "'") {
            j += 2;
            continue;
          }
          break;
        }
        j++;
      }
      if (j >= sql.length) {
        diagnostics.push({
          from: innerFrom + strIdx,
          to: innerTo,
          severity: "error",
          message: i18n.global.t("action.regoSqlUnclosedString"),
        });
      }
      strIdx = sql.indexOf("'", j + 1);
    }

    // 括号配对检查（跳过单引号字符串内部）
    const stack: { ch: string; index: number }[] = [];
    let inStr = false;
    for (let i = 0; i < sql.length; i++) {
      const ch = sql[i];
      if (ch === "'") {
        if (inStr) {
          if (sql[i + 1] === "'") {
            i++;
            continue;
          }
          inStr = false;
        } else {
          inStr = true;
        }
        continue;
      }
      if (inStr) {
        continue;
      }
      if (ch === "(") {
        stack.push({ ch, index: i });
      } else if (ch === ")") {
        const open = stack.pop();
        if (!open) {
          diagnostics.push({
            from: innerFrom + i,
            to: innerFrom + i + 1,
            severity: "error",
            message: i18n.global.t("action.regoSqlUnmatchedParen"),
          });
        }
      }
    }
    for (const open of stack) {
      diagnostics.push({
        from: innerFrom + open.index,
        to: innerFrom + open.index + 1,
        severity: "error",
        message: i18n.global.t("action.regoSqlUnmatchedParen"),
      });
    }

    // FROM 引用的表必须已注册
    const tableRe = /\bfrom\s+([a-zA-Z_][a-zA-Z0-9_]*)/gi;
    let tm: RegExpExecArray | null;
    while ((tm = tableRe.exec(sql)) !== null) {
      const name = tm[1];
      if (props.sqliteDatasources.some((d) => d.name === name)) {
        continue;
      }
      const from = innerFrom + tm.index + tm[0].lastIndexOf(name);
      diagnostics.push({
        from,
        to: from + name.length,
        severity: "warning",
        message: i18n.global.t("action.regoSqlUnknownTable", { table: name }),
      });
    }
  }
  return diagnostics;
}

/**
 * 自定义补全源：先由字段元信息与 `extraCompletions` 补全 `input.<field>`，
 * 再回退到 `codemirror-lang-rego` 注册的 Rego 语言补全源（关键字、OPA 内置函数）。
 * 回退是位置感知的：当后续单词会构成语法错误时不提供补全（如 `input.field == "..."`
 * 之后），项（term）位置（如 `==` 之后）会过滤掉 `if`/`else` 等语句开头关键字。
 */
async function regoCompletion(context: CompletionContext): Promise<CompletionResult | null> {
  const line = context.state.doc.lineAt(context.pos);
  const textBefore = line.text.slice(0, context.pos - line.from);

  // 1. "/" 字段/函数触发器：弹出字段与自定义函数下拉并随输入实时过滤。from 指向
  //    触发斜杠本身，使孤立 "/"（空查询）也满足 from < to 立即弹层；filter:false
  //    关闭 CM 内置过滤（过滤由 inputFieldCompletions / functionSlashOptions 按斜杠
  //    后查询完成），不设 validFor 让每次输入都重查实现「输入即筛选」；选中时经
  //    withSlashApply 把斜杠与查询一并替换为字段片段或函数调用。第二个 "/"（取消
  //    触发）在 keymap 层被吞掉。
  const slashFrom = findSlashPos(context.state, context.pos, inRegoLiteral);
  if (slashFrom !== null) {
    const query = textBefore.slice(slashFrom - line.from + 1);
    if (/^[a-zA-Z0-9_]*$/.test(query)) {
      const options = withSlashApply([
        // 函数项在前：filter:false 下 CM 不按 boost 排序，仅按数组顺序展示，
        // 字段项众多，函数（sqlite.query）置顶避免被淹没
        ...functionSlashOptions(query),
        ...inputFieldCompletions(query),
      ]);
      if (options.length > 0) {
        return { from: slashFrom, options, filter: false };
      }
    }
    // 触发斜杠存在但查询含非词字符（粘贴等场景）：不弹下拉，也不落到语言补全
    return null;
  }
  // "//" 视为字面量斜杠：不弹任何下拉
  if (/\/\/$/.test(textBefore)) {
    return null;
  }

  // 2. sqlite.query("...") 字符串内部 → SQL 补全（表名/列名/关键词）
  if (sqliteQueryStringAt(context.state, context.pos)) {
    return sqliteCompletion(context);
  }

  // 3. 自定义 OPA 内置函数名补全（如 sqlite / sqlite.）
  const fnResult = customFunctionCompletion(context);
  if (fnResult) {
    return fnResult;
  }

  const dotMatch = /input\.([a-zA-Z_][a-zA-Z0-9_]*)$/.exec(textBefore);
  if (dotMatch) {
    const result = await completeFromList(inputFieldCompletions(""))(context);
    if (result) {
      return result;
    }
  }
  const kind = classifyRegoCompletionContext(textBefore);
  if (kind === "block") {
    return null;
  }
  const sources = context.state.languageDataAt<CompletionSource>("autocomplete", context.pos);
  for (const source of sources) {
    const result = await source(context);
    if (result) {
      if (kind === "term") {
        return {
          from: result.from,
          validFor: result.validFor,
          options: result.options.filter((c) => !isRegoExpressionKeyword(c.label)),
        };
      }
      return result;
    }
  }
  return null;
}

// 动态切换只读编辑（如编辑对话框在用户切换到编辑模式前锁定字段）。
const editableCompartment = new Compartment();

// 悬停提示：选中样例后，悬停在 `input.<field>` 引用上会显示该字段在样例中的
// 实际值，便于用户无需试运行即可判断规则正确性。
const sampleHover = hoverTooltip((editorView, pos) => {
  const hit = inputFieldAt(editorView, pos);
  if (!hit) {
    return null;
  }
  const value = sampleValueFor(hit.field);
  if (value === undefined) {
    return null;
  }
  return {
    pos: hit.from,
    end: hit.to,
    above: true,
    create: () => {
      const dom = document.createElement("div");
      dom.className = "cm-sample-hover";
      const label = document.createElement("span");
      label.className = "cm-sample-hover-label";
      label.textContent = hit.field;
      const val = document.createElement("span");
      val.className = "cm-sample-hover-value";
      val.textContent = formatSampleValue(value);
      dom.append(label, val);
      return { dom };
    },
  };
});

// 悬停提示：悬停在自定义 OPA 内置函数调用（如 sqlite.query）上时，显示来自后端
// 元信息的本地化描述、签名与使用示例，便于用户无需离开编辑器即可查看调用契约。
const functionHover = hoverTooltip((editorView, pos) => {
  const hit = customFunctionAt(editorView.state, pos);
  if (!hit) {
    return null;
  }
  return {
    pos: hit.from,
    end: hit.to,
    above: true,
    create: () => {
      const dom = document.createElement("div");
      dom.className = "cm-function-hover";
      const sig = document.createElement("div");
      sig.className = "cm-function-hover-signature";
      sig.textContent = hit.fn.signature;
      dom.append(sig);
      if (hit.fn.description) {
        const desc = document.createElement("div");
        desc.className = "cm-function-hover-description";
        desc.textContent = hit.fn.description;
        dom.append(desc);
      }
      if (hit.fn.example) {
        const example = document.createElement("pre");
        example.className = "cm-function-hover-example";
        example.textContent = hit.fn.example;
        dom.append(example);
      }
      return { dom };
    },
  };
});

/** 合并后端校验诊断与 sqlite.query 内 SQL 轻量提示，统一推入 lint 状态。 */
function updateDiagnostics(): void {
  const editorView = view.value;
  if (!editorView) {
    return;
  }
  const state = editorView.state;
  const doc = state.doc;
  const diagnostics: Diagnostic[] = (props.errors ?? []).map((err) => {
    const lineNo = Math.max(1, Math.min(err.line, doc.lines));
    const line = doc.line(lineNo);
    const from = line.from + Math.max(0, Math.min(err.column - 1, line.length));
    const hint = err.hint ? `\n${err.hint}` : "";
    return {
      from,
      to: line.to,
      severity: err.type === "warning" ? "warning" : "error",
      message: err.message + hint,
    };
  });
  diagnostics.push(...sqliteSqlDiagnostics(state));
  editorView.dispatch(setDiagnostics(state, diagnostics));
}

const extensions: Extension[] = [
  rego({ autocomplete: true, hover: true }),
  lineNumbers(),
  highlightActiveLine(),
  highlightCompartment.of([highlightFor(appStore.theme), sqlHighlightFor(appStore.theme)]),
  bracketMatching(),
  // 空文档时显示 "/" 用法提示（点击输入后自动隐藏）
  placeholder(i18n.global.t("action.regoFieldHint")),
  autocompletion({ override: [regoCompletion] }),
  sampleHover,
  functionHover,
  sqliteHighlightField,
  // 占位 linter，使经 `setDiagnostics` 推入的外部诊断得以渲染。
  // 不启用 lintGutter：避免行号区右侧渲染空的 lint 占位列，与其它编辑器一致；
  // 错误下划线、悬停提示与校验逻辑均不受影响。
  linter(() => []),
  history(),
  // "/" 输入规则见 slashTrigger.ts：前一字符为 "/" 且下拉已打开时该斜杠仅作为
  // 关闭信号被吞掉；其余情况按正常输入（打开下拉由补全源负责）。
  keymap.of([slashKeyBinding]),
  keymap.of([...defaultKeymap, ...historyKeymap]),
  editableCompartment.of(EditorState.readOnly.of(props.disabled)),
  EditorView.lineWrapping,
  EditorView.theme({
    "&": { fontSize: "var(--zk-editor-font-size)", height: "100%" },
    ".cm-scroller": {
      fontFamily: "var(--zk-font-mono)",
      lineHeight: "1.6",
    },
    "&.cm-focused": { outline: "none" },
  }),
  EditorView.updateListener.of((update) => {
    if (update.docChanged) {
      emit("update:modelValue", update.state.doc.toString());
      // sqlite.query 内 SQL 轻量提示随文档编辑实时刷新；后端校验诊断由 props.errors 驱动
      updateDiagnostics();
    }
  }),
];

// 编辑器容器尺寸变化（全屏切换、窗口缩放）时触发 CodeMirror 重测，
// 否则 lineWrapping 分行/横向布局不会随宽度正确重排
const resizeObserver = new ResizeObserver(() => view.value?.requestMeasure());

onMounted(() => {
  if (!hostRef.value) {
    return;
  }
  resizeObserver.observe(hostRef.value);
  view.value = new EditorView({
    state: EditorState.create({ doc: props.modelValue, extensions }),
    parent: hostRef.value,
  });
});

onBeforeUnmount(() => {
  resizeObserver.disconnect();
  view.value?.destroy();
  view.value = undefined;
});

// 样例列表变化（如切换模型）时重置样例选择。
watch(
  () => props.samples,
  (list) => {
    selectedSampleName.value = list[0] ? list[0].name : "";
  },
  { immediate: true },
);

// 向上抛出所选样例的输入，供父组件与试运行面板共享，使编辑器与试运行功能保持同步。
watch(selectedSampleName, (name) => {
  const found = props.samples.find((s) => s.name === name);
  selectedSampleInput.value = found ? found.input : null;
  emit("update:sampleInput", selectedSampleInput.value);
});

// 同步外部内容变化（如生成骨架、片段芯片）。
watch(
  () => props.modelValue,
  (val) => {
    const editorView = view.value;
    if (!editorView || editorView.state.doc.toString() === val) {
      return;
    }
    editorView.dispatch({
      changes: { from: 0, to: editorView.state.doc.length, insert: val },
    });
  },
);

// `disabled` 属性变化时切换只读状态。
watch(
  () => props.disabled,
  (val) => {
    view.value?.dispatch({
      effects: editableCompartment.reconfigure(EditorState.readOnly.of(val)),
    });
  },
);

// 应用主题变化时切换语法高亮（含 sqlite.query 的 SQL 语义配色）。
watch(
  () => appStore.theme,
  (theme) => {
    view.value?.dispatch({
      effects: highlightCompartment.reconfigure([highlightFor(theme), sqlHighlightFor(theme)]),
    });
  },
);

// 父组件每次重新校验规则时，把校验诊断（后端 + sqlite SQL 提示）推入 CodeMirror
// 的 lint 状态。
watch(
  () => props.errors,
  () => updateDiagnostics(),
  { immediate: true },
);

defineExpose({
  focus: () => view.value?.focus(),
});
</script>

<style scoped>
.rego-editor-wrap {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--zk-space-2);
}

/* 顶栏：样例选择器 + 全屏/参考按钮，与编辑器同框一体 */
.rego-toolbar {
  display: flex;
  align-items: center;
  gap: var(--zk-space-2);
}

.toolbar-btn {
  flex-shrink: 0;
}

.sample-selector {
  flex: 1;
  min-width: 0;
}

.rego-body {
  display: flex;
  gap: var(--zk-space-2);
  min-width: 0;
  flex: 1;
}

.rego-editor {
  flex: 1 1 0%;
  min-width: 0;
}

/* 全屏模式：编辑器整屏覆盖（fixed 铺满视口），主体占据剩余高度，
   参考栏在其右侧以并排分栏出现，宽度充足不与编辑器互相挤压 */
.rego-editor-wrap.is-fullscreen {
  position: fixed;
  inset: 0;
  z-index: var(--zk-z-fullscreen);
  padding: var(--zk-space-4);
}

.is-fullscreen .rego-editor {
  block-size: 100%;
}

.is-fullscreen .rego-editor :deep(.cm-editor),
.is-fullscreen .rego-editor :deep(.cm-scroller) {
  block-size: 100%;
}

/* 全屏主体改用 grid 两列，列高严格受视口约束，参考栏超高时能内部滚动 */
.is-fullscreen .rego-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 340px;
  gap: var(--zk-space-2);
  align-items: stretch;
  min-block-size: 0;
}

/* 全屏模式下的右侧参考栏：并排分栏，固定宽度 + 独立滚动（外观走 .zk-panel） */
.fullscreen-guide {
  flex-shrink: 0;
  width: 340px;
  min-block-size: 0;
  overflow-y: auto;
  padding: var(--zk-space-3);
}

.guide-enter-active,
.guide-leave-active {
  transition:
    transform var(--zk-transition-base) ease,
    opacity var(--zk-transition-base) ease;
}

.guide-enter-from,
.guide-leave-to {
  transform: translateX(100%);
  opacity: 0;
}
</style>
