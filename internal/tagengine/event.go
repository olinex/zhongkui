package tagengine

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// DetectionCondition 标签事件检测条件的运行时形态，由 core 库 tag_event_conditions 行或 dry-run 请求映射。
type DetectionCondition struct {
	RuleID        uint                  // 所属规则 ID
	RuleName      string                // 所属规则名称
	MatchAll      bool                  // 规则是否要求所有条件命中才触发
	ConditionID   uint                  // 条件 ID
	ConditionName string                // 条件名称
	Namespace     string                // 条件所属命名空间
	Model         string                // 目标模型类型键过滤，空表示不限制
	Dimension     string                // 标签维度过滤，空表示不限制
	Item          string                // 标签维度具体值过滤，空表示不限制
	Window        serializer.DurationNS // 时间窗口
	Threshold     int                   // 阈值
	// SuppressWindow 规则级抑制窗口（纳秒），0 表示不抑制；dry-run 候选规则为零值不参与判定
	SuppressWindow serializer.DurationNS // 抑制窗口
	Expression     string                // 标签组合布尔表达式
}

// DetectionHit 标签事件检测查询结果行，按 (namespace, model, dimension, item) 分组聚合。
type DetectionHit struct {
	// ConditionID 命中分组所属条件的 ID
	ConditionID uint `gorm:"column:condition_id"`
	// Namespace 命中分组所属命名空间
	Namespace string `gorm:"column:namespace"`
	// Model 命中分组的目标模型类型键
	Model string `gorm:"column:model"`
	// Dimension 命中分组的标签维度
	Dimension string `gorm:"column:dimension"`
	// Item 命中分组的标签维度具体值
	Item string `gorm:"column:item"`
	// Total 命中分组在时间窗口内的命中总次数
	Total uint64 `gorm:"column:total"`
	// IDs 该命中分组去重聚合后的原始记录 ID 列表（如 http_transports.id）
	IDs serializer.HexUUIDs `gorm:"column:ids"`
}

// TagEventRecord 标签事件记录识别结果，作为检测入库的持久化数据源或 dry-run 响应返回。
type TagEventRecord struct {
	// Namespace 事件所属命名空间
	Namespace string `json:"namespace"`
	// RuleID 触发事件的规则 ID
	RuleID uint `json:"ruleID"`
	// RuleName 触发事件的规则名称
	RuleName string `json:"ruleName"`
	// MatchAll 触发规则是否要求所有条件命中
	MatchAll bool `json:"matchAll"`
	// DetectedAt 事件检测时间
	DetectedAt time.Time `json:"detectedAt"`
	// Hits 事件包含的命中分组列表
	Hits []TagEventRecordHit `json:"hits"`
}

// TagEventRecordHit 标签事件记录中的单条命中分组信息。
type TagEventRecordHit struct {
	// ConditionID 命中分组所属条件的 ID
	ConditionID uint `json:"conditionID"`
	// ConditionName 命中分组所属条件的名称
	ConditionName string `json:"conditionName"`
	// Model 命中分组的目标模型类型键
	Model string `json:"model"`
	// Dimension 命中分组的标签维度
	Dimension string `json:"dimension"`
	// Item 命中分组的标签维度具体值
	Item string `json:"item"`
	// Count 命中分组在时间窗口内的命中次数
	Count uint64 `json:"count"`
	// Threshold 条件触发阈值
	Threshold int `json:"threshold"`
	// Window 统计时间窗口（纳秒）
	Window serializer.DurationNS `json:"window"`
	// IDs 命中分组去重聚合后的原始记录 ID 列表
	IDs serializer.HexUUIDs `json:"ids"`
}

