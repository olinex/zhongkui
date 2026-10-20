// 本文件是四个无状态的高阶方程计算内置工具的薄适配器：poly_solve（一元多项式求根）、
// linear_solve（线性方程组求解）、root_find（一般方程数值求根）与 nonlinear_solve（非线性方程组
// 求解）。四者均不持有任何依赖，故构造器忽略 Deps。纯算法实现已下沉到 internal/pkg/utils
// （SolvePolynomial / SolveLinearSystem / FindRoots / SolveNonlinearSystem），本文件只保留入参结构体、
// Info（含中文 Desc/schema）、init 期登记与 InvokableRun 骨架：解析入参 JSON、用 utils 的 Default*
// 常量填可选缺省值、调用 utils 导出函数、出错时补上工具名前缀并以 JSON 序列化结果。工具元信息
// （internal/ai 的 ToolSpecs）与出厂行（002_fixtures.sql）由另外两处提供。

package tools

import (
	// 标准库
	"context"
	"encoding/json"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

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
	Coefficients []float64 `json:"coefficients"`
	// Tolerance 数值求解的收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance"`
	// MaxIterations 迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations"`
}

// polySolveTool 是内置工具 poly_solve 的无状态实现：不持有依赖。
type polySolveTool struct{}

// Info 返回 poly_solve 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// coefficients（number 数组）必填，tolerance（number）与 maxIterations（integer）可选。
func (t *polySolveTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "poly_solve",
		Desc: "求一元多项式方程 P(x)=0 的全部根。coefficients 按从最高次到最低次给出系数，支持整数与" +
			"小数系数；对整系数多项式用有理根定理精确给出有理根，其余根用数值迭代在复数域求出。" +
			"返回每个根的实部、虚部、可精确表达时的有理数串与重数。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"coefficients": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.Number},
				Desc: "多项式系数，从最高次到最低次排列，如 [1,0,-1] 表示 x^2-1；最高次系数不能为 0，" +
					"且至少为一元方程（次数 >= 1）。必填。",
				Required: true,
			},
			"tolerance": {
				Type: schema.Number,
				Desc: "数值求解的收敛容差，可选，缺省 1e-12，必须为正。",
			},
			"maxIterations": {
				Type: schema.Integer,
				Desc: "Durand–Kerner 迭代的最大次数，可选，缺省 100，必须不小于 1。",
			},
		}),
	}, nil
}

// InvokableRun 执行 poly_solve：解析入参并用 utils 缺省填充可选值，调用 utils.SolvePolynomial 求根，
// 返回形如 {"degree":2,"roots":[{"real":-1,"imag":0,"exact":"-1","multiplicity":1}, ...]} 的 JSON
// 字符串。入参非法时返回英文错误（工具名前缀在此补齐）。
func (t *polySolveTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args polySolveArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	tolerance := utils.DefaultPolySolveTolerance
	if args.Tolerance != nil {
		tolerance = *args.Tolerance
	}
	maxIterations := utils.DefaultPolySolveMaxIterations
	if args.MaxIterations != nil {
		maxIterations = *args.MaxIterations
	}
	result, err := utils.SolvePolynomial(args.Coefficients, tolerance, maxIterations)
	if err != nil {
		return "", errors.Newf("poly_solve: %s", err.Error())
	}
	return equationEncodeResult(result)
}

// ============================================================================
// linear_solve：线性方程组 Ax=b
// ============================================================================

// linearSolveArgs 是 linear_solve 的 JSON 入参形态：a 与 b 必填；tolerance 可选。
type linearSolveArgs struct {
	// A 系数矩阵（按行给出，必填）
	A [][]float64 `json:"a"`
	// B 常数列向量（必填）
	B []float64 `json:"b"`
	// Tolerance 判定零主元的容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance"`
}

// linearSolveTool 是内置工具 linear_solve 的无状态实现：不持有依赖。
type linearSolveTool struct{}

// Info 返回 linear_solve 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// a（number 二维数组）与 b（number 数组）必填，tolerance（number）可选。
func (t *linearSolveTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "linear_solve",
		Desc: "求解线性方程组 A x = b。a 为系数矩阵（按行给出，必须为方阵），b 为常数列向量。" +
			"返回解的类型（unique 唯一解 / infinite 无穷多解 / none 无解）、矩阵 A 的秩，以及相应" +
			"的解向量、特解与零空间基向量。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"a": {
				Type: schema.Array,
				ElemInfo: &schema.ParameterInfo{
					Type:     schema.Array,
					ElemInfo: &schema.ParameterInfo{Type: schema.Number},
				},
				Desc:     "系数矩阵，按行给出，如 [[1,1],[1,-1]]，必须为方阵（行数等于列数）。必填。",
				Required: true,
			},
			"b": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.Number},
				Desc:     "常数列向量，如 [0,2]，长度必须等于 a 的行数。必填。",
				Required: true,
			},
			"tolerance": {
				Type: schema.Number,
				Desc: "判定零主元的容差，可选，缺省 1e-12，必须为正。",
			},
		}),
	}, nil
}

