// 本文件实现按事件模板注册表（template.go）为命名空间幂等种子化/回填内置默认
// 通知模板（NotificationTemplate，source=default）的逻辑。供命名空间创建（manager 层）
// 与启动回填（cmd 层）复用。
//
// This file implements the idempotent seeding/backfilling of built-in default
// notification templates (NotificationTemplate, source=default) for a namespace from
// the event template registry (template.go). It is reused by namespace creation
// (manager layer) and startup backfill (cmd layer).
package notify

import (
	// third-party
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// templateConflictColumns 默认模板回填落库时冲突判断的列：与唯一索引
// uq_notification_templates_namespace_event_channel_language 的列顺序保持一致。
// templateConflictColumns are the columns used to detect conflicts when seeding default
// templates, aligned with the unique index
// uq_notification_templates_namespace_event_channel_language column order.
var templateConflictColumns = []clause.Column{
	{Name: "namespace_name"},
	{Name: "event"},
	{Name: "channel_type"},
	{Name: "language"},
}

// SeedDefaultTemplates 为指定命名空间幂等写入注册表（TemplateSpecs）中的内置默认模板行。
// 遍历每个规格，对规格 Languages 中 TitleByLang 与 ContentByLang 均非空的每种语言，
// 生成一行 (Event, "*", language) 组合，沿用规格中的 Title/Content、Source=default、
// Description 为空；使用 INSERT ... ON CONFLICT DO NOTHING 保证幂等——同名
// (namespace_name,event,channel_type,language) 已存在（含用户覆盖 override 行）时静默跳过，
// 不覆盖、不产生重复。返回错误由调用方决定是否中断；内部不 panic。
// logger 为可选参数，非 nil 时以 debug 级别记录每条回填的模板行。
//
// SeedDefaultTemplates idempotently writes the built-in default template rows from the
// registry (TemplateSpecs) for the given namespace. For each spec, a row
// (Event, "*", language) is produced for every language whose TitleByLang and
// ContentByLang are both non-empty, carrying the spec's Title/Content, Source=default
// and an empty Description; INSERT ... ON CONFLICT DO NOTHING guarantees idempotency —
// an existing row with the same (namespace_name,event,channel_type,language), including
// a user override (Source=override), is skipped silently, never overwritten or
// duplicated. Callers decide whether a returned error aborts; it never panics.
// logger is optional; when non-nil each seeded row is logged at debug level.
func SeedDefaultTemplates(db *gorm.DB, namespace string, logger ...*logging.Logger) error {
	var lg *logging.Logger
	if len(logger) > 0 {
		lg = logger[0]
	}

	specs := TemplateSpecs()
	templates := make([]*core.NotificationTemplate, 0, len(specs))
	for _, spec := range specs {
		for _, lang := range spec.Languages {
			title, okTitle := spec.TitleByLang[lang]
			content, okContent := spec.ContentByLang[lang]
			// 仅当该语言下标题与正文模板均存在且非空时才落库
			if !okTitle || !okContent || title == "" || content == "" {
				continue
			}
			templates = append(templates, &core.NotificationTemplate{
				NamespaceName: namespace,
				Event:         spec.Event,
				ChannelType:   "*",
				Language:      lang,
				Title:         title,
				Content:       content,
				Source:        core.TemplateSourceDefault,
			})
		}
	}
	if len(templates) == 0 {
		return nil
	}

	// 冲突行跳过（既不覆盖既有 override 行，也不产生重复 default 行），实现幂等
	if err := db.Clauses(clause.OnConflict{
		Columns:   templateConflictColumns,
		DoNothing: true,
	}).Create(&templates).Error; err != nil {
		return errors.WithStack(err)
	}

	if lg != nil {
		for _, t := range templates {
			lg.Debugf(
				"notify: ensure default template namespace=%s event=%s channel=* lang=%s",
				namespace, t.Event, t.Language,
			)
		}
	}
	return nil
}

// BackfillDefaultTemplates 启动时为既有命名空间幂等回填内置默认通知模板：读取命名空间表
// 的全部名称，逐命名空间调用 SeedDefaultTemplates。单个命名空间回填失败仅记录警告并继续，
// 不中断整体；返回首个失败（无失败时返回 nil），供调用方以警告级别记录而不阻断启动。
//
// BackfillDefaultTemplates backfills the built-in default notification templates for all
// existing namespaces at startup: it reads every namespace name and calls
// SeedDefaultTemplates per namespace. A failure on one namespace is logged as a warning
// and the loop continues; the first failure is returned (nil when all succeed) so the
// caller can log a warning without aborting startup.
func BackfillDefaultTemplates(db *gorm.DB, logger *logging.Logger) error {
	var namespaces []string
	if err := db.Model(&core.Namespace{}).Distinct().Pluck("name", &namespaces).Error; err != nil {
		return errors.WithStack(err)
	}

	var firstErr error
	for _, ns := range namespaces {
		if err := SeedDefaultTemplates(db, ns, logger); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			logger.Warnf(
				"notify: failed to backfill default templates for namespace=%s: %v",
				ns, err,
			)
		}
	}
	return firstErr
}