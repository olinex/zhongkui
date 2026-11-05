// Package retrieval 承载检索结果的线协议形态与融合、去重、排序、向量字面量等纯算法，
// 供知识检索与长期记忆检索共用同一套结果口径。本包不访问数据库、不调用模型。
package retrieval

import (
	// 标准库
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	// 第三方
	"github.com/cloudwego/eino/schema"

	// 项目内部

	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
)

// rrfK 是 RRF（Reciprocal Rank Fusion）融合的固定常数 k。
const rrfK = 60

// RetrievalResult 是一条检索结果的视图层载体（json 序列化给 view 层使用）。
type RetrievalResult struct {
	// DocumentID 所属知识文档 ID（字符串形式）
	DocumentID string `json:"documentID"`
	// NamespaceName 所属知识文档所在的命名空间名（检索范围为调用方命名空间 + 内置系统命名空间，
	// 同一文档 ID 可能在两者同时存在，故需据此消歧）
	NamespaceName string `json:"namespaceName"`
	// Title 所属文档标题
	Title string `json:"title"`
	// ChunkIndex 分块在文档内的序号
	ChunkIndex int `json:"chunkIndex"`
	// Content 该块的检索文本（标题路径 + 正文）；Complete 为 true 时为整篇文档全文
	Content string `json:"content"`
	// Score 检索相关度分数（融合后得分）
	Score float64 `json:"score"`
	// Complete 内容是否为整篇文档展开结果：为 true 时消费方不再截断。
	// 不参与线序列化，保持前端契约不变。
	Complete bool `json:"-"`
}

// chunkKey 是融合打分时识别同一分块的键。
type chunkKey struct {
	// docID 文档 ID（字符串形式）
	docID string
	// index 分块序号
	index int
}

// FusionWeights 是 weighted 融合的两路权重。
type FusionWeights struct {
	// Vector 向量路权重
	Vector float64
	// Keyword 关键词路权重
	Keyword float64
}

// fusionWeighted 是加权融合策略的取值（与行为参数 retrieval.fusion 的合法值一致，见
// internal/ai/config）。本包只消费该字符串，不反向依赖参数注册表。
const fusionWeighted = "weighted"

// FuseRetrievalResults 按策略融合两路检索结果；未知策略默认 RRF。
func FuseRetrievalResults(vector, keyword []*RetrievalResult, fusion string, weights FusionWeights) []*RetrievalResult {
	switch strings.ToLower(strings.TrimSpace(fusion)) {
	case fusionWeighted:
		return fuseWeighted(vector, keyword, weights)
	default:
		return FuseRRF(vector, keyword)
	}
}

// FuseRRF 以 RRF（sum(1/(k+rank))，k=60）融合两路结果，按融合分降序返回。
func FuseRRF(vector, keyword []*RetrievalResult) []*RetrievalResult {
	scores := make(map[chunkKey]*RetrievalResult)
	addPath := func(path []*RetrievalResult) {
		for i, hit := range path {
			key := chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}
			entry, ok := scores[key]
			if !ok {
				cp := *hit
				cp.Score = 0
				entry = &cp
				scores[key] = entry
			}
			entry.Score += 1.0 / float64(rrfK+i+1)
		}
	}
	addPath(vector)
	addPath(keyword)
	return sortedResults(scores)
}

// fuseWeighted 以归一化加权融合两路结果：每路得分先 min-max 归一化（最大值等于最小值
// 时取 0.5），再按 vector/keyword 权重线性加权，最后按融合分降序返回。
func fuseWeighted(vector, keyword []*RetrievalResult, weights FusionWeights) []*RetrievalResult {
	scores := make(map[chunkKey]*RetrievalResult)
	normalized := func(path []*RetrievalResult) map[chunkKey]float64 {
		out := make(map[chunkKey]float64, len(path))
		if len(path) == 0 {
			return out
		}
		minS, maxS := path[0].Score, path[0].Score
		for _, hit := range path {
			if hit.Score < minS {
				minS = hit.Score
			}
			if hit.Score > maxS {
				maxS = hit.Score
			}
		}
		for _, hit := range path {
			norm := 0.5
			if maxS > minS {
				norm = (hit.Score - minS) / (maxS - minS)
			}
			out[chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}] = norm
		}
		return out
	}
	vecNorm := normalized(vector)
	kwNorm := normalized(keyword)
	keyOf := func(hit *RetrievalResult) chunkKey {
		return chunkKey{docID: hit.DocumentID, index: hit.ChunkIndex}
	}
	merge := func(path []*RetrievalResult, w float64, norm map[chunkKey]float64) {
		for _, hit := range path {
			key := keyOf(hit)
			if _, ok := scores[key]; !ok {
				cp := *hit
				cp.Score = 0
				scores[key] = &cp
			}
			// 每路只叠加本路的加权归一化分，避免两路同现的命中被重复计分
			scores[key].Score += w * norm[key]
		}
	}
	merge(vector, weights.Vector, vecNorm)
	merge(keyword, weights.Keyword, kwNorm)
	return sortedResults(scores)
}

