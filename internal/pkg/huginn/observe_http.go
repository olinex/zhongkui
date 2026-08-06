// HTTP/1.x 观测值提取：huginn-net-http 的 http1/parser、http1/process 与
// http/languages 模块的纯 Go 移植。
//
// 输入为完整的 HTTP/1.x 请求或响应头字节，输出为 p0f 风格的 HTTP 观测结构
// （版本、头顺序、缺失头、期望子串）以及请求的语言偏好。
package huginn

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// HTTP/1.x 解析失败的错误哨兵（对应 Http1ParseError 的各变体）。
var (
	// ErrInvalidUTF8 HTTP 数据不是合法 UTF-8。
	ErrInvalidUTF8 = errors.New("huginn: invalid UTF-8 in HTTP data")
	// ErrIncompleteData HTTP 数据缺少完整头结束标记。
	ErrIncompleteData = errors.New("huginn: incomplete HTTP data")
	// ErrInvalidHTTPRequestLine 请求行非法。
	ErrInvalidHTTPRequestLine = errors.New("huginn: invalid HTTP request line")
	// ErrInvalidHTTPStatusLine 状态行非法。
	ErrInvalidHTTPStatusLine = errors.New("huginn: invalid HTTP status line")
	// ErrInvalidHTTPVersion HTTP 版本非法（仅接受 HTTP/1.0 与 HTTP/1.1）。
	ErrInvalidHTTPVersion = errors.New("huginn: invalid HTTP version")
	// ErrInvalidHTTPMethod HTTP 方法不在白名单中。
	ErrInvalidHTTPMethod = errors.New("huginn: invalid HTTP method")
)

// httpHeaderRaw 解析出的原始 HTTP 头（对应 http::common::HttpHeader 的简化形式）。
type httpHeaderRaw struct {
	name  string
	value *string
}

// httpRequestObservation HTTP/1.x 请求的观测结果：匹配用结构与语言偏好。
type httpRequestObservation struct {
	matching httpObservation
	lang     string
}

// httpResponseObservation HTTP/1.x 响应的观测结果：匹配用结构。
type httpResponseObservation struct {
	matching httpObservation
}

// validHTTPMethods HTTP/1.x 方法白名单（对应 parser.rs is_valid_method）。
var validHTTPMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "DELETE": true, "HEAD": true,
	"OPTIONS": true, "PATCH": true, "TRACE": true, "CONNECT": true,
	"PROPFIND": true, "PROPPATCH": true, "MKCOL": true, "COPY": true,
	"MOVE": true, "LOCK": true, "UNLOCK": true, "MKCALENDAR": true, "REPORT": true,
}

// requestOptionalHeaders 请求头中"存在则匹配、缺失则忽略"的可选头（对应 request_optional_headers）。
var requestOptionalHeaders = []string{
	"Cookie", "Referer", "Origin", "Range", "If-Modified-Since", "If-None-Match",
	"Via", "X-Forwarded-For", "Authorization", "Proxy-Authorization", "Cache-Control",
}

// responseOptionalHeaders 响应头中的可选头（对应 response_optional_headers）。
var responseOptionalHeaders = []string{
	"Set-Cookie", "Last-Modified", "ETag", "Content-Length", "Content-Disposition",
	"Cache-Control", "Expires", "Pragma", "Location", "Refresh", "Content-Range", "Vary",
}

// requestSkipValueHeaders 请求头中忽略值的头（对应 request_skip_value_headers）。
var requestSkipValueHeaders = []string{"Host", "User-Agent"}

// responseSkipValueHeaders 响应头中忽略值的头（对应 response_skip_value_headers）。
var responseSkipValueHeaders = []string{"Date", "Content-Type", "Server"}

// requestCommonHeaders 请求"常见头"列表，用于构建缺失头集合（对应 request_common_headers）。
var requestCommonHeaders = []string{
	"Host", "User-Agent", "Connection", "Accept", "Accept-Encoding",
	"Accept-Language", "Accept-Charset", "Keep-Alive",
}

// responseCommonHeaders 响应"常见头"列表（对应 response_common_headers）。
var responseCommonHeaders = []string{"Content-Type", "Connection", "Keep-Alive", "Accept-Ranges", "Date"}

