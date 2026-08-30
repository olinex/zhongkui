package notify

import (
	// stdlib
	"testing"

	// third-party
	"github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
)

// newSeedTestDB 创建 sqlite 内存库并迁移命名空间与通知模板表（复用 service_test 的
// newServiceTestDB 单连接 sqlite 模式）。newServiceTestDB 已迁移通知相关五表，此处补齐
// NotificationTemplate 与 Namespace 两张测试所需的表。
// newSeedTestDB creates an sqlite in-memory DB and migrates the namespace and
// notification template tables (reusing the single-connection sqlite pattern from
// newServiceTestDB). newServiceTestDB already migrates the five notification tables;
// this additionally migrates NotificationTemplate and Namespace, which the seeding test
// needs.
func newSeedTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := newServiceTestDB(t)
	if err := db.AutoMigrate(&core.NotificationTemplate{}, &core.Namespace{}); err != nil {
		t.Fatal("failed to migrate seed test database: " + err.Error())
	}
	return db
}

// expectedDefaultRows 汇总注册表在单命名空间下应种子化的默认模板行，(event, lang) 去重集合
// 用于断言。仅统计 TitleByLang 与 ContentByLang 均非空的语言。
// expectedDefaultRows aggregates, per namespace, the (event, lang) default rows expected
// from the registry, counting only languages whose TitleByLang and ContentByLang are non-empty.
func expectedDefaultRows() map[[2]string]bool {
	want := map[[2]string]bool{}
	for _, spec := range TemplateSpecs() {
		for _, lang := range spec.Languages {
			if spec.TitleByLang[lang] != "" && spec.ContentByLang[lang] != "" {
				want[[2]string{spec.Event, lang}] = true
			}
		}
	}
	return want
}

// TestSeedDefaultTemplates 测试 SeedDefaultTemplates：为命名空间幂等写入注册表中全部
// 默认模板行，二次调用不重复，且不覆盖既有用户覆盖（override）行。
// TestSeedDefaultTemplates tests SeedDefaultTemplates: idempotently seeding the full set
// of default rows for a namespace, no duplication on a second call, and no overwriting of
// an existing user override row.
func TestSeedDefaultTemplates(t *testing.T) {
	convey.Convey("SeedDefaultTemplates 默认模板种子化", t, func() {
		convey.Convey("写入每个 (event,*,lang) 一行，Source=default、Title/Content 取自注册表", func() {
			db := newSeedTestDB(t)
			convey.So(db.Create(&core.Namespace{Name: "default", Description: "default"}).Error, convey.ShouldBeNil)

			convey.So(SeedDefaultTemplates(db, "default"), convey.ShouldBeNil)

			var templates []core.NotificationTemplate
			convey.So(db.Where("namespace_name = ?", "default").Find(&templates).Error, convey.ShouldBeNil)
			want := expectedDefaultRows()
			convey.So(len(templates), convey.ShouldEqual, len(want))

			for _, spec := range TemplateSpecs() {
				for _, lang := range spec.Languages {
					if spec.TitleByLang[lang] == "" || spec.ContentByLang[lang] == "" {
						continue
					}
					var row core.NotificationTemplate
					convey.So(db.Where(
						"namespace_name = ? AND event = ? AND channel_type = ? AND language = ?",
						"default", spec.Event, "*", lang,
					).First(&row).Error, convey.ShouldBeNil)
					convey.So(row.ChannelType, convey.ShouldEqual, "*")
					convey.So(row.Source, convey.ShouldEqual, core.TemplateSourceDefault)
					convey.So(row.Title, convey.ShouldEqual, spec.TitleByLang[lang])
					convey.So(row.Content, convey.ShouldEqual, spec.ContentByLang[lang])
					convey.So(row.Description, convey.ShouldEqual, "")
				}
			}
		})

		convey.Convey("二次调用幂等，不产生重复行", func() {
			db := newSeedTestDB(t)
			convey.So(db.Create(&core.Namespace{Name: "default", Description: "default"}).Error, convey.ShouldBeNil)

			convey.So(SeedDefaultTemplates(db, "default"), convey.ShouldBeNil)
			var count int64
			convey.So(db.Model(&core.NotificationTemplate{}).Where("namespace_name = ?", "default").Count(&count).Error, convey.ShouldBeNil)
			firstCount := count

			convey.So(SeedDefaultTemplates(db, "default"), convey.ShouldBeNil)
			convey.So(db.Model(&core.NotificationTemplate{}).Where("namespace_name = ?", "default").Count(&count).Error, convey.ShouldBeNil)
			convey.So(count, convey.ShouldEqual, firstCount)
		})

		convey.Convey("已存在的用户覆盖（override）行不被回填覆盖", func() {
			db := newSeedTestDB(t)
			convey.So(db.Create(&core.Namespace{Name: "default", Description: "default"}).Error, convey.ShouldBeNil)

			// 为 zh-Hans 预置一条用户覆盖行，其标题/正文与注册表默认值不同
			spec := TemplateSpecs()[0]
			convey.So(spec.TitleByLang["zh-Hans"], convey.ShouldNotEqual, "")
			overrideTitle := "override title: {{.RuleName}}"
			overrideContent := "override content"
			convey.So(db.Create(&core.NotificationTemplate{
				NamespaceName: "default",
				Event:         spec.Event,
				ChannelType:   "*",
				Language:      "zh-Hans",
				Title:         overrideTitle,
				Content:       overrideContent,
				Source:        core.TemplateSourceOverride,
			}).Error, convey.ShouldBeNil)

			convey.So(SeedDefaultTemplates(db, "default"), convey.ShouldBeNil)

			var row core.NotificationTemplate
			convey.So(db.Where(
				"namespace_name = ? AND event = ? AND channel_type = ? AND language = ?",
				"default", spec.Event, "*", "zh-Hans",
			).First(&row).Error, convey.ShouldBeNil)
			convey.So(row.Source, convey.ShouldEqual, core.TemplateSourceOverride)
			convey.So(row.Title, convey.ShouldEqual, overrideTitle)
			convey.So(row.Content, convey.ShouldEqual, overrideContent)
		})
	})
}