// sortedResults 将融合得分表转为按分数降序的切片（分数相同按 docID/chunkIndex 稳定排序）。
func sortedResults(scores map[chunkKey]*RetrievalResult) []*RetrievalResult {
	out := make([]*RetrievalResult, 0, len(scores))
	for _, hit := range scores {
		out = append(out, hit)
	}
	slices.SortFunc(out, compareResult)
	return out
}

// compareResult 比较两条检索结果的先后（分数更高者在前，同分时 docID/chunkIndex 更小者在前），
// 供 slices.SortFunc 使用。
func compareResult(a, b *RetrievalResult) int {
	if a.Score != b.Score {
		if a.Score > b.Score {
			return -1
		}
		return 1
	}
	if a.DocumentID != b.DocumentID {
		return strings.Compare(a.DocumentID, b.DocumentID)
	}
	return a.ChunkIndex - b.ChunkIndex
}

// 内容去重的字符 n-gram 长度与 Jaccard 相似度阈值。阈值取高值，只拦「内容实质相同」的命中：
// 分片接缝处的重叠复读（默认 overlap=100 对 size=800）只占约一成，远低于阈值，故相邻块
// 不会被误删；而同一篇自重复文本被切成相邻块、同一份内容被重复索引等真重复会被拦下。
const (
	// dedupeShingleSize 字符 n-gram 长度
	dedupeShingleSize = 4
	// dedupeSimilarityCutoff 默认相似度阈值
	dedupeSimilarityCutoff = 0.8
)

// DedupeSimilarResults 去掉融合排序结果中内容实质重复的命中：按排序自前向后逐条保留，
// 某条与任一已保留条目的字符 n-gram Jaccard 相似度不低于 dedupeSimilarityCutoff 时丢弃。
// 保留排序更靠前的一条——重排与融合都认定它更相关，分数更高且信息不劣于后者。输入不足
// 两条时原样返回。
func DedupeSimilarResults(hits []*RetrievalResult) []*RetrievalResult {
	return DedupeSimilarResultsWithCutoff(hits, dedupeSimilarityCutoff)
}

// DedupeSimilarResultsWithCutoff 是 DedupeSimilarResults 的阈值可变体：相似度阈值由调用方
// 指定（长期记忆检索去重用参数 memory.dedupeThreshold），其余行为完全一致。
func DedupeSimilarResultsWithCutoff(hits []*RetrievalResult, cutoff float64) []*RetrievalResult {
	if len(hits) < 2 {
		return hits
	}
	kept := make([]*RetrievalResult, 0, len(hits))
	keptShingles := make([]map[string]struct{}, 0, len(hits))
	for _, hit := range hits {
		shingles := contentShingles(hit.Content)
		duplicated := false
		for _, prev := range keptShingles {
			if jaccardSimilarityWithCutoff(shingles, prev, cutoff) >= cutoff {
				duplicated = true
				break
			}
		}
		if duplicated {
			continue
		}
		kept = append(kept, hit)
		keptShingles = append(keptShingles, shingles)
	}
	return kept
}

// contentShingles 把分块正文归一化后切成字符 n-gram 集合用作相似度指纹：归一化先折叠
// 全部空白，使标题路径拼接、段落分隔等排版差异不影响判定；正文短于 n-gram 长度时整体
// 作为一个指纹，保证「同样短小」也能互相判为重复。
func contentShingles(content string) map[string]struct{} {
	runes := []rune(strings.Join(strings.Fields(content), ""))
	if len(runes) == 0 {
		return nil
	}
	if len(runes) < dedupeShingleSize {
		return map[string]struct{}{string(runes): {}}
	}
	out := make(map[string]struct{}, len(runes)-dedupeShingleSize+1)
	for i := 0; i+dedupeShingleSize <= len(runes); i++ {
		out[string(runes[i:i+dedupeShingleSize])] = struct{}{}
	}
	return out
}

