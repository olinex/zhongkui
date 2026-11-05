// Package mermaid 用 mermaid 官方解析器校验图表源码：Agent 在把带 mermaid 图表的回答交给前端前
// 先自检，出错时按官方报错的行号与修改建议整改，避免把不合法的源码发给前端后只剩一片源码文本。
//
// ## 为什么内嵌官方 bundle
//
// 官方只有 JS 实现（mermaid.parse 是唯一权威语法校验），Go 侧没有等价的库；社区纯 Go 的
// mermaid-check 仍是正则实现，且会静默跳过解析不了的行（连「-- 文本 ==>」都识别不到），
// 不能作为校验器。本包因此把官方 bundle 注册进进程内的 JS 运行时（internal/pkg/jsengine，
// 由 provider 统一管理生命周期），在单二进制内部完成校验，不依赖 Node / Chromium，也不需要
// 我们自行维护语法规则。
//
// assets/mermaid.min.js 是官方发行包（mermaid 12.1.0，MIT），必须与 web 侧依赖（
// web/package.json 的 mermaid 版本）保持一致：升级前端 mermaid 时同步替换该文件，否则校验
// 口径会与前端渲染分叉。
//
// ## 与官方解析的差异
//
// 解析前会做两步无损预处理：剥离可选的 ```mermaid 围栏（用等长空白替换，行号不漂移）、把
// 标签内 HTML 标签（如 <br/>）等长替换为空格——两者的目的都是绕开「解析期需要真实 DOM」的
// 限制（DOMPurify 在 goja 的无 DOM 环境中不可用），实际渲染仍使用原文。
package mermaid

import (
	// 标准库
	_ "embed"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/jsengine"
)

// officialBundle 是官方 mermaid 发行包，见包注释中的版本约定。
//
//go:embed assets/mermaid.min.js
var officialBundle []byte

// Issue 是单条校验问题：行号从 1 开始；列号官方未给出，0 表示未知。
type Issue struct {
	// Line 问题所在行号
	Line int `json:"line"`
	// Column 问题所在列号（0 表示未知）
	Column int `json:"column,omitempty"`
	// Message 官方解析器给出的问题描述
	Message string `json:"message"`
	// Hint 修改建议（可选，按已知模式补充中文提示）
	Hint string `json:"hint,omitempty"`
}

// CheckResult 是 mermaid_check 的返回结果。
type CheckResult struct {
	// DiagramType 官方解析器识别出的图表类型（如 flowchart-v2、sequence）
	DiagramType string `json:"diagramType"`
	// Valid 源码是否通过官方语法校验
	Valid bool `json:"valid"`
	// Issues 问题清单；Valid 为 true 时为空
	Issues []Issue `json:"issues"`
}

// stubScript 是 mermaid 解析期需要的最小浏览器环境：mermaid.parse 只访问少量全局对象
// （navigator / 文档 / 动画帧），用 Proxy 兜底即可；DOMPurify 依赖的部分（HTML 标签）由
// 源码预处理绕开。浏览器桩归属于 mermaid 这个使用方，不放进通用 JS 引擎。
const stubScript = `(function () {
	var handler = {
		get: function (target, prop) {
			if (prop === Symbol.toPrimitive) return function () { return ""; };
			if (prop === "toString") return function () { return ""; };
			if (prop === "nodeType") return 9;
			if (prop === "length") return 0;
			return dummy;
		},
		apply: function () { return dummy; },
		construct: function () { return dummy; },
		set: function () { return true; },
		has: function () { return true; }
	};
	var dummy;
	dummy = new Proxy(function () {}, handler);
	var globalObject = globalThis;
	globalObject.window = globalObject;
	globalObject.self = globalObject;
	globalObject.document = dummy;
	globalObject.DOMParser = dummy;
	globalObject.MutationObserver = dummy;
	globalObject.HTMLElement = dummy;
	globalObject.Element = dummy;
	globalObject.Node = dummy;
	globalObject.requestAnimationFrame = function () { return 0; };
	globalObject.cancelAnimationFrame = function () {};
	globalObject.navigator = { userAgent: "node" };
	globalObject.location = { href: "https://example.com/" };
	globalObject.structuredClone = function (value) { return JSON.parse(JSON.stringify(value)); };
	globalObject.console = { log: function () {}, info: function () {}, warn: function () {}, error: function () {}, debug: function () {} };
	globalObject.addEventListener = function () {};
	globalObject.removeEventListener = function () {};
	globalObject.dispatchEvent = function () { return true; };
	globalObject.matchMedia = function () { return {}; };
	globalObject.NodeFilter = {
		SHOW_ELEMENT: 1, SHOW_ATTRIBUTE: 2, SHOW_TEXT: 4, SHOW_CDATA_SECTION: 8,
		SHOW_ENTITY_REFERENCE: 16, SHOW_ENTITY: 32, SHOW_PROCESSING_INSTRUCTION: 64,
		SHOW_COMMENT: 128, SHOW_DOCUMENT: 256, SHOW_DOCUMENT_TYPE: 512,
		SHOW_DOCUMENT_FRAGMENT: 1024, SHOW_NOTATION: 2048,
		FILTER_ACCEPT: 1, FILTER_REJECT: 2, FILTER_SKIP: 3
	};
})();`

