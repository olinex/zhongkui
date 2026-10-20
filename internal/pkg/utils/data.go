package utils

// 本文件承载三个与业务无关的数据类纯计算能力：ProcessJSON（JSON 校验 / 格式化 / 压缩 / 路径查询）、
// ParseCSV（CSV 文本解析）与 ParseURL（URL 拆解）。它们不依赖 eino 等上层组件，只依赖标准库；
// internal/ai/tools 中的同名工具由此处的导出函数驱动，仅负责入参 JSON 解析与结果序列化。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串与重构前
// 保持一致。

import (
	// 标准库
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ============================================================================
// ProcessJSON：JSON 处理
// ============================================================================

// json 工具的缩进参数：缺省 2，合法区间 1..8。
const (
	// DefaultJSONIndent 是 format 操作 indent 缺省值。
	DefaultJSONIndent = 2
	jsonMinIndent     = 1
	jsonMaxIndent     = 8
)

// JSONOperations 是 json 工具操作取值空间的字符串清单（字典序固定），供参数 schema 的枚举与错误
// 文案渲染复用。
var JSONOperations = []string{"validate", "format", "minify", "query"}

// JSONValidateResult 是 json 工具 validate 操作返回的载荷：合法时 error 省略。
type JSONValidateResult struct {
	// Operation 实际执行的操作（恒为 validate）
	Operation string `json:"operation"`
	// Valid 文本是否为合法 JSON
	Valid bool `json:"valid"`
	// Error 非法时的错误文本（合法时省略）
	Error string `json:"error,omitempty"`
}

// JSONTextResult 是 json 工具 format / minify 操作返回的载荷。
type JSONTextResult struct {
	// Operation 实际执行的操作（format 或 minify）
	Operation string `json:"operation"`
	// Result 处理后的 JSON 文本
	Result string `json:"result"`
}

// JSONQueryResult 是 json 工具 query 操作返回的载荷：命中时 value 为任意 JSON 值（未命中时省略）。
// Value 用 json.RawMessage 承载，以便正确区分「未命中」（省略）与命中且值为 null / 0 / false / ""
// 等零值（仍需输出）。
type JSONQueryResult struct {
	// Operation 实际执行的操作（恒为 query）
	Operation string `json:"operation"`
	// Found 是否命中路径
	Found bool `json:"found"`
	// Value 命中路径处的值（未命中时省略）
	Value json.RawMessage `json:"value,omitempty"`
	// Path 本次查询使用的路径
	Path string `json:"path"`
}

// ProcessJSON 按 operation（validate / format / minify / query）处理 JSON 文本并返回对应结果
// （JSONValidateResult / JSONTextResult / JSONQueryResult）。operation 先去除首尾空白；操作非法、
// format 缩进越界、非法 JSON 或非法查询路径时返回英文错误（工具名前缀由适配器补齐）。indent 为 0
// 时取 DefaultJSONIndent。validate 对非法文本不视为失败，而是返回 Valid=false 与错误文本。
func ProcessJSON(operation, text, path string, indent int) (any, error) {
	operation = strings.TrimSpace(operation)
	switch operation {
	case "validate":
		raw := []byte(text)
		if json.Valid(raw) {
			return JSONValidateResult{Operation: operation, Valid: true}, nil
		}
		// json.Valid 只给判定，再解一次以取回错误文本作为提示；此乃正常返回而非工具失败
		var value any
		errText := "invalid JSON"
		if err := json.Unmarshal(raw, &value); err != nil {
			errText = err.Error()
		}
		return JSONValidateResult{Operation: operation, Valid: false, Error: errText}, nil
	case "format":
		if indent == 0 {
			indent = DefaultJSONIndent
		}
		if indent < jsonMinIndent || indent > jsonMaxIndent {
			return nil, errors.Newf("indent %d out of range, expected %d..%d",
				indent, jsonMinIndent, jsonMaxIndent)
		}
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, []byte(text)); err != nil {
			return nil, errors.Newf("invalid JSON: %v", err)
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, compacted.Bytes(), "", strings.Repeat(" ", indent)); err != nil {
			return nil, errors.WithStack(err)
		}
		return JSONTextResult{Operation: operation, Result: pretty.String()}, nil
	case "minify":
		var compacted bytes.Buffer
		if err := json.Compact(&compacted, []byte(text)); err != nil {
			return nil, errors.Newf("invalid JSON: %v", err)
		}
		return JSONTextResult{Operation: operation, Result: compacted.String()}, nil
	case "query":
		var root any
		if err := json.Unmarshal([]byte(text), &root); err != nil {
			return nil, errors.Newf("invalid JSON: %v", err)
		}
		value, found, err := jsonPathEval(root, path)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		result := JSONQueryResult{Operation: operation, Found: found, Path: path}
		if found {
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			result.Value = json.RawMessage(raw)
		}
		return result, nil
	default:
		return nil, errors.Newf("invalid operation %q, expected one of %s",
			operation, strings.Join(JSONOperations, ", "))
	}
}

