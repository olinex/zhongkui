package utils

// 本文件承载三个与业务无关的文本类纯计算算法：RunRegex（正则测试与替换）、DiffJSON（JSON 差异比较）
// 与 StatsText（文本统计）。它们不依赖 eino 等上层组件，只依赖标准库；internal/ai/tools 中的同名
// 工具由此处的导出函数驱动，仅负责入参 JSON 解析与结果序列化。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串与重构前
// 保持一致。

import (
	// 标准库
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ============================================================================
// regex：正则测试与替换
// ============================================================================

// regex 支持的操作取值。
const (
	// regexOpTest 仅测试是否匹配并返回全部匹配
	regexOpTest = "test"
	// regexOpReplace 执行替换并返回替换后的文本
	regexOpReplace = "replace"
)

// RegexOperations 是 regex 操作取值空间的字符串清单，供参数 schema 的枚举与错误文案渲染复用。
var RegexOperations = []string{regexOpTest, regexOpReplace}

// RegexMatch 是 test 操作返回的单条匹配：文本、起始字节位置、捕获组与命名捕获组。
type RegexMatch struct {
	// Text 整条匹配命中的文本
	Text string `json:"text"`
	// Index 匹配在原文中的起始字节位置
	Index int `json:"index"`
	// Groups 捕获组文本（不含整条匹配，未参与匹配的组为空串；无组时为空数组）
	Groups []string `json:"groups"`
	// NamedGroups 命名捕获组名到文本的映射（无命名组时为空对象）
	NamedGroups map[string]string `json:"namedGroups"`
}

// RegexTestResult 是 regex 在 operation 为 test 时返回的载荷。
type RegexTestResult struct {
	// Operation 恒为 test
	Operation string `json:"operation"`
	// Matched 是否至少命中一次
	Matched bool `json:"matched"`
	// Count 命中次数
	Count int `json:"count"`
	// Matches 全部命中（无命中时为空数组而非 null）
	Matches []RegexMatch `json:"matches"`
}

// RegexReplaceResult 是 regex 在 operation 为 replace 时返回的载荷。
type RegexReplaceResult struct {
	// Operation 恒为 replace
	Operation string `json:"operation"`
	// Result 替换后的文本
	Result string `json:"result"`
	// Count 实际执行的替换次数
	Count int `json:"count"`
}

// RunRegex 执行 regex 工具：校验 operation / pattern / replacement，编译正则后按操作类型返回
// RegexTestResult（test）或 RegexReplaceResult（replace）。operation 非法、pattern / replacement
// 为空或正则编译失败时返回英文错误（工具名前缀由适配器补齐）。
func RunRegex(operation, pattern, text, replacement string, global bool) (any, error) {
	op := strings.TrimSpace(operation)
	switch op {
	case regexOpTest, regexOpReplace:
	default:
		return nil, errors.Newf("invalid operation %q, expected one of %s",
			operation, strings.Join(RegexOperations, ", "))
	}
	if pattern == "" {
		return nil, errors.New("pattern must be non-empty")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if op == regexOpReplace {
		if replacement == "" {
			return nil, errors.New("replacement must be non-empty when operation is replace")
		}
		return regexRunReplace(re, text, replacement, global)
	}
	return regexRunTest(re, text)
}

// regexRunTest 执行 test 操作：遍历全部命中构造匹配列表并返回。无命中时返回空数组而非 null。
func regexRunTest(re *regexp.Regexp, text string) (RegexTestResult, error) {
	names := re.SubexpNames()
	matches := make([]RegexMatch, 0)
	for _, idx := range re.FindAllStringSubmatchIndex(text, -1) {
		groups := make([]string, 0, len(idx)/2-1)
		named := make(map[string]string)
		for i := 1; i < len(idx)/2; i++ {
			value := ""
			if idx[2*i] >= 0 {
				value = text[idx[2*i]:idx[2*i+1]]
			}
			groups = append(groups, value)
			if i < len(names) && names[i] != "" {
				named[names[i]] = value
			}
		}
		matches = append(matches, RegexMatch{
			Text:        text[idx[0]:idx[1]],
			Index:       idx[0],
			Groups:      groups,
			NamedGroups: named,
		})
	}
	return RegexTestResult{
		Operation: regexOpTest,
		Matched:   len(matches) > 0,
		Count:     len(matches),
		Matches:   matches,
	}, nil
}

// regexRunReplace 执行 replace 操作：global 为 true 时全部替换，否则仅替换首个；返回替换后的文本与
// 实际替换次数。替换表达式沿用 Go 的展开语义（$1 等引用捕获组）。
func regexRunReplace(re *regexp.Regexp, text, replacement string, global bool) (RegexReplaceResult, error) {
	result := text
	count := 0
	if global {
		count = len(re.FindAllStringIndex(text, -1))
		result = re.ReplaceAllString(text, replacement)
	} else if loc := re.FindStringSubmatchIndex(text); loc != nil {
		result = text[:loc[0]] + string(re.ExpandString(nil, replacement, text, loc)) + text[loc[1]:]
		count = 1
	}
	return RegexReplaceResult{
		Operation: regexOpReplace,
		Result:    result,
		Count:     count,
	}, nil
}

// ============================================================================
// json_diff：JSON 差异比较
// ============================================================================

// json_diff 的差异类型取值。
const (
	// jsonDiffAdded right 有而 left 无
	jsonDiffAdded = "added"
	// jsonDiffRemoved left 有而 right 无
	jsonDiffRemoved = "removed"
	// jsonDiffChanged 两侧都有但值不等
	jsonDiffChanged = "changed"
)

// JsonDiffEntry 是单条差异：路径、类型与两侧取值。added 的左值与 removed 的右值为 null。
type JsonDiffEntry struct {
	// Path 差异路径：对象键用 .key，数组下标用 [i]，根路径为空串
	Path string `json:"path"`
	// Kind 差异类型：added / removed / changed
	Kind string `json:"kind"`
	// Left 左侧取值（removed / changed 时有效）
	Left interface{} `json:"left"`
	// Right 右侧取值（added / changed 时有效）
	Right interface{} `json:"right"`
}

// JsonDiffResult 是 json_diff 返回的载荷：differences 按路径确定性排列（无差异时为空数组）。
type JsonDiffResult struct {
	// Equal 两侧是否完全相等
	Equal bool `json:"equal"`
	// Count 差异条数
	Count int `json:"count"`
	// Differences 差异列表（无差异时为空数组而非 null）
	Differences []JsonDiffEntry `json:"differences"`
}

// DiffJSON 比较两段 JSON 文本的结构化差异：对象按键比较（缺失 / 新增 / 值变），数组按下标比较（长度
// 差视为新增 / 删除），标量直接比较。任一侧非法 JSON 时返回英文错误（工具名前缀由适配器补齐）。
func DiffJSON(left, right string) (JsonDiffResult, error) {
	leftValue, err := jsonDiffDecodeSide(left, "left")
	if err != nil {
		return JsonDiffResult{}, errors.WithStack(err)
	}
	rightValue, err := jsonDiffDecodeSide(right, "right")
	if err != nil {
		return JsonDiffResult{}, errors.WithStack(err)
	}
	diffs := make([]JsonDiffEntry, 0)
	jsonDiffValues(leftValue, rightValue, "", &diffs)
	return JsonDiffResult{
		Equal:       len(diffs) == 0,
		Count:       len(diffs),
		Differences: diffs,
	}, nil
}

// jsonDiffDecodeSide 把一个入参侧的 JSON 文本解码为通用值；非法 JSON 时返回英文错误并指明是哪一侧。
func jsonDiffDecodeSide(text, side string) (interface{}, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, errors.Newf("%s is not valid JSON: %v", side, err)
	}
	return v, nil
}

// jsonDiffValues 递归比较两侧取值并把差异追加到 out：两侧同为对象则按键比较，同为数组则按下标比较，
// 否则视为标量直接比较（相等则无差异，不等记为 changed）。path 为当前路径，根路径为空串。
func jsonDiffValues(left, right interface{}, path string, out *[]JsonDiffEntry) {
	leftObj, leftIsObj := left.(map[string]interface{})
	rightObj, rightIsObj := right.(map[string]interface{})
	if leftIsObj && rightIsObj {
		jsonDiffObjects(leftObj, rightObj, path, out)
		return
	}
	leftArr, leftIsArr := left.([]interface{})
	rightArr, rightIsArr := right.([]interface{})
	if leftIsArr && rightIsArr {
		jsonDiffArrays(leftArr, rightArr, path, out)
		return
	}
	if !reflect.DeepEqual(left, right) {
		*out = append(*out, JsonDiffEntry{Path: path, Kind: jsonDiffChanged, Left: left, Right: right})
	}
}

// jsonDiffObjects 按键比较两个对象：两侧都有的键递归比较，仅左侧有的记为 removed，仅右侧有的记为
// added。键排序后遍历以保证差异顺序的确定性。
func jsonDiffObjects(left, right map[string]interface{}, path string, out *[]JsonDiffEntry) {
	seen := make(map[string]struct{}, len(left)+len(right))
	keys := make([]string, 0, len(left)+len(right))
	for k := range left {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	for k := range right {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		leftValue, leftOK := left[k]
		rightValue, rightOK := right[k]
		child := jsonDiffJoinPath(path, k)
		switch {
		case leftOK && rightOK:
			jsonDiffValues(leftValue, rightValue, child, out)
		case leftOK:
			*out = append(*out, JsonDiffEntry{Path: child, Kind: jsonDiffRemoved, Left: leftValue})
		default:
			*out = append(*out, JsonDiffEntry{Path: child, Kind: jsonDiffAdded, Right: rightValue})
		}
	}
}

// jsonDiffArrays 按下标比较两个数组：重合下标递归比较，左侧多出的下标记为 removed，右侧多出的下标
// 记为 added。
func jsonDiffArrays(left, right []interface{}, path string, out *[]JsonDiffEntry) {
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		child := fmt.Sprintf("%s[%d]", path, i)
		switch {
		case i < len(left) && i < len(right):
			jsonDiffValues(left[i], right[i], child, out)
		case i < len(left):
			*out = append(*out, JsonDiffEntry{Path: child, Kind: jsonDiffRemoved, Left: left[i]})
		default:
			*out = append(*out, JsonDiffEntry{Path: child, Kind: jsonDiffAdded, Right: right[i]})
		}
	}
}

// jsonDiffJoinPath 在对象路径后追加一个键：根路径为空串时直接用键名，否则以 . 连接（如 a.b）。
func jsonDiffJoinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// ============================================================================
// text_stats：文本统计
// ============================================================================

// TextStatsResult 是 text_stats 返回的载荷，覆盖字符、单词、行与段落的统计。
type TextStatsResult struct {
	// Characters rune 数（字符数）
	Characters int `json:"characters"`
	// CharactersNoWhitespace 去除空白后的 rune 数
	CharactersNoWhitespace int `json:"charactersNoWhitespace"`
	// Bytes UTF-8 字节数
	Bytes int `json:"bytes"`
	// Words 按空白切分的词数
	Words int `json:"words"`
	// Lines 按 \n 切分的行数（空文本为 0）
	Lines int `json:"lines"`
	// Paragraphs 以连续空行分隔的非空块数
	Paragraphs int `json:"paragraphs"`
}

// StatsText 统计一段文本的字符数、去空白字符数、UTF-8 字节数、词数、行数与段落数。字符数按 Unicode
// 码点（rune）计，行数按 \n 计，段落按连续空行分隔的非空块计；空文本各项均为 0。
func StatsText(text string) (TextStatsResult, error) {
	return TextStatsResult{
		Characters:             utf8.RuneCountInString(text),
		CharactersNoWhitespace: textCountNonWhitespaceRunes(text),
		Bytes:                  len(text),
		Words:                  len(strings.Fields(text)),
		Lines:                  textCountLines(text),
		Paragraphs:             textCountParagraphs(text),
	}, nil
}

// textCountNonWhitespaceRunes 统计 s 中非空白 rune 的数量（空白判定用 unicode.IsSpace，覆盖空格、制表、
// 换行等）。
func textCountNonWhitespaceRunes(s string) int {
	n := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// textCountLines 按 \n 统计行数：空文本为 0，否则为换行符个数加一（以 \n 结尾时末尾空行为一行）。
func textCountLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// textCountParagraphs 统计段落数：以连续空行（去空白后为空的行）分隔的非空块各计一段。
func textCountParagraphs(s string) int {
	count := 0
	inBlock := false
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == "" {
			inBlock = false
			continue
		}
		if !inBlock {
			count++
			inBlock = true
		}
	}
	return count
}
