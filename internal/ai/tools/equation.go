// 本文件是四个无状态的高阶方程计算内置工具的薄适配器：poly_solve（一元多项式求根）、
// linear_solve（线性方程组求解）、root_find（一般方程数值求根）与 nonlinear_solve（非线性方程组
// 求解）。四者均不持有任何依赖，故构造器忽略 Deps。纯算法实现已下沉到 internal/pkg/utils
// （SolvePolynomial / SolveLinearSystem / FindRoots / SolveNonlinearSystem），工具契约由 eino 的
// tool/utils.InferTool 按入参结构体生成参数 schema，并自动完成入参解析与结果序列化；本文件只保留
// 入参结构体、工具描述与调用 utils 的薄函数。工具元信息（internal/ai 的 ToolSpecs）与出厂行
// （002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"

	// 第三方
	"github.com/cloudwego/eino/components/tool"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// ============================================================================
// poly_solve：一元多项式求根
// ============================================================================

// polySolveArgs 是 poly_solve 的 JSON 入参形态：coefficients 必填；tolerance 与 maxIterations
// 可选，用指针区分「未提供」（取缺省）。
type polySolveArgs struct {
	// Coefficients 多项式系数，从最高次到最低次排列（必填）
	Coefficients []float64 `json:"coefficients" jsonschema_description:"多项式系数，从最高次到最低次排列，如 [1,0,-1] 表示 x^2-1；最高次系数不能为 0，且至少为一元方程（次数 >= 1）。必填。"`
	// Tolerance 数值求解的收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance,omitempty" jsonschema_description:"数值求解的收敛容差，可选，缺省 1e-12，必须为正。"`
	// MaxIterations 迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations,omitempty" jsonschema_description:"Durand–Kerner 迭代的最大次数，可选，缺省 100，必须不小于 1。"`
}

