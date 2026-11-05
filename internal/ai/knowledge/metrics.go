package knowledge

import (
	// 标准库
	"context"
	"math"
	"sort"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/port"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// contextPrecision 计算 RAGAS 上下文精确率：相关分块的 1-based 位置集合 R 上各
// precision@k（前 k 个位置中相关数 / k）的均值；无相关分块时返回 0。
func contextPrecision(judgments []ChunkJudgment) float64 {
	// 模型可能不按检索顺序返回判断，先按分块下标排序，使前 k 位的分母与检索顺序一致
	sorted := make([]ChunkJudgment, len(judgments))
	copy(sorted, judgments)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Index < sorted[j].Index })
	var relevant int
	var sum float64
	for k, j := range sorted {
		if j.Relevant {
			relevant++
			sum += float64(relevant) / float64(k+1)
		}
	}
	if relevant == 0 {
		return 0
	}
	return sum / float64(relevant)
}

// faithfulness 计算 RAGAS 忠实度：supported 声明数 / 声明总数；无声明时返回 0。
func faithfulness(claims []FaithfulnessClaim) float64 {
	if len(claims) == 0 {
		return 0
	}
	var supported int
	for _, c := range claims {
		if c.Supported {
			supported++
		}
	}
	return float64(supported) / float64(len(claims))
}

// contextUtilization 计算 RAGAS 上下文利用率：相关分块集合 R 与被 supported 声明引用的
// 分块集合 C 的交集大小 / |R|；无相关分块时返回 0。
func contextUtilization(judgments []ChunkJudgment, claims []FaithfulnessClaim) float64 {
	relevant := make(map[int]bool, len(judgments))
	for _, j := range judgments {
		if j.Relevant {
			relevant[j.Index] = true
		}
	}
	if len(relevant) == 0 {
		return 0
	}
	used := make(map[int]bool)
	for _, c := range claims {
		if !c.Supported {
			continue
		}
		for _, idx := range c.ChunkIndexes {
			used[idx] = true
		}
	}
	var overlap int
	for idx := range relevant {
		if used[idx] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(relevant))
}

// answerRelevance 计算 RAGAS 答案相关度：query 与各反推问题的嵌入余弦相似度均值；
// questions 为空时返回 (0, nil)；嵌入返回的向量数与输入文本数不符时返回英文错误。
func answerRelevance(ctx context.Context, embed port.Embedder, query string, questions []string) (float64, error) {
	if len(questions) == 0 {
		return 0, nil
	}
	texts := make([]string, 0, len(questions)+1)
	texts = append(texts, query)
	texts = append(texts, questions...)
	vecs, err := embed.EmbedStrings(ctx, texts)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if len(vecs) != len(texts) {
		return 0, errors.Newf("dry-run: answer relevance embedding returned %d vectors for %d texts", len(vecs), len(texts))
	}
	var sum float64
	for _, v := range vecs[1:] {
		sum += cosineSimilarity(vecs[0], v)
	}
	return sum / float64(len(questions)), nil
}

// cosineSimilarity 计算两个向量的余弦相似度（零向量或长度不等时返回 0）。
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