// TestBackfillDefaultTemplates 测试 BackfillDefaultTemplates：遍历存量命名空间补插缺失的
// 默认模板，为每个命名空间写入注册表全部默认行；二次调用幂等不重复。
// 回填仅处理已有命名空间，不干预用户覆盖（override）行（回填即 Seed 逻辑，见
// TestSeedDefaultTemplates 的覆盖保护断言）。
// TestBackfillDefaultTemplates tests BackfillDefaultTemplates: backfilling missing default
// templates for all existing namespaces, seeding the full default set per namespace; a
// second call is idempotent with no duplication. Backfill only touches existing namespaces
// and does not interfere with override rows (backfill reuses the Seed logic whose override
// protection is asserted in TestSeedDefaultTemplates).
func TestBackfillDefaultTemplates(t *testing.T) {
	convey.Convey("BackfillDefaultTemplates 默认模板回填", t, func() {
		logger, err := logging.NewDefaultLogger()
		convey.So(err, convey.ShouldBeNil)

		convey.Convey("为每个存量命名空间写入注册表全部默认行", func() {
			db := newSeedTestDB(t)
			convey.So(db.Create(&core.Namespace{Name: "default", Description: "default"}).Error, convey.ShouldBeNil)
			convey.So(db.Create(&core.Namespace{Name: "ns2", Description: "ns2"}).Error, convey.ShouldBeNil)

			convey.So(BackfillDefaultTemplates(db, logger), convey.ShouldBeNil)

			want := len(expectedDefaultRows())
			convey.So(want, convey.ShouldBeGreaterThan, 0)
			for _, ns := range []string{"default", "ns2"} {
				var count int64
				convey.So(db.Model(&core.NotificationTemplate{}).Where("namespace_name = ?", ns).Count(&count).Error, convey.ShouldBeNil)
				convey.So(int(count), convey.ShouldEqual, want)
				var row core.NotificationTemplate
				convey.So(db.Where("namespace_name = ? AND source = ?", ns, core.TemplateSourceDefault).
					First(&row).Error, convey.ShouldBeNil)
			}
		})

		convey.Convey("二次调用幂等，不产生重复行", func() {
			db := newSeedTestDB(t)
			convey.So(db.Create(&core.Namespace{Name: "default", Description: "default"}).Error, convey.ShouldBeNil)

			convey.So(BackfillDefaultTemplates(db, logger), convey.ShouldBeNil)
			var first int64
			convey.So(db.Model(&core.NotificationTemplate{}).Count(&first).Error, convey.ShouldBeNil)

			convey.So(BackfillDefaultTemplates(db, logger), convey.ShouldBeNil)
			var second int64
			convey.So(db.Model(&core.NotificationTemplate{}).Count(&second).Error, convey.ShouldBeNil)
			convey.So(second, convey.ShouldEqual, first)
		})
	})
}