// jsonPathEval 按路径从已解析的 JSON 根值求值：命中返回 (值, true)，路径合法但未命中返回
// (nil, false)，路径语法非法返回英文错误。
func jsonPathEval(root any, path string) (any, bool, error) {
	segments, err := jsonPathSegments(path)
	if err != nil {
		return nil, false, errors.WithStack(err)
	}
	current := root
	for _, segment := range segments {
		switch key := segment.(type) {
		case string:
			object, ok := current.(map[string]any)
			if !ok {
				return nil, false, nil
			}
			value, ok := object[key]
			if !ok {
				return nil, false, nil
			}
			current = value
		case int:
			array, ok := current.([]any)
			if !ok {
				return nil, false, nil
			}
			if key < 0 || key >= len(array) {
				return nil, false, nil
			}
			current = array[key]
		}
	}
	return current, true, nil
}

// jsonPathSegments 把路径解析为有序段：字符串段为对象键，整数段为数组下标。语法为对象键用点号
// 分隔、数组下标写成 [i]，如 a.b[0].c 或 [0].a。路径为空或语法非法返回英文错误。
func jsonPathSegments(path string) ([]any, error) {
	if path == "" {
		return nil, errors.New("path must be non-empty")
	}
	segments := make([]any, 0, 4)
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, errors.Newf("invalid path %q", path)
		}
		key := part
		rest := ""
		if open := strings.IndexByte(part, '['); open >= 0 {
			key = part[:open]
			rest = part[open:]
		}
		if key != "" {
			segments = append(segments, key)
		}
		for rest != "" {
			if rest[0] != '[' {
				return nil, errors.Newf("invalid path %q", path)
			}
			close := strings.IndexByte(rest, ']')
			if close < 0 {
				return nil, errors.Newf("invalid path %q", path)
			}
			index, err := strconv.Atoi(rest[1:close])
			if err != nil {
				return nil, errors.Newf("invalid path %q", path)
			}
			segments = append(segments, index)
			rest = rest[close+1:]
		}
	}
	return segments, nil
}

// ============================================================================
// ParseCSV：CSV 文本解析
// ============================================================================

// csv 的可选入参缺省值。
const (
	// DefaultCSVDelimiter 是 delimiter 缺省值（英文逗号）。
	DefaultCSVDelimiter = ","
	// DefaultCSVHasHeader 是 has_header 缺省值（首行为表头）。
	DefaultCSVHasHeader = true
)

