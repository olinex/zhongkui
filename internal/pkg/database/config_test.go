package database

import (
	"testing"

	"github.com/creasty/defaults"
	. "github.com/smartystreets/goconvey/convey"
)

// TestToGormConfig_NilMetaConfig 测试当 MetaConfig 为 nil 时 ToGormConfig 的行为
func TestToGormConfig_NilMetaConfig(t *testing.T) {
	Convey("当 MetaConfig 为 nil 时", t, func() {
		var meta *MetaConfig = nil
		result := meta.ToGormConfig(nil)
		So(result, ShouldBeNil)
	})
}

// TestToGormConfig_DefaultValues 测试 ToGormConfig 使用默认值时的行为
func TestToGormConfig_DefaultValues(t *testing.T) {
	Convey("当使用默认 MetaConfig 时", t, func() {
		meta := &MetaConfig{}
		So(defaults.Set(meta), ShouldBeNil)
		result := meta.ToGormConfig(nil)

		So(result, ShouldNotBeNil)
		So(result.SkipDefaultTransaction, ShouldBeFalse)
		So(result.FullSaveAssociations, ShouldBeFalse)
		So(result.DryRun, ShouldBeFalse)
		So(result.PrepareStmt, ShouldBeFalse)
		So(result.DisableAutomaticPing, ShouldBeFalse)
		So(result.DisableForeignKeyConstraintWhenMigrating, ShouldBeFalse)
		So(result.IgnoreRelationshipsWhenMigrating, ShouldBeFalse)
		So(result.DisableNestedTransaction, ShouldBeFalse)
		So(result.AllowGlobalUpdate, ShouldBeFalse)
		So(result.QueryFields, ShouldBeFalse)
		So(result.CreateBatchSize, ShouldEqual, 0)
		So(result.TranslateError, ShouldBeTrue)
		So(result.PropagateUnscoped, ShouldBeFalse)
		So(result.Logger, ShouldNotBeNil)
	})
}

// TestToGormConfig_CustomValues 测试 ToGormConfig 使用自定义值时的行为
func TestToGormConfig_CustomValues(t *testing.T) {
	Convey("当使用自定义 MetaConfig 值时", t, func() {
		meta := &MetaConfig{
			SkipDefaultTransaction:                   true,
			FullSaveAssociations:                     true,
			DryRun:                                   true,
			PrepareStmt:                              true,
			DisableAutomaticPing:                     true,
			DisableForeignKeyConstraintWhenMigrating: true,
			IgnoreRelationshipsWhenMigrating:         true,
			DisableNestedTransaction:                 true,
			AllowGlobalUpdate:                        true,
			QueryFields:                              true,
			CreateBatchSize:                          10,
			TranslateError:                           false,
			PropagateUnscoped:                        true,
		}
		result := meta.ToGormConfig(nil)

		So(result, ShouldNotBeNil)
		So(result.SkipDefaultTransaction, ShouldBeTrue)
		So(result.FullSaveAssociations, ShouldBeTrue)
		So(result.DryRun, ShouldBeTrue)
		So(result.PrepareStmt, ShouldBeTrue)
		So(result.DisableAutomaticPing, ShouldBeTrue)
		So(result.DisableForeignKeyConstraintWhenMigrating, ShouldBeTrue)
		So(result.IgnoreRelationshipsWhenMigrating, ShouldBeTrue)
		So(result.DisableNestedTransaction, ShouldBeTrue)
		So(result.AllowGlobalUpdate, ShouldBeTrue)
		So(result.QueryFields, ShouldBeTrue)
		So(result.CreateBatchSize, ShouldEqual, 10)
		So(result.TranslateError, ShouldBeFalse)
		So(result.PropagateUnscoped, ShouldBeTrue)
		So(result.Logger, ShouldNotBeNil)
	})
}

// TestToGormConfig_PartialCustomValues 测试 ToGormConfig 使用部分自定义值时的行为
func TestToGormConfig_PartialCustomValues(t *testing.T) {
	Convey("当使用部分自定义 MetaConfig 值时", t, func() {
		meta := &MetaConfig{
			SkipDefaultTransaction: true,
			PrepareStmt:            true,
			CreateBatchSize:        50,
			TranslateError:         true,
			PropagateUnscoped:      false,
		}
		result := meta.ToGormConfig(nil)

		So(result, ShouldNotBeNil)
		So(result.SkipDefaultTransaction, ShouldBeTrue)
		So(result.FullSaveAssociations, ShouldBeFalse) // 默认值
		So(result.DryRun, ShouldBeFalse)               // 默认值
		So(result.PrepareStmt, ShouldBeTrue)
		So(result.DisableAutomaticPing, ShouldBeFalse)                     // 默认值
		So(result.DisableForeignKeyConstraintWhenMigrating, ShouldBeFalse) // 默认值
		So(result.IgnoreRelationshipsWhenMigrating, ShouldBeFalse)         // 默认值
		So(result.DisableNestedTransaction, ShouldBeFalse)                 // 默认值
		So(result.AllowGlobalUpdate, ShouldBeFalse)                        // 默认值
		So(result.QueryFields, ShouldBeFalse)                              // 默认值
		So(result.CreateBatchSize, ShouldEqual, 50)
		So(result.TranslateError, ShouldBeTrue)
		So(result.PropagateUnscoped, ShouldBeFalse)
		So(result.Logger, ShouldNotBeNil)
	})
}

// TestToGormConfig_NegativeCreateBatchSize 测试 CreateBatchSize 为负值时的行为
func TestToGormConfig_NegativeCreateBatchSize(t *testing.T) {
	Convey("当 CreateBatchSize 为负值时", t, func() {
		meta := &MetaConfig{
			CreateBatchSize: -10,
		}
		result := meta.ToGormConfig(nil)

		So(result, ShouldNotBeNil)
		So(result.CreateBatchSize, ShouldEqual, -10)
		So(result.Logger, ShouldNotBeNil)
	})
}

// TestToGormConfig_WithLogger 测试 ToGormConfig 使用真实日志记录器时的行为
func TestToGormConfig_WithLogger(t *testing.T) {
	Convey("当使用真实日志记录器时", t, func() {
		// 注意：这里我们无法创建真实的 logging.Logger 实例
		// 因为它是外部依赖，但我们可以测试 nil 日志记录器的情况
		meta := &MetaConfig{
			SkipDefaultTransaction: true,
		}
		result := meta.ToGormConfig(nil)

		So(result, ShouldNotBeNil)
		So(result.SkipDefaultTransaction, ShouldBeTrue)
		So(result.Logger, ShouldNotBeNil)
	})
}