// TranslateExpression 将标签组合布尔表达式翻译为 GORM clause 表达式树。
// 每个标签标识符转换为 clause.Expr{SQL: "has(tags, ?)", Vars: [label]}，
// AND/OR/NOT/括号分别由 clause.And/clause.Or/clause.Not 与嵌套表达式表达，
// 括号渲染与参数绑定均由 GORM 负责，无需手工拼接 SQL 片段。
func TranslateExpression(expr string) (clause.Expression, error) {
	ast, err := tag.ParseBoolExpr(expr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return boolExprToClause(ast)
}

func boolExprToClause(e *tag.BoolExpr) (clause.Expression, error) {
	parts := make([]clause.Expression, 0, len(e.Or))
	for _, term := range e.Or {
		p, err := andTermToClause(term)
		if err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return clause.Or(parts...), nil
}

func andTermToClause(t *tag.AndTerm) (clause.Expression, error) {
	parts := make([]clause.Expression, 0, len(t.Not))
	for _, factor := range t.Not {
		p, err := notFactorToClause(factor)
		if err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return clause.And(parts...), nil
}

func notFactorToClause(f *tag.NotFactor) (clause.Expression, error) {
	atom, err := atomToClause(f.Atom)
	if err != nil {
		return nil, err
	}
	if f.Not {
		return clause.Not(atom), nil
	}
	return atom, nil
}

func atomToClause(a *tag.Atom) (clause.Expression, error) {
	if a == nil {
		return nil, errors.New("tag event expression: empty atom")
	}
	if a.Label != "" {
		return clause.Expr{SQL: "has(tags, ?)", Vars: []any{a.Label}}, nil
	}
	if a.Expr != nil {
		return boolExprToClause(a.Expr)
	}
	return nil, errors.New("tag event expression: atom has neither label nor sub expression")
}

// BuildDetectionQuery 将全部启用条件组装为单条 UNION ALL SQL，一次查询返回所有命中分组。
// 每个条件一个 SELECT 分支：按 (namespace, model, dimension, item) 分组，SUM(cnt) > threshold 视为命中；
// 每个分组额外以 arrayDistinct(arrayFlatten(groupArray(ids))) 去重聚合窗口内各分组的原始记录 ID（ids 列），
// 供命中明细回溯到原始记录；时间窗口为 minute >= now-window AND minute <= now，
// 边界用 database.ToUTCDatetime64 格式化为 UTC 微秒精度字符串。
// 各分支由 GORM 链式 API 构造（Select/Where/Group/Having），表达式谓词以 clause 表达式树传入，
// 再通过 db.Raw("? UNION ALL ?", ...) 将子查询展开合并为单条 SQL，避免手工拼接整条查询字符串。
// 条件 model/dimension/item 非空时附加等值过滤；表达式翻译失败的条件跳过。
// 返回 SQL 与按占位符顺序排列的参数；conds 为空或全部无效时返回错误。
func BuildDetectionQuery(db *gorm.DB, conds []DetectionCondition, now time.Time) (string, []any, error) {
	if len(conds) == 0 {
		return "", nil, errors.New("tag event detection: no conditions to build query")
	}
	subqueries := make([]any, 0, len(conds))
	for _, cond := range conds {
		condClause, err := TranslateExpression(cond.Expression)
		if err != nil {
			continue // 表达式非法跳过该分支（调用方已预校验，此处兜底）
		}
		sub := db.Table("tag_summings").
			Select("? AS condition_id, namespace, model, dimension, item, SUM(cnt) AS total, arrayDistinct(arrayFlatten(groupArray(ids))) AS ids", cond.ConditionID).
			Where("namespace = ?", cond.Namespace).
			Where("minute >= ?", database.ToUTCDatetime64(now.Add(-cond.Window.Std()), 6)).
			Where("minute <= ?", database.ToUTCDatetime64(now, 6)).
			Where(condClause)
		if cond.Model != "" {
			sub = sub.Where("model = ?", cond.Model)
		}
		if cond.Dimension != "" {
			sub = sub.Where("dimension = ?", cond.Dimension)
		}
		if cond.Item != "" {
			sub = sub.Where("item = ?", cond.Item)
		}
		sub = sub.Group("namespace, model, dimension, item").Having("SUM(cnt) > ?", cond.Threshold)
		subqueries = append(subqueries, sub)
	}
	if len(subqueries) == 0 {
		return "", nil, errors.New("tag event detection: no valid conditions to build query")
	}
	// db.Raw 遇到 *gorm.DB 参数时会在 DryRun 下展开子查询 SQL 并合并其占位符参数。
	tx := db.Raw(strings.TrimSuffix(strings.Repeat("? UNION ALL ", len(subqueries)), " UNION ALL "), subqueries...)
	return tx.Statement.SQL.String(), tx.Statement.Vars, nil
}

// GroupEvents 将检测命中的分组按规则/条件聚合为标签事件。
// match_all=false：任一条件命中即触发，每个 (条件, 命中分组) 生成一条事件（Hits 仅含该分组）；
// match_all=true：仅当规则下所有启用条件均至少有一个命中分组时触发一次，Hits 汇总全部命中分组。
func GroupEvents(conds []DetectionCondition, hits []DetectionHit, now time.Time) []*TagEventRecord {
	hitsByCond := make(map[uint][]DetectionHit)
	for _, hit := range hits {
		hitsByCond[hit.ConditionID] = append(hitsByCond[hit.ConditionID], hit)
	}
	var ruleOrder []uint
	condsByRule := make(map[uint][]DetectionCondition)
	for _, c := range conds {
		if _, ok := condsByRule[c.RuleID]; !ok {
			ruleOrder = append(ruleOrder, c.RuleID)
		}
		condsByRule[c.RuleID] = append(condsByRule[c.RuleID], c)
	}

	var events []*TagEventRecord
	for _, ruleID := range ruleOrder {
		ruleConds := condsByRule[ruleID]
		rule := ruleConds[0]
		if rule.MatchAll {
			var evHits []TagEventRecordHit
			allHit := true
			for _, c := range ruleConds {
				groupHits := hitsByCond[c.ConditionID]
				if len(groupHits) == 0 {
					allHit = false
					break
				}
				evHits = append(evHits, toEventHits(c, groupHits)...)
			}
			if allHit {
				events = append(events, &TagEventRecord{
					Namespace:  rule.Namespace,
					RuleID:     rule.RuleID,
					RuleName:   rule.RuleName,
					MatchAll:   true,
					DetectedAt: now,
					Hits:       evHits,
				})
			}
		} else {
			for _, c := range ruleConds {
				for _, hit := range hitsByCond[c.ConditionID] {
					events = append(events, &TagEventRecord{
						Namespace:  hit.Namespace,
						RuleID:     rule.RuleID,
						RuleName:   rule.RuleName,
						MatchAll:   false,
						DetectedAt: now,
						Hits:       []TagEventRecordHit{toEventHit(c, hit)},
					})
				}
			}
		}
	}
	return events
}

func toEventHits(c DetectionCondition, hits []DetectionHit) []TagEventRecordHit {
	out := make([]TagEventRecordHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, toEventHit(c, hit))
	}
	return out
}

func toEventHit(c DetectionCondition, hit DetectionHit) TagEventRecordHit {
	return TagEventRecordHit{
		ConditionID:   c.ConditionID,
		ConditionName: c.ConditionName,
		// Model 直接存完整类型键（数据库存储语义，与 TagRule.Model /
		// TagSumming.Model / TagEventCondition.Model 一致）；展示 label 由消费方
		// 经 tagRuleModel 枚举类按 enum.tagRuleModel.<ModelKey> 推导，
		// 命中明细不做 value→key 的预转换。
		Model:     hit.Model,
		Dimension: hit.Dimension,
		Item:      hit.Item,
		Count:     hit.Total,
		Threshold: c.Threshold,
		Window:    c.Window,
		IDs:       hit.IDs,
	}
}

// Detect 执行一次标签事件识别。
// 条件由调用方传入（本函数不主动读取规则库），按指定时间点 now 查询聚合统计并聚合为标签事件结果；
// 供定时检测（worker 消费 tag:event:detect）与接口层 dry-run 模式复用。
// 非法表达式条件被静默跳过，全部条件无效时返回空结果而非错误。
func Detect(ctx context.Context, rawDB *database.DB, conds []DetectionCondition, now time.Time) ([]*TagEventRecord, error) {
	valid := make([]DetectionCondition, 0, len(conds))
	for _, cond := range conds {
		if _, err := TranslateExpression(cond.Expression); err != nil {
			continue // 表达式非法跳过（调用方可预校验以便告警，此处兜底）
		}
		valid = append(valid, cond)
	}
	if len(valid) == 0 {
		return nil, nil
	}
	query, args, err := BuildDetectionQuery(rawDB, valid, now)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var hits []DetectionHit
	if err := rawDB.WithContext(ctx).Raw(query, args...).Scan(&hits).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return GroupEvents(valid, hits, now), nil
}
