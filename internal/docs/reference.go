package docs

import (
	// 第三方
	"github.com/spf13/cobra"
)

// buildReferenceProjections 汇总 P1 参考投影：接口、数据表、枚举、配置与 CLI。
// 所有内容均由声明派生，不读取策展目录。
func buildReferenceProjections(projectRoot string, root *cobra.Command) ([]Projection, error) {
	api, err := apiReferenceProjections()
	if err != nil {
		return nil, err
	}
	schema, err := schemaReferenceProjections(projectRoot)
	if err != nil {
		return nil, err
	}
	projections := make([]Projection, 0, len(api)+len(schema)+3)
	projections = append(projections, api...)
	projections = append(projections, schema...)
	projections = append(projections, enumReferenceProjection(), configReferenceProjection(), capabilityProjection())
	if root != nil {
		projections = append(projections, cliReferenceProjection(root))
	}
	return projections, nil
}