// parseScript 调用官方解析并在 Promise 落定后写回 __out：成功写 diagramType，失败写官方错误文本。
const parseScript = `(function () {
	__out = "pending";
	try {
		mermaid.parse(__src).then(
			function (result) {
				__out = JSON.stringify({ ok: true, diagramType: (result && result.diagramType) || "" });
			},
			function (error) {
				__out = JSON.stringify({ ok: false, message: String((error && error.message) || error) });
			}
		);
	} catch (error) {
		__out = JSON.stringify({ ok: false, message: String((error && error.message) || error) });
	}
})();`

// parseOutcome 是 JS 侧回写的校验结果。
type parseOutcome struct {
	OK          bool   `json:"ok"`
	DiagramType string `json:"diagramType"`
	Message     string `json:"message"`
}

// errParseTimeout 表示 JS 侧未在预期时间内落定 Promise。
var errParseTimeout = errors.New("mermaid: parse did not settle in time")

// fencePattern 匹配 markdown 围栏起始/结束行。
var fencePattern = regexp.MustCompile("^(```|~~~)")

// htmlTagPattern 匹配标签内的 HTML 标签（如 <br/>、<b>、</b>）：只用于「等长替换」，
// 不改变行号与列号。
var htmlTagPattern = regexp.MustCompile(`<[A-Za-z/][^<>]*>`)

// linePattern 从官方错误文本中取行号（形如 "Parse error on line 4:"）。
var linePattern = regexp.MustCompile(`on line (\d+)`)

// labelMismatchHints 是已知的连线标签/箭头家族不匹配写法：官方报错文本偏开发者口径，
// 这里补一条可直接执行的修改建议。仅在官方校验失败时使用，不参与合法性判定。
var labelMismatchHints = []struct {
	pattern *regexp.Regexp
	hint    string
}{
	{regexp.MustCompile(`--\s*[^\n<>]+\s*==>`), "带文字的粗箭头应写作 `== 文本 ==>`，当前 `-- 文本 ==>` 不合法。"},
	{regexp.MustCompile(`==\s*[^\n=<>]+\s*-->`), "带文字的普通箭头应写作 `-- 文本 -->`，当前 `== 文本 -->` 不合法。"},
	{regexp.MustCompile(`-\.\s*[^\n.<>]+\s*-->`), "带文字的普通箭头应写作 `-- 文本 -->`，当前 `-. 文本 -->` 不合法。"},
	{regexp.MustCompile(`--\s*[^\n.<>]+\s*\.->`), "带文字的虚线箭头应写作 `-. 文本 .->`，当前 `-- 文本 .->` 不合法。"},
}

// init 把 mermaid bundle 注册进共享 JS 引擎：浏览器桩在前，官方 bundle 在后；引擎启动时
// 依次执行（见 internal/pkg/jsengine）。mermaid 只是 JS 引擎的一个使用方。
func init() {
	jsengine.Register(jsengine.Registrar{
		Name: "mermaid",
		Setup: func(vm *jsengine.Runtime) error {
			if _, err := vm.RunString(stubScript); err != nil {
				return err
			}
			if _, err := vm.RunString(string(officialBundle)); err != nil {
				return err
			}
			if vm.Get("mermaid") == nil {
				return errors.New("mermaid: official bundle did not expose the global mermaid object")
			}
			return nil
		},
	})
}

