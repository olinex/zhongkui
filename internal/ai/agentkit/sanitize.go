package agentkit

import (
	"encoding/json"
	"reflect"
	"sort"

	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// SanitizeBusinessData 对业务数据执行清洗与限界，返回与原数据相互独立的结果
// （入参不会被修改）：
//   - 清洗：递归剔除空值条目（nil、空字符串、空切片、空 map）——map 递归清洗每个
//     条目、slice/array 递归清洗每个元素，嵌套结构清洗后变空的条目/元素一并剔除；
//   - 条目数限界：清洗后顶层条目数超过参数 sanitize.maxItems（>0 时生效）时，按条目
//     序列化字节数从大到小依次丢弃，直至满足上限；
//   - 字节限界：若紧凑 JSON 序列化字节数超过参数 sanitize.maxBytes，按条目序列化字节数
//     从大到小依次丢弃（字节数相同时按 key 字典序升序），直至整体不超限——优先保留信息
//     密度更高的短条目；
//   - 两个限界参数（经 params 读取）非正数时视为不设该上限。
//
// 返回结果始终是合法 JSON 对象（json.Marshal 不会失败），供上层直接序列化注入。
func SanitizeBusinessData(data contract.BusinessData, params *config.Params) (contract.BusinessData, error) {
	out := make(contract.BusinessData, len(data))
	for k, v := range data {
		cleaned := sanitizeBusinessValue(v)
		if isEmptyBusinessValue(cleaned) {
			continue
		}
		out[k] = cleaned
	}
	maxItems := params.Int("sanitize.maxItems")
	if maxItems > 0 && len(out) > maxItems {
		for _, k := range businessKeysBySizeDesc(out) {
			delete(out, k)
			if len(out) <= maxItems {
				break
			}
		}
	}
	maxBytes := params.Int("sanitize.maxBytes")
	if maxBytes <= 0 {
		return out, nil
	}
	if fits, err := jsonFits(out, maxBytes); err != nil {
		return nil, errors.WithStack(err)
	} else if fits {
		return out, nil
	}
	// 按字节数从大到小依次丢弃条目，直至整体不超限；每次删除后重新序列化保证预算判定精确。
	for _, k := range businessKeysBySizeDesc(out) {
		delete(out, k)
		if fits, err := jsonFits(out, maxBytes); err != nil {
			return nil, errors.WithStack(err)
		} else if fits {
			break
		}
	}
	return out, nil
}

// sanitizeBusinessValue 递归清洗单个业务值并返回独立副本：map 逐条目清洗并剔除
// 清洗后为空的条目，slice/array 逐元素清洗并剔除清洗后为空的元素；其余类型（字符串、
// 数值、布尔、nil 及不可反射类型）原样返回。返回 nil 表示该值清洗后为空，应由调用方剔除。
func sanitizeBusinessValue(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		// 仅支持 string 键的 map（JSON 对象）；其余键类型按整体处理（无法逐条目清洗）。
		if rv.Type().Key().Kind() != reflect.String {
			return v
		}
		out := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			cleaned := sanitizeBusinessValue(iter.Value().Interface())
			if isEmptyBusinessValue(cleaned) {
				continue
			}
			// 复制 key 使其可寻址，避免 SetMapIndex 对不可寻址 key 报错。
			key := reflect.New(iter.Key().Type()).Elem()
			key.Set(iter.Key())
			out.SetMapIndex(key, reflect.ValueOf(cleaned))
		}
		if out.Len() == 0 {
			return nil
		}
		return out.Interface()
	case reflect.Slice, reflect.Array:
		out := reflect.MakeSlice(rv.Type(), 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			cleaned := sanitizeBusinessValue(rv.Index(i).Interface())
			if isEmptyBusinessValue(cleaned) {
				continue
			}
			out = reflect.Append(out, reflect.ValueOf(cleaned))
		}
		if out.Len() == 0 {
			return nil
		}
		return out.Interface()
	default:
		return v
	}
}

// isEmptyBusinessValue 报告清洗后的值是否应被剔除。
func isEmptyBusinessValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return rv.Len() == 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	default:
		return false
	}
}

// jsonFits 报告 data 经紧凑 JSON 序列化后的字节数是否不超过 maxBytes。
func jsonFits(data contract.BusinessData, maxBytes int) (bool, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return false, errors.WithStack(err)
	}
	return len(b) <= maxBytes, nil
}

// businessKeysBySizeDesc 按条目序列化字节数降序返回 key（字节数相同时按字典序升序），
// 保证截断行为确定可复现。
func businessKeysBySizeDesc(data contract.BusinessData) []string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sizeOf := func(k string) int {
		b, err := json.Marshal(data[k])
		if err != nil {
			// 整体可序列化（jsonFits 已校验）则单值必然可序列化，此处不会触发。
			return 0
		}
		return len(b)
	}
	sort.Slice(keys, func(i, j int) bool {
		si, sj := sizeOf(keys[i]), sizeOf(keys[j])
		if si != sj {
			return si > sj
		}
		return keys[i] < keys[j]
	})
	return keys
}
