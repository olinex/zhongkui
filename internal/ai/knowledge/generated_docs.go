package knowledge

import (
	// 标准库
	"context"
	"strings"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto"
	"zhongkuisec.com/zhongkui/internal/pkg/docconv"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// GeneratedKnowledgeDoc 是由代码注释派生的生成知识文档的通用载体（叙述页等）：Key/Title 均为稳定键
// （Title 作为入库与去重的稳定键），Content 为 Markdown 正文。文档集由调用方从注释抽取模型派生后
// 传入，本包不自行派生。
type GeneratedKnowledgeDoc struct {
	// Key 生成条目的稳定键（同一包内唯一）
	Key string
	// Title 文档标题，同时作为入库与同步判定的稳定键
	Title string
	// Content 文档正文（Markdown）
	Content string
}

// 注释生成文档（叙述页等）在知识库中按 format 分区隔离：以 docconv.FormatGenerated 落库，与
// 人工导入/手工录入的业务文档可区分。同步某一分区只读取、创建、更新与删除该 format 的文档，
// 绝不触碰其余格式的文档。

// existingGeneratedDoc 是已入库生成文档的摘要视图：仅取同步判定所需的 ID、标题与内容摘要，
// 不载入正文密文与向量。
type existingGeneratedDoc struct {
	// ID 文档 ID。
	ID uint
	// Title 文档标题，为分区内的稳定键。
	Title string
	// Hash 文档内容摘要（IndexDocument 写入的 HMAC-SHA256），与派生内容摘要比对判定是否需更新。
	Hash string
}

// partitionUpdate 描述一条需更新正文的生成文档。
type partitionUpdate struct {
	// DocID 既有文档 ID。
	DocID uint
	// Title 稳定标题（同时作为文档标题）。
	Title string
	// Content 新正文（Markdown）。
	Content string
}

// partitionRebuildPlan 是一次分区同步的差异计划：需新增、需更新正文、需删除的文档。
type partitionRebuildPlan struct {
	// create 生成集合中新增、库中不存在的文档。
	create []GeneratedKnowledgeDoc
	// update 库中已存在但正文摘要变化的文档。
	update []partitionUpdate
	// delete 库中存在但生成集合已移除的文档 ID。
	delete []uint
}

// empty 报告计划是否为空（无需任何写入），用于幂等短路。
func (p partitionRebuildPlan) empty() bool {
	return len(p.create) == 0 && len(p.update) == 0 && len(p.delete) == 0
}

// SyncGeneratedDocs 以传入的生成文档集为准，幂等同步指定命名空间下 format 分区的文档：新增文档集
// 中新出现的条目、更新正文变化的条目、删除库中已有但文档集已移除的条目；正文未变的条目原样保留
// （不重复向量化、不产生重复条目）。文档集由调用方从注释抽取模型派生后传入，本方法不再自行派生。
// 全程逐条幂等，任一步失败即返回错误（已成功处理的条目不回滚，下次触发按同一差异计划继续收敛）。
// report 为进度上报入口（为 nil 时不上报）：计划构建后上报一次 Start(计划总条数)，每条成功后上报
// 一次 Step（含动作、标题、耗时与 token 用量），结束（含提前失败）时经 defer 上报一次 Finish。
// namespaceName 为空时返回英文错误。
func (k *Knowledge) SyncGeneratedDocs(ctx context.Context, namespaceName string, format docconv.Format, docs []GeneratedKnowledgeDoc, report contract.SyncReporter) error {
	if strings.TrimSpace(namespaceName) == "" {
		return errors.New("knowledge: empty namespace for generated docs sync")
	}
	existing, err := k.loadPartitionDocuments(ctx, namespaceName, format)
	if err != nil {
		return errors.WithStack(err)
	}
	plan, err := buildPartitionRebuildPlan(docs, existing)
	if err != nil {
		return errors.WithStack(err)
	}
	// 删除项在计划里只保留 ID，标题取自既有文档摘要，供删除 Step 上报条目名
	titles := make(map[uint]string, len(existing))
	for _, doc := range existing {
		titles[doc.ID] = doc.Title
	}
	total := len(plan.create) + len(plan.update) + len(plan.delete)
	contract.ReportStart(report, total)
	defer contract.ReportFinish(report)
	return k.applyPartitionRebuildPlan(ctx, namespaceName, format, plan, titles, report)
}

// loadPartitionDocuments 读取命名空间下指定 format 分区的文档摘要（仅 id/title/hash，含软删除过滤，
// 只取该 format，不触碰其余分区文档）。
func (k *Knowledge) loadPartitionDocuments(ctx context.Context, namespaceName string, format docconv.Format) ([]existingGeneratedDoc, error) {
	var rows []existingGeneratedDoc
	if err := k.db.WithContext(ctx).
		Table("knowledge_documents").
		Select("id, title, hash").
		Where("namespace_name = ? AND format = ? AND deleted_at IS NULL", namespaceName, string(format)).
		Find(&rows).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return rows, nil
}

// buildPartitionRebuildPlan 比较派生文档与已入库文档，产出新增/更新/删除差异（纯函数，不触达
// 数据库）：标题为稳定键；既有摘要与派生摘要一致即视为未变而跳过；库中存在但生成集合已无的标题
// 进入删除集。派生内容摘要不可计算时返回错误。
func buildPartitionRebuildPlan(derived []GeneratedKnowledgeDoc, existing []existingGeneratedDoc) (partitionRebuildPlan, error) {
	byTitle := make(map[string]existingGeneratedDoc, len(existing))
	for _, doc := range existing {
		byTitle[doc.Title] = doc
	}
	var plan partitionRebuildPlan
	seen := make(map[string]struct{}, len(derived))
	for _, item := range derived {
		seen[item.Title] = struct{}{}
		hash, err := contentDigest(item.Content)
		if err != nil {
			return partitionRebuildPlan{}, errors.WithStack(err)
		}
		doc, ok := byTitle[item.Title]
		switch {
		case !ok:
			plan.create = append(plan.create, item)
		case doc.Hash != hash:
			plan.update = append(plan.update, partitionUpdate{
				DocID:   doc.ID,
				Title:   item.Title,
				Content: item.Content,
			})
		}
	}
	for _, doc := range existing {
		if _, ok := seen[doc.Title]; !ok {
			plan.delete = append(plan.delete, doc.ID)
		}
	}
	return plan, nil
}

// applyPartitionRebuildPlan 执行差异计划：新增文档按 format 落库后建索引；更新文档先落新正文再
// 重建索引；删除文档复用 DeleteDocument（同时清理分块）。每步计时并经 indexDocument 取回 token
// 用量，逐条上报进度（report 为 nil 时不上报）；titles 提供删除项（计划中仅有 ID）的条目名。
func (k *Knowledge) applyPartitionRebuildPlan(ctx context.Context, namespaceName string, format docconv.Format, plan partitionRebuildPlan, titles map[uint]string, report contract.SyncReporter) error {
	for _, item := range plan.create {
		start := time.Now()
		doc := &ai.KnowledgeDocument{
			NamespaceName: namespaceName,
			Title:         item.Title,
			Content:       serializer.NewCrypto([]byte(item.Content)),
			Format:        string(format),
			Status:        string(ai.KnowledgeDocStatusReady),
		}
		if err := k.db.WithContext(ctx).Create(doc).Error; err != nil {
			return errors.WithStack(err)
		}
		usage, err := k.indexDocument(ctx, doc)
		if err != nil {
			return errors.WithStack(err)
		}
		contract.ReportStep(report, contract.SyncProgressItem{
			Title:     item.Title,
			Action:    contract.SyncActionCreate,
			Count:     1,
			ElapsedMs: time.Since(start).Milliseconds(),
			Usage:     usage,
		})
	}
	for _, item := range plan.update {
		start := time.Now()
		// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
		if err := k.db.WithContext(ctx).Model(&ai.KnowledgeDocument{}).Where("id = ?", item.DocID).
			Select("content").Updates(&ai.KnowledgeDocument{Content: serializer.NewCrypto([]byte(item.Content))}).Error; err != nil {
			return errors.WithStack(err)
		}
		doc := &ai.KnowledgeDocument{
			NamespaceName: namespaceName,
			Title:         item.Title,
			Content:       serializer.NewCrypto([]byte(item.Content)),
		}
		doc.ID = item.DocID
		usage, err := k.indexDocument(ctx, doc)
		if err != nil {
			return errors.WithStack(err)
		}
		contract.ReportStep(report, contract.SyncProgressItem{
			Title:     item.Title,
			Action:    contract.SyncActionUpdate,
			Count:     1,
			ElapsedMs: time.Since(start).Milliseconds(),
			Usage:     usage,
		})
	}
	for _, docID := range plan.delete {
		start := time.Now()
		if err := k.DeleteDocument(ctx, docID); err != nil {
			return errors.WithStack(err)
		}
		contract.ReportStep(report, contract.SyncProgressItem{
			Title:     titles[docID],
			Action:    contract.SyncActionDelete,
			Count:     1,
			ElapsedMs: time.Since(start).Milliseconds(),
			Usage:     contract.EmbedUsage{},
		})
	}
	return nil
}

// contentDigest 计算生成文档正文的内容摘要，复用文档 hash 字段的密钥与算法，使派生摘要与
// IndexDocument 落库的摘要可直接等值比对。
func contentDigest(content string) (string, error) {
	return crypto.DigestForField(&ai.KnowledgeDocument{}, "hash", "json", []byte(content))
}