// Check 用官方 mermaid 解析器校验一段源码：合法时返回识别到的图表类型，不合法时返回官方报错
// 与中文修改建议。engine 为 provider 管理的 JS 运行时；为空或未启动时返回可定位的 Issue。
func Check(engine *jsengine.Engine, source string) CheckResult {
	if engine == nil {
		return CheckResult{Valid: false, Issues: []Issue{{
			Message: "mermaid 校验不可用：未注入 JS 引擎",
			Hint:    "检查 provider 是否注册并启动 jsengine（internal/provider/jsengine.go）。",
		}}}
	}

	normalized, ok := normalizeSource(source)
	if !ok {
		return CheckResult{Valid: false, Issues: []Issue{{
			Line:    1,
			Message: "源码为空或未找到可校验的图表内容",
			Hint:    "传入一段 mermaid 源码（可带 ```mermaid 围栏），一次只传一个图表。",
		}}}
	}

	var outcome parseOutcome
	runErr := engine.Do(func(vm *jsengine.Runtime) error {
		if err := vm.Set("__src", normalized); err != nil {
			return err
		}
		if err := vm.Set("__out", "pending"); err != nil {
			return err
		}
		if _, err := vm.RunString(parseScript); err != nil {
			return err
		}
		if !waitOutcome(vm, &outcome) {
			return errParseTimeout
		}
		return nil
	})
	if runErr != nil {
		return CheckResult{Valid: false, Issues: []Issue{{
			Message: "mermaid 校验运行时不可用：" + runErr.Error(),
			Hint:    "确认 JS 引擎已启动且内嵌的 assets/mermaid.min.js 完整。",
		}}}
	}
	if outcome.OK {
		return CheckResult{DiagramType: outcome.DiagramType, Valid: true}
	}
	return CheckResult{Issues: []Issue{issueFor(normalized, outcome.Message)}}
}

// waitOutcome 等待 JS 侧把 __out 从 "pending" 改写为结果：goja 在 RunString 结束时排空 Promise
// 微任务，此处再多泵几轮以覆盖异步落定，避免竞态。
func waitOutcome(rt *jsengine.Runtime, outcome *parseOutcome) bool {
	for attempt := 0; attempt < 5; attempt++ {
		value := rt.Get("__out")
		if value == nil || jsengine.IsUndefined(value) {
			return false
		}
		text := value.String()
		if text != "pending" {
			return json.Unmarshal([]byte(text), outcome) == nil
		}
		// 触发一次空执行让 goja 调度挂起的微任务
		if _, err := rt.RunString(""); err != nil {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// issueFor 把官方错误文本转换为结构化问题：行号取自 "on line N"。mermaid 12.1.0 的报错行号
// 比源码实际行号大 1（解析前会补一行），此处校正并把官方文本中的行号一并改写，避免模型看到
// 两套行号。
func issueFor(source, message string) Issue {
	message = strings.TrimSpace(message)
	issue := Issue{Message: message}
	if match := linePattern.FindStringSubmatch(message); len(match) == 2 {
		if reported, err := strconv.Atoi(match[1]); err == nil && reported > 1 {
			line := reported - 1
			issue.Line = line
			message = strings.Replace(message, "on line "+match[1], "on line "+strconv.Itoa(line), 1)
		}
	}
	issue.Message = message
	issue.Hint = hintFor(source, message)
	return issue
}

// hintFor 按官方错误文本与源码给出可直接执行的修改建议。
func hintFor(source, message string) string {
	if strings.Contains(message, "No diagram type detected") || strings.Contains(message, "UnknownDiagramError") {
		return "首行需声明图表类型（如 `flowchart TD`、`sequenceDiagram`）；一次只校验一个图表。"
	}
	for _, candidate := range labelMismatchHints {
		if candidate.pattern.MatchString(source) {
			return candidate.hint
		}
	}
	if strings.Contains(message, "Expecting 'LINK'") || strings.Contains(message, "EDGE_TEXT") {
		return "该行连线写法不合法：普通箭头 `-- 文本 -->`、粗箭头 `== 文本 ==>`、虚线 `-. 文本 .->`。"
	}
	return ""
}

// normalizeSource 做解析前的无损预处理并返回是否还有可校验内容：
//   - 统一换行、去 BOM；
//   - 把 ```mermaid / ~~~ 围栏行与标签内 HTML 标签替换为等长空格，保持行号/列号不漂移。
func normalizeSource(source string) (string, bool) {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.TrimPrefix(normalized, "\ufeff")
	lines := strings.Split(normalized, "\n")
	for index, line := range lines {
		if fencePattern.MatchString(strings.TrimSpace(line)) {
			lines[index] = strings.Repeat(" ", len(line))
		}
	}
	normalized = strings.Join(lines, "\n")
	normalized = htmlTagPattern.ReplaceAllStringFunc(normalized, func(tag string) string {
		return strings.Repeat(" ", len(tag))
	})
	return normalized, strings.TrimSpace(normalized) != ""
}