// InvokableRun 执行 linear_solve：解析入参并用 utils 缺省填充可选值，调用 utils.SolveLinearSystem，
// 返回形如 {"solutionType":"unique","rank":2,"solution":[1,-1],"particular":[],"nullspace":[]} 的
// JSON 字符串。入参非法时返回英文错误（工具名前缀在此补齐）。
func (t *linearSolveTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args linearSolveArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	tolerance := utils.DefaultLinearSolveTolerance
	if args.Tolerance != nil {
		tolerance = *args.Tolerance
	}
	result, err := utils.SolveLinearSystem(args.A, args.B, tolerance)
	if err != nil {
		return "", errors.Newf("linear_solve: %s", err.Error())
	}
	return equationEncodeResult(result)
}

// ============================================================================
// root_find：一般方程 f(x)=0 数值求根
// ============================================================================

// rootFindArgs 是 root_find 的 JSON 入参形态：expression 必填；interval（区间）与 initial（初值）
// 至少提供一个，二者都给时合并两条模式的结果；tolerance 与 maxIterations 可选。
type rootFindArgs struct {
	// Expression 方程表达式 f(x)，变量名固定为 x（必填）
	Expression string `json:"expression"`
	// Interval 扫描区间 [lo, hi]（可选，须满足 lo < hi）
	Interval *[2]float64 `json:"interval"`
	// Initial 牛顿法迭代初值（可选）
	Initial *float64 `json:"initial"`
	// Tolerance 收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance"`
	// MaxIterations 迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations"`
}

// rootFindTool 是内置工具 root_find 的无状态实现：不持有依赖。
type rootFindTool struct{}

// Info 返回 root_find 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// expression（string）必填，interval / initial 至少提供一个，tolerance 与 maxIterations 可选。
func (t *rootFindTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "root_find",
		Desc: "对一般方程 f(x)=0 做数值求根，表达式中的变量名固定为 x。可给定扫描区间 interval" +
			"（在区间内扫描变号子区间并用二分法细化）或给定初值 initial（用牛顿法迭代），至少提供" +
			"其中之一，二者都给时合并两条模式的结果。支持 sin cos tan exp log sqrt abs pow 等数学函数。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"expression": {
				Type:     schema.String,
				Desc:     "方程表达式 f(x)，例如 x^2-4 或 sin(x)-0.5；^ 表示幂运算。必填。",
				Required: true,
			},
			"interval": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.Number},
				Desc:     "扫描区间 [lo, hi]，可选，长度须为正且 lo < hi。",
			},
			"initial": {
				Type: schema.Number,
				Desc: "牛顿法迭代初值，可选。",
			},
			"tolerance": {
				Type: schema.Number,
				Desc: "收敛容差，可选，缺省 1e-12，必须为正。",
			},
			"maxIterations": {
				Type: schema.Integer,
				Desc: "迭代的最大次数，可选，缺省 100，必须不小于 1。",
			},
		}),
	}, nil
}

// InvokableRun 执行 root_find：解析入参并用 utils 缺省填充可选值，调用 utils.FindRoots，返回形如
// {"roots":[{"root":-2,"fValue":0,"method":"bisection"}, ...]} 的 JSON 字符串。入参非法时返回英文
// 错误（工具名前缀在此补齐）。
func (t *rootFindTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args rootFindArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	tolerance := utils.DefaultRootFindTolerance
	if args.Tolerance != nil {
		tolerance = *args.Tolerance
	}
	maxIterations := utils.DefaultRootFindMaxIterations
	if args.MaxIterations != nil {
		maxIterations = *args.MaxIterations
	}
	result, err := utils.FindRoots(args.Expression, args.Interval, args.Initial, tolerance, maxIterations)
	if err != nil {
		return "", errors.Newf("root_find: %s", err.Error())
	}
	return equationEncodeResult(result)
}

// ============================================================================
// nonlinear_solve：非线性方程组
// ============================================================================