// httpLangNames 语言代码 → 英文名映射（对应 languages.rs 的 LANGUAGES 表）。
var httpLangNames = map[string]string{
	"ro": "Romanian", "sw": "Swahili", "ne": "Nepali", "nl": "Dutch", "sn": "Shona",
	"ln": "Lingala", "en": "English", "ie": "Interlingue", "bg": "Bulgarian",
	"ha": "Hausa", "cs": "Czech", "ko": "Korean", "gv": "Manx", "vi": "Vietnamese",
	"mt": "Maltese", "bo": "Tibetan", "de": "German", "pa": "Panjabi", "lg": "Ganda",
	"tk": "Turkmen", "gl": "Galician", "yo": "Yoruba", "sc": "Sardinian", "or": "Oriya",
	"fr": "French", "ae": "Avestan", "am": "Amharic", "mh": "Marshallese",
	"hr": "Croatian", "sg": "Sango", "ps": "Pushto", "to": "Tonga", "kj": "Kuanyama",
	"kv": "Komi", "li": "Limburgan", "ng": "Ndonga", "lu": "Luba-Katanga",
	"nn": "Norwegian Nynorsk", "es": "Spanish", "gn": "Guarani", "pl": "Polish",
	"om": "Oromo", "lb": "Luxembourgish", "se": "Northern Sami", "ab": "Abkhazian",
	"ar": "Arabic", "az": "Azerbaijani", "si": "Sinhala", "ba": "Bashkir",
	"sr": "Serbian", "vo": "Volapuk", "kl": "Kalaallisut", "th": "Thai",
	"cu": "Church Slavic", "ja": "Japanese", "fy": "Western Frisian", "ch": "Chamorro",
	"hy": "Armenian", "ht": "Haitian", "fo": "Faroese", "fj": "Fijian",
	"gd": "Scottish Gaelic", "ig": "Igbo", "is": "Icelandic", "bi": "Bislama",
	"za": "Zhuang", "eu": "Basque", "id": "Indonesian", "ks": "Kashmiri",
	"cr": "Cree", "ga": "Irish", "gu": "Gujarati", "st": "Southern Sotho",
	"ur": "Urdu", "ce": "Chechen", "kg": "Kongo", "he": "Hebrew", "dv": "Dhivehi",
	"ru": "Russian", "ts": "Tsonga", "bn": "Bengali", "sv": "Swedish",
	"ug": "Uighur", "bs": "Bosnian", "wa": "Walloon", "ho": "Hiri Motu",
	"ii": "Sichuan Yi", "sk": "Slovak", "nb": "Norwegian Bokmal", "co": "Corsican",
	"lt": "Lithuanian", "ms": "Malay", "da": "Danish", "ny": "Nyanja",
	"ik": "Inupiaq", "iu": "Inuktitut", "sd": "Sindhi", "rw": "Kinyarwanda",
	"ki": "Kikuyu", "uk": "Ukrainian", "la": "Latin", "nr": "South Ndebele",
	"oc": "Occitan", "ml": "Malayalam", "ku": "Kurdish", "rn": "Rundi",
	"kn": "Kannada", "ta": "Tamil", "pi": "Pali", "sm": "Samoan", "tw": "Twi",
	"nd": "North Ndebele", "oj": "Ojibwa", "tl": "Tagalog", "aa": "Afar",
	"ay": "Aymara", "te": "Telugu", "eo": "Esperanto", "ia": "Interlingua",
	"xh": "Xhosa", "jv": "Javanese", "ty": "Tahitian", "os": "Ossetian",
	"et": "Estonian", "cy": "Welsh", "so": "Somali", "sq": "Albanian",
	"pt": "Portuguese", "tn": "Tswana", "zu": "Zulu", "bh": "Bihari",
	"mn": "Mongolian", "uz": "Uzbek", "lo": "Lao", "ee": "Ewe", "mg": "Malagasy",
	"lv": "Latvian", "fi": "Finnish", "af": "Afrikaans", "an": "Aragonese",
	"av": "Avaric", "hi": "Hindi", "ff": "Fulah", "nv": "Navajo", "fa": "Persian",
	"yi": "Yiddish", "kw": "Cornish", "tg": "Tajik", "be": "Belarusian",
	"na": "Nauru", "qu": "Quechua", "sh": "Serbo-Croatian", "dz": "Dzongkha",
	"kk": "Kazakh", "cv": "Chuvash", "kr": "Kanuri", "br": "Breton", "bm": "Bambara",
	"ss": "Swati", "tr": "Turkish", "mi": "Maori", "no": "Norwegian", "ak": "Akan",
	"as": "Assamese", "it": "Italian", "ca": "Catalan", "km": "Central Khmer",
	"mk": "Macedonian", "tt": "Tatar", "rm": "Romansh", "io": "Ido",
	"sl": "Slovenian", "hz": "Herero", "ka": "Georgian", "ky": "Kirghiz",
	"ve": "Venda", "el": "Modern Greek", "sa": "Sanskrit", "wo": "Wolof",
	"mr": "Marathi", "zh": "Chinese", "su": "Sundanese", "my": "Burmese",
	"hu": "Hungarian", "ti": "Tigrinya",
}