// polySolveTool 是内置工具 poly_solve 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var polySolveTool = mustInferTool(
	"poly_solve",
	"求一元多项式方程 P(x)=0 的全部根。coefficients 按从最高次到最低次给出系数，支持整数与"+
		"小数系数；对整系数多项式用有理根定理精确给出有理根，其余根用数值迭代在复数域求出。"+
		"返回每个根的实部、虚部、可精确表达时的有理数串与重数。",
	func(_ context.Context, in polySolveArgs) (utils.PolynomialSolution, error) {
		tolerance := utils.DefaultPolySolveTolerance
		if in.Tolerance != nil {
			tolerance = *in.Tolerance
		}
		maxIterations := utils.DefaultPolySolveMaxIterations
		if in.MaxIterations != nil {
			maxIterations = *in.MaxIterations
		}
		result, err := utils.SolvePolynomial(in.Coefficients, tolerance, maxIterations)
		if err != nil {
			return utils.PolynomialSolution{}, errors.Newf("poly_solve: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// linear_solve：线性方程组 Ax=b
// ============================================================================

// linearSolveArgs 是 linear_solve 的 JSON 入参形态：a 与 b 必填；tolerance 可选。
type linearSolveArgs struct {
	// A 系数矩阵（按行给出，必填）
	A [][]float64 `json:"a" jsonschema_description:"系数矩阵，按行给出，如 [[1,1],[1,-1]]，必须为方阵（行数等于列数）。必填。"`
	// B 常数列向量（必填）
	B []float64 `json:"b" jsonschema_description:"常数列向量，如 [0,2]，长度必须等于 a 的行数。必填。"`
	// Tolerance 判定零主元的容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance,omitempty" jsonschema_description:"判定零主元的容差，可选，缺省 1e-12，必须为正。"`
}

// linearSolveTool 是内置工具 linear_solve 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var linearSolveTool = mustInferTool(
	"linear_solve",
	"求解线性方程组 A x = b。a 为系数矩阵（按行给出，必须为方阵），b 为常数列向量。"+
		"返回解的类型（unique 唯一解 / infinite 无穷多解 / none 无解）、矩阵 A 的秩，以及相应"+
		"的解向量、特解与零空间基向量。",
	func(_ context.Context, in linearSolveArgs) (utils.LinearSystemSolution, error) {
		tolerance := utils.DefaultLinearSolveTolerance
		if in.Tolerance != nil {
			tolerance = *in.Tolerance
		}
		result, err := utils.SolveLinearSystem(in.A, in.B, tolerance)
		if err != nil {
			return utils.LinearSystemSolution{}, errors.Newf("linear_solve: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// root_find：一般方程 f(x)=0 数值求根
// ============================================================================

// rootFindArgs 是 root_find 的 JSON 入参形态：expression 必填；interval（区间）与 initial（初值）
// 至少提供一个，二者都给时合并两条模式的结果；tolerance 与 maxIterations 可选。
type rootFindArgs struct {
	// Expression 方程表达式 f(x)，变量名固定为 x（必填）
	Expression string `json:"expression" jsonschema_description:"方程表达式 f(x)，例如 x^2-4 或 sin(x)-0.5；^ 表示幂运算。必填。"`
	// Interval 扫描区间 [lo, hi]（可选，须满足 lo < hi）
	Interval *[2]float64 `json:"interval,omitempty" jsonschema_description:"扫描区间 [lo, hi]，可选，长度须为正且 lo < hi。"`
	// Initial 牛顿法迭代初值（可选）
	Initial *float64 `json:"initial,omitempty" jsonschema_description:"牛顿法迭代初值，可选。"`
	// Tolerance 收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance,omitempty" jsonschema_description:"收敛容差，可选，缺省 1e-12，必须为正。"`
	// MaxIterations 迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations,omitempty" jsonschema_description:"迭代的最大次数，可选，缺省 100，必须不小于 1。"`
}

// rootFindTool 是内置工具 root_find 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var rootFindTool = mustInferTool(
	"root_find",
	"对一般方程 f(x)=0 做数值求根，表达式中的变量名固定为 x。可给定扫描区间 interval"+
		"（在区间内扫描变号子区间并用二分法细化）或给定初值 initial（用牛顿法迭代），至少提供"+
		"其中之一，二者都给时合并两条模式的结果。支持 sin cos tan exp log sqrt abs pow 等数学函数。",
	func(_ context.Context, in rootFindArgs) (utils.RootFindSolution, error) {
		tolerance := utils.DefaultRootFindTolerance
		if in.Tolerance != nil {
			tolerance = *in.Tolerance
		}
		maxIterations := utils.DefaultRootFindMaxIterations
		if in.MaxIterations != nil {
			maxIterations = *in.MaxIterations
		}
		result, err := utils.FindRoots(in.Expression, in.Interval, in.Initial, tolerance, maxIterations)
		if err != nil {
			return utils.RootFindSolution{}, errors.Newf("root_find: %s", err.Error())
		}
		return result, nil
	},
)

// ============================================================================
// nonlinear_solve：非线性方程组
// ============================================================================

// nonlinearSolveArgs 是 nonlinear_solve 的 JSON 入参形态：equations、variables 与 initial 必填且
// 长度一致；tolerance 与 maxIterations 可选。
type nonlinearSolveArgs struct {
	// Equations 方程组，每项为形如 "x^2+y^2-1" 的表达式，约定等于 0（必填）
	Equations []string `json:"equations" jsonschema_description:"方程组表达式列表，每项约定等于 0，如 [\"x^2+y^2-1\",\"x-y\"]；^ 表示幂运算。必填。"`
	// Variables 变量名列表，如 ["x","y"]（必填）
	Variables []string `json:"variables" jsonschema_description:"变量名列表，如 [\"x\",\"y\"]，须与 equations 长度一致且互不相同。必填。"`
	// Initial 迭代初值，长度须与变量数一致（必填）
	Initial []float64 `json:"initial" jsonschema_description:"迭代初值列表，长度须与变量数一致，如 [1,1]。必填。"`
	// Tolerance 收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance,omitempty" jsonschema_description:"收敛容差，可选，缺省 1e-12，必须为正。"`
	// MaxIterations 牛顿迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations,omitempty" jsonschema_description:"牛顿迭代的最大次数，可选，缺省 100，必须不小于 1。"`
}

// nonlinearSolveTool 是内置工具 nonlinear_solve 的 eino 实现：不持有依赖，参数 schema 由入参结构体推导。
var nonlinearSolveTool = mustInferTool(
	"nonlinear_solve",
	"求解非线性方程组 F(x)=0（牛顿-拉夫逊法，雅可比矩阵用中心差分数值近似）。equations 每项"+
		"为约定等于 0 的表达式，variables 给出变量名，initial 给出迭代初值；三者长度必须一致。"+
		"支持 sin cos tan exp log sqrt abs pow 等数学函数。未在迭代上限内收敛时仍返回当前结果并"+
		"将 converged 置为 false。",
	func(_ context.Context, in nonlinearSolveArgs) (utils.NonlinearSolution, error) {
		tolerance := utils.DefaultNonlinearSolveTolerance
		if in.Tolerance != nil {
			tolerance = *in.Tolerance
		}
		maxIterations := utils.DefaultNonlinearSolveMaxIterations
		if in.MaxIterations != nil {
			maxIterations = *in.MaxIterations
		}
		result, err := utils.SolveNonlinearSystem(in.Equations, in.Variables, in.Initial, tolerance, maxIterations)
		if err != nil {
			return utils.NonlinearSolution{}, errors.Newf("nonlinear_solve: %s", err.Error())
		}
		return result, nil
	},
)

// init 向内置工具实现注册表登记 poly_solve / linear_solve / root_find / nonlinear_solve；四者均无状态，
// 不消费 Deps。
func init() {
	Register("poly_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return polySolveTool, nil
	})
	Register("linear_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return linearSolveTool, nil
	})
	Register("root_find", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return rootFindTool, nil
	})
	Register("nonlinear_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return nonlinearSolveTool, nil
	})
}