// nonlinearSolveArgs 是 nonlinear_solve 的 JSON 入参形态：equations、variables 与 initial 必填且
// 长度一致；tolerance 与 maxIterations 可选。
type nonlinearSolveArgs struct {
	// Equations 方程组，每项为形如 "x^2+y^2-1" 的表达式，约定等于 0（必填）
	Equations []string `json:"equations"`
	// Variables 变量名列表，如 ["x","y"]（必填）
	Variables []string `json:"variables"`
	// Initial 迭代初值，长度须与变量数一致（必填）
	Initial []float64 `json:"initial"`
	// Tolerance 收敛容差（可选，缺省 1e-12）
	Tolerance *float64 `json:"tolerance"`
	// MaxIterations 牛顿迭代的最大次数（可选，缺省 100）
	MaxIterations *int `json:"maxIterations"`
}

// nonlinearSolveTool 是内置工具 nonlinear_solve 的无状态实现：不持有依赖。
type nonlinearSolveTool struct{}

// Info 返回 nonlinear_solve 的工具元信息与参数 JSON schema。参数经 eino 的 ParameterInfo 形式描述，
// equations（string 数组）、variables（string 数组）与 initial（number 数组）均必填。
func (t *nonlinearSolveTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "nonlinear_solve",
		Desc: "求解非线性方程组 F(x)=0（牛顿-拉夫逊法，雅可比矩阵用中心差分数值近似）。equations 每项" +
			"为约定等于 0 的表达式，variables 给出变量名，initial 给出迭代初值；三者长度必须一致。" +
			"支持 sin cos tan exp log sqrt abs pow 等数学函数。未在迭代上限内收敛时仍返回当前结果并" +
			"将 converged 置为 false。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"equations": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "方程组表达式列表，每项约定等于 0，如 [\"x^2+y^2-1\",\"x-y\"]；^ 表示幂运算。必填。",
				Required: true,
			},
			"variables": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.String},
				Desc:     "变量名列表，如 [\"x\",\"y\"]，须与 equations 长度一致且互不相同。必填。",
				Required: true,
			},
			"initial": {
				Type:     schema.Array,
				ElemInfo: &schema.ParameterInfo{Type: schema.Number},
				Desc:     "迭代初值列表，长度须与变量数一致，如 [1,1]。必填。",
				Required: true,
			},
			"tolerance": {
				Type: schema.Number,
				Desc: "收敛容差，可选，缺省 1e-12，必须为正。",
			},
			"maxIterations": {
				Type: schema.Integer,
				Desc: "牛顿迭代的最大次数，可选，缺省 100，必须不小于 1。",
			},
		}),
	}, nil
}

// InvokableRun 执行 nonlinear_solve：解析入参并用 utils 缺省填充可选值，调用
// utils.SolveNonlinearSystem，返回形如
// {"solution":{"x":0.7071,"y":0.7071},"residual":0,"iterations":4,"converged":true} 的 JSON 字符串。
// 入参非法或雅可比奇异时返回英文错误（工具名前缀在此补齐）；迭代上限内未收敛不报错，仅置
// converged=false。
func (t *nonlinearSolveTool) InvokableRun(_ context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args nonlinearSolveArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", errors.WithStack(err)
	}
	tolerance := utils.DefaultNonlinearSolveTolerance
	if args.Tolerance != nil {
		tolerance = *args.Tolerance
	}
	maxIterations := utils.DefaultNonlinearSolveMaxIterations
	if args.MaxIterations != nil {
		maxIterations = *args.MaxIterations
	}
	result, err := utils.SolveNonlinearSystem(args.Equations, args.Variables, args.Initial, tolerance, maxIterations)
	if err != nil {
		return "", errors.Newf("nonlinear_solve: %s", err.Error())
	}
	return equationEncodeResult(result)
}

// equationEncodeResult 把 utils 返回的结果结构体序列化为紧凑 JSON 字符串；失败时返回带堆栈错误。
func equationEncodeResult(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return string(out), nil
}

// init 向内置工具实现注册表登记 poly_solve / linear_solve / root_find / nonlinear_solve；四者均无状态，
// 不消费 Deps。
func init() {
	Register("poly_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &polySolveTool{}, nil
	})
	Register("linear_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &linearSolveTool{}, nil
	})
	Register("root_find", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &rootFindTool{}, nil
	})
	Register("nonlinear_solve", func(_ context.Context, _ Deps) (tool.BaseTool, error) {
		return &nonlinearSolveTool{}, nil
	})
}