// jaccardSimilarityWithCutoff 计算两个 n-gram 集合的 Jaccard 相似度；任一集合为空时返回 0。
// 先做上限剪枝：Jaccard 至多为 min(|A|,|B|)/max(|A|,|B|)，达不到阈值的组合直接返回 0，
// 免去全量交集计算（正文长度悬殊的条目占绝大多数）。阈值由调用方指定（长期记忆检索去重用参数
// memory.dedupeThreshold）。
func jaccardSimilarityWithCutoff(a, b map[string]struct{}, cutoff float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	small, large := len(a), len(b)
	if small > large {
		small, large = large, small
	}
	if float64(small)/float64(large) < cutoff {
		return 0
	}
	var overlap int
	for shingle := range a {
		if _, ok := b[shingle]; ok {
			overlap++
		}
	}
	return float64(overlap) / float64(len(a)+len(b)-overlap)
}

// SelectTopK 取融合排序后的前 topK 条（重排不可用/失败时的降级结果）；topK 非正数时返回全部。
func SelectTopK(cands []*RetrievalResult, topK int) []*RetrievalResult {
	if topK <= 0 || len(cands) <= topK {
		return cands
	}
	return cands[:topK]
}

// ToRetrievalResults 从 eino schema.Document 列表（MetaData 携带 documentID/title/
// chunkIndex/score，Content 为检索文本）转换出 RetrievalResult 列表，供 view 层序列化。
func ToRetrievalResults(docs []*schema.Document) []RetrievalResult {
	out := make([]RetrievalResult, 0, len(docs))
	for _, d := range docs {
		r := RetrievalResult{Content: d.Content}
		if d.MetaData != nil {
			if v, ok := d.MetaData["documentID"].(string); ok {
				r.DocumentID = v
			}
			if v, ok := d.MetaData["namespaceName"].(string); ok {
				r.NamespaceName = v
			}
			if v, ok := d.MetaData["title"].(string); ok {
				r.Title = v
			}
			if v, ok := d.MetaData["chunkIndex"].(int); ok {
				r.ChunkIndex = v
			}
			if v, ok := d.MetaData["score"].(float64); ok {
				r.Score = v
			}
			if v, ok := d.MetaData["complete"].(bool); ok {
				r.Complete = v
			}
		}
		out = append(out, r)
	}
	return out
}

// RowsToResults 将检索 SQL 扫描行转换为检索结果，Content 为标题路径 + 正文的检索文本。
func RowsToResults(rows []struct {
	modelsai.KnowledgeChunk
	Title string
	Score float64
}) []*RetrievalResult {
	out := make([]*RetrievalResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, &RetrievalResult{
			DocumentID:    strconv.FormatUint(uint64(row.DocumentID), 10),
			NamespaceName: row.NamespaceName,
			Title:         row.Title,
			ChunkIndex:    row.ChunkIndex,
			Content:       strings.TrimSpace(string(row.HeadingPath.Value()) + "\n" + string(row.Content.Value())),
			Score:         row.Score,
		})
	}
	return out
}

// VectorLiteral 将 []float64 序列化为 pgvector 字面量（如 "[0.1,0.2,...]"）。
func VectorLiteral(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// ToFloat32 将 []float64 转为 []float32（向量化输出转存储维度类型）。
func ToFloat32(vec []float64) []float32 {
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = float32(v)
	}
	return out
}

// TruncateText 按 rune 截断字符串到不超过 max 个字符，超限时追加 "..."；max 非正数或未超限时原样返回。
func TruncateText(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// TruncateBytes 把字符串按 UTF-8 rune 边界截断到不超过 max 字节（max 非正数时不截断），
// 发生截断时追加 "..."，保证不产生残缺的多字节字符。与 TruncateText（rune 语义）不同，
// 本函数以字节为限，用于限制工具结果对模型上下文的占用。
func TruncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	// 从 max 处向前回退到 rune 边界，避免切断多字节字符。
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}