// CSVParseResult 是 ParseCSV 成功时返回的载荷：header / rows 无数据时为空数组而非 null；rowCount
// 为数据行数（不含表头），columnCount 为列数。
type CSVParseResult struct {
	// Delimiter 实际使用的字段分隔符
	Delimiter string `json:"delimiter"`
	// HasHeader 首行是否被当作表头
	HasHeader bool `json:"hasHeader"`
	// Header 表头字段（无表头时为空数组）
	Header []string `json:"header"`
	// Rows 数据行（不含表头）
	Rows [][]string `json:"rows"`
	// RowCount 数据行数（不含表头）
	RowCount int `json:"rowCount"`
	// ColumnCount 列数
	ColumnCount int `json:"columnCount"`
}

// ParseCSV 用 delimiter（为空时取 DefaultCSVDelimiter，须为单个字符）解析 CSV 文本，按 hasHeader
// 决定首行是否作为表头，返回 CSVParseResult。分隔符非单字符或 CSV 语法非法（如引号不闭合）时返回
// 英文错误（工具名前缀由适配器补齐）。
func ParseCSV(text, delimiter string, hasHeader bool) (CSVParseResult, error) {
	if delimiter == "" {
		delimiter = DefaultCSVDelimiter
	}
	runes := []rune(delimiter)
	if len(runes) != 1 {
		return CSVParseResult{}, errors.Newf("delimiter %q must be a single character", delimiter)
	}
	delimiter = string(runes[0])
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = runes[0]
	// 容忍不定列数：宽表 / 参差行不视为错误
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return CSVParseResult{}, errors.Newf("%v", err)
	}
	header := make([]string, 0)
	rows := make([][]string, 0)
	if hasHeader {
		if len(records) > 0 {
			header = records[0]
			rows = records[1:]
		}
	} else {
		rows = records
	}
	if rows == nil {
		rows = make([][]string, 0)
	}
	columnCount := 0
	if len(header) > 0 {
		columnCount = len(header)
	} else if len(rows) > 0 {
		columnCount = len(rows[0])
	}
	return CSVParseResult{
		Delimiter:   delimiter,
		HasHeader:   hasHeader,
		Header:      header,
		Rows:        rows,
		RowCount:    len(rows),
		ColumnCount: columnCount,
	}, nil
}

// ============================================================================
// ParseURL：URL 拆解
// ============================================================================

// URLParseResult 是 ParseURL 成功时返回的载荷：query 为参数名到值的映射（无参数时为空对象而非
// null）。
type URLParseResult struct {
	// Scheme 协议方案
	Scheme string `json:"scheme"`
	// User 用户名（无则空串）
	User string `json:"user"`
	// Host 主机名（不含端口）
	Host string `json:"host"`
	// Port 端口（无则空串）
	Port string `json:"port"`
	// Path 路径
	Path string `json:"path"`
	// Fragment 片段标识（# 之后的部分）
	Fragment string `json:"fragment"`
	// Query 查询参数（参数名到值列表的映射）
	Query map[string][]string `json:"query"`
	// QueryString 原始查询串（未解码）
	QueryString string `json:"queryString"`
	// IsAbsolute 是否为绝对 URL（带 scheme）
	IsAbsolute bool `json:"isAbsolute"`
}

// ParseURL 用标准库 net/url 把 raw 拆解为结构化字段并返回 URLParseResult。raw 去空白后为空或非法
// 时返回英文错误（工具名前缀由适配器补齐）。
func ParseURL(raw string) (URLParseResult, error) {
	if strings.TrimSpace(raw) == "" {
		return URLParseResult{}, errors.New("url must be non-empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return URLParseResult{}, errors.Newf("invalid URL: %v", err)
	}
	user := ""
	if parsed.User != nil {
		user = parsed.User.Username()
	}
	query := parsed.Query()
	if query == nil {
		query = map[string][]string{}
	}
	return URLParseResult{
		Scheme:      parsed.Scheme,
		User:        user,
		Host:        parsed.Hostname(),
		Port:        parsed.Port(),
		Path:        parsed.Path,
		Fragment:    parsed.Fragment,
		Query:       query,
		QueryString: parsed.RawQuery,
		IsAbsolute:  parsed.IsAbs(),
	}, nil
}