// splitHTTPLines 按数据中的行分隔符拆分（对应 parser.rs 的 \r\n / \n 二选一逻辑）。
func splitHTTPLines(data string) []string {
	if strings.Contains(data, "\r\n") {
		return strings.Split(data, "\r\n")
	}
	return strings.Split(data, "\n")
}

// parseHTTPHeaders 解析头行列表（对应 parser.rs parse_headers 的非严格模式）。
// 无冒号或名字为空的行被跳过；名字与值均去除首尾空白。
func parseHTTPHeaders(lines []string) []httpHeaderRaw {
	headers := make([]httpHeaderRaw, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			break
		}
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		if name == "" {
			continue
		}
		value := strings.TrimSpace(line[colon+1:])
		headers = append(headers, httpHeaderRaw{name: name, value: &value})
	}
	return headers
}

// parseHTTPRequest 解析 HTTP/1.x 请求（对应 parser.rs parse_request）。
// 返回 nil 表示数据不完整（Ok(None)）。
func parseHTTPRequest(data []byte) (*httpRequestObservation, error) {
	if !utf8.Valid(data) {
		return nil, ErrInvalidUTF8
	}
	str := string(data)
	if !strings.Contains(str, "\r\n\r\n") && !strings.Contains(str, "\n\n") {
		return nil, ErrIncompleteData
	}
	lines := splitHTTPLines(str)
	if len(lines) == 0 {
		return nil, ErrIncompleteData
	}

	parts := strings.Fields(lines[0])
	if len(parts) != 3 {
		return nil, ErrInvalidHTTPRequestLine
	}
	method, uri, versionStr := parts[0], parts[1], parts[2]
	if !validHTTPMethods[method] {
		return nil, ErrInvalidHTTPMethod
	}
	version, ok := parseHTTPVersionToken(versionStr)
	if !ok || (version != httpV10 && version != httpV11) {
		return nil, ErrInvalidHTTPVersion
	}

	headerEnd := len(lines)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			headerEnd = i
			break
		}
	}

	allHeaders := parseHTTPHeaders(lines[1:headerEnd])

	// 收集小写化头映射（首值生效），同时剔除 Cookie / Referer（对应 parser.rs）
	headersMap := make(map[string]string, len(allHeaders))
	headers := make([]httpHeaderRaw, 0, len(allHeaders))
	for _, h := range allHeaders {
		lower := strings.ToLower(h.name)
		switch lower {
		case "cookie", "referer":
		default:
			if _, ok := headersMap[lower]; !ok {
				headersMap[lower] = *h.value
			}
			headers = append(headers, h)
		}
	}

	obs := &httpRequestObservation{
		matching: httpObservation{
			version: version,
			horder:  convertHeadersToHTTPFormat(headers, true),
			habsent: buildAbsentHeaders(headers, true),
			expsw:   expswValue(headersMap["user-agent"]),
		},
		lang: getHighestQualityLanguage(headersMap["accept-language"]),
	}
	_ = uri // 观测结构不消费 URI
	return obs, nil
}

// parseHTTPResponse 解析 HTTP/1.x 响应（对应 parser.rs parse_response）。
// 返回 nil 表示数据不完整（Ok(None)）。
func parseHTTPResponse(data []byte) (*httpResponseObservation, error) {
	if !utf8.Valid(data) {
		return nil, ErrInvalidUTF8
	}
	str := string(data)
	if !strings.Contains(str, "\r\n\r\n") && !strings.Contains(str, "\n\n") {
		return nil, ErrIncompleteData
	}
	lines := splitHTTPLines(str)
	if len(lines) == 0 {
		return nil, ErrIncompleteData
	}

	statusParts := strings.SplitN(lines[0], " ", 3)
	if len(statusParts) < 2 {
		return nil, ErrInvalidHTTPStatusLine
	}
	version, ok := parseHTTPVersionToken(statusParts[0])
	if !ok || (version != httpV10 && version != httpV11) {
		return nil, ErrInvalidHTTPVersion
	}
	status := statusParts[1]
	if len(status) != 3 || !isASCIIDigit(status[0]) || !isASCIIDigit(status[1]) || !isASCIIDigit(status[2]) {
		return nil, ErrInvalidHTTPStatusLine
	}

	headerEnd := len(lines)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			headerEnd = i
			break
		}
	}

	headers := parseHTTPHeaders(lines[1:headerEnd])
	headersMap := make(map[string]string, len(headers))
	for _, h := range headers {
		if _, ok := headersMap[strings.ToLower(h.name)]; !ok {
			headersMap[strings.ToLower(h.name)] = *h.value
		}
	}

	return &httpResponseObservation{
		matching: httpObservation{
			version: version,
			horder:  convertHeadersToHTTPFormat(headers, false),
			habsent: buildAbsentHeaders(headers, false),
			expsw:   expswValue(headersMap["server"]),
		},
	}, nil
}

// convertHeadersToHTTPFormat 将原始头转换为指纹格式（对应 convert_headers_to_http_format）。
// 可选头标记为 optional；忽略值的头不带值；其余保留值。
func convertHeadersToHTTPFormat(headers []httpHeaderRaw, isRequest bool) []httpHeader {
	optionalList := responseOptionalHeaders
	skipValueList := responseSkipValueHeaders
	if isRequest {
		optionalList = requestOptionalHeaders
		skipValueList = requestSkipValueHeaders
	}
	out := make([]httpHeader, 0, len(headers))
	for _, h := range headers {
		switch {
		case slices.Contains(optionalList, h.name):
			out = append(out, httpHeader{optional: true, name: h.name})
		case slices.Contains(skipValueList, h.name):
			out = append(out, httpHeader{name: h.name})
		default:
			out = append(out, httpHeader{name: h.name, value: h.value})
		}
	}
	return out
}

// buildAbsentHeaders 构建常见头中缺失的头列表（对应 build_absent_headers_from_new_parser）。
func buildAbsentHeaders(headers []httpHeaderRaw, isRequest bool) []httpHeader {
	commonList := responseCommonHeaders
	if isRequest {
		commonList = requestCommonHeaders
	}
	present := make(map[string]bool, len(headers))
	for _, h := range headers {
		present[strings.ToLower(h.name)] = true
	}
	out := make([]httpHeader, 0, len(commonList))
	for _, name := range commonList {
		if !present[strings.ToLower(name)] {
			out = append(out, httpHeader{name: name})
		}
	}
	return out
}

// expswValue 提取期望子串：缺失时为 "???"（对应 extract_traffic_classification）。
func expswValue(value string) string {
	if value == "" {
		return "???"
	}
	return value
}

// getHighestQualityLanguage 从 Accept-Language 头解析语言偏好
// （对应 languages.rs get_highest_quality_language）。
// 按质量降序、同质量取更靠前条目；无匹配时返回空串。
func getHighestQualityLanguage(acceptLanguage string) string {
	if acceptLanguage == "" {
		return ""
	}
	var (
		bestQuality float64
		bestName    string
		found       bool
	)
	for _, part := range strings.Split(acceptLanguage, ",") {
		langAndQuality := strings.SplitN(part, ";", 2)
		fullLanguage := strings.TrimSpace(langAndQuality[0])
		if fullLanguage == "" {
			continue
		}
		language := strings.SplitN(fullLanguage, "-", 2)[0]
		quality := 1.0
		if len(langAndQuality) > 1 {
			q := strings.TrimPrefix(strings.TrimSpace(langAndQuality[1]), "q=")
			if v, err := strconv.ParseFloat(q, 32); err == nil {
				quality = v
			}
		}
		name, ok := httpLangNames[language]
		if !ok {
			continue
		}
		// 质量更高则替换；质量相同保持先出现的（索引更靠前）
		if !found || quality > bestQuality {
			found = true
			bestQuality = quality
			bestName = name
		}
	}
	return bestName
}
