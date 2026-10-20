package utils

// 本文件承载四个与业务无关的纯计算能力的算法实现：SolvePolynomial（一元多项式求根）、
// SolveLinearSystem（线性方程组求解）、FindRoots（一般方程数值求根）与 SolveNonlinearSystem
// （非线性方程组求解）。它们只依赖标准库与既有第三方 github.com/casbin/govaluate，不依赖 eino
// 等上层组件；internal/ai/tools 中的同名工具由此处的导出函数驱动，仅负责入参 JSON 解析、可选
// 缺省值填充（Default* 常量）与结果序列化。
//
// 数值算法：多项式求根用有理根定理精确提取有理根并对剩余多项式用 Durand–Kerner 迭代求复根；
// 线性 / 非线性求解用带部分主元的高斯-约当消元；一般方程与方程组用二分法 / 牛顿法加中心差分
// 雅可比。
//
// 错误信息一律为英文、描述性、不带工具名前缀（工具名前缀由适配器补齐），故最终对外字符串与
// 重构前保持一致。

import (
	// 标准库
	"math"
	"math/big"
	"math/cmplx"
	"sort"
	"strings"

	// 第三方
	"github.com/casbin/govaluate"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// ============================================================================
// 缺省值常量（供适配器填充可选入参）与共享固定阈值
// ============================================================================

// 各工具的缺省容差与迭代上限，供适配器在可选入参缺省时填充。
const (
	// DefaultPolySolveTolerance SolvePolynomial 的缺省收敛容差
	DefaultPolySolveTolerance = 1e-12
	// DefaultPolySolveMaxIterations SolvePolynomial 的缺省迭代上限
	DefaultPolySolveMaxIterations = 100
	// DefaultLinearSolveTolerance SolveLinearSystem 的缺省容差
	DefaultLinearSolveTolerance = 1e-12
	// DefaultRootFindTolerance FindRoots 的缺省容差
	DefaultRootFindTolerance = 1e-12
	// DefaultRootFindMaxIterations FindRoots 的缺省迭代上限
	DefaultRootFindMaxIterations = 100
	// DefaultNonlinearSolveTolerance SolveNonlinearSystem 的缺省容差
	DefaultNonlinearSolveTolerance = 1e-12
	// DefaultNonlinearSolveMaxIterations SolveNonlinearSystem 的缺省迭代上限
	DefaultNonlinearSolveMaxIterations = 100
)

// 多项式精确提取 / 数值迭代用到的固定阈值。
const (
	// polyMaxExactCoefficient 走有理根精确提取的系数绝对值上限，超过即退化为纯数值求根
	polyMaxExactCoefficient = 1e9
	// polyExactEpsilon 判定系数「整数值」的相对误差阈值
	polyExactEpsilon = 1e-9
	// polyMaxFactorizeIterations 因子枚举的试除迭代预算，防止极端系数导致卡死
	polyMaxFactorizeIterations = 1_000_000

	// rootFindSubintervals FindRoots 区间扫描时把区间切分的子区间数
	rootFindSubintervals = 1000
	// rootFindMergeTolerance 合并不同模式 / 相邻子区间得到的重复根的相对阈值
	rootFindMergeTolerance = 1e-6
	// rootNewtonStep 中心差分求数值导数时使用的相对步长
	rootNewtonStep = 1e-7
)

// ============================================================================
// 多项式求根：SolvePolynomial
// ============================================================================

// PolynomialRoot 是多项式求根输出中的单个根：实部 / 虚部、可精确表达时的有理数串与重数。
type PolynomialRoot struct {
	// Real 根的实部
	Real float64 `json:"real"`
	// Imag 根的虚部
	Imag float64 `json:"imag"`
	// Exact 有理根的精确表示（如 "1"、"-1/2"）；复根为空串
	Exact string `json:"exact"`
	// Multiplicity 该根的重数
	Multiplicity int `json:"multiplicity"`
}

// PolynomialSolution 是多项式求根成功时返回的载荷。
type PolynomialSolution struct {
	// Degree 去掉前导零系数后的多项式次数
	Degree int `json:"degree"`
	// Roots 全部根，按实部升序、再按虚部升序排列
	Roots []PolynomialRoot `json:"roots"`
}

// polyRootGroup 是有理根的分组累积：同一个精确有理数被多次提取即累加重数。
type polyRootGroup struct {
	rat   *big.Rat
	count int
}

// SolvePolynomial 求一元多项式 P(x)=0 的全部根。coefficients 从最高次到最低次排列；去掉前导零
// 系数得到真实次数后，若系数可表示为整数则用有理根定理精确提取有理根（含重数）并对约化多项式用
// Durand–Kerner 求复根。容差非正、迭代上限小于 1、系数为空 / 全零或次数小于 1 时返回英文错误。
func SolvePolynomial(coefficients []float64, tolerance float64, maxIterations int) (PolynomialSolution, error) {
	if tolerance <= 0 {
		return PolynomialSolution{}, errors.Newf("tolerance %g must be positive", tolerance)
	}
	if maxIterations < 1 {
		return PolynomialSolution{}, errors.Newf("maxIterations %d must be at least 1", maxIterations)
	}
	if len(coefficients) == 0 {
		return PolynomialSolution{}, errors.New("coefficients must be non-empty")
	}
	coeffs := make([]float64, 0, len(coefficients))
	coeffs = append(coeffs, coefficients...)
	for len(coeffs) > 0 && coeffs[0] == 0 {
		coeffs = coeffs[1:]
	}
	if len(coeffs) == 0 {
		return PolynomialSolution{}, errors.New("coefficients must not be all zero")
	}
	degree := len(coeffs) - 1
	if degree < 1 {
		return PolynomialSolution{}, errors.Newf("polynomial degree %d is less than 1, expected a univariate equation", degree)
	}
	groups := make([]polyRootGroup, 0)
	groupIndex := make(map[string]int)
	reduced := coeffs
	if ints, ok := polyIntegerCoefficients(coeffs); ok {
		rationalRoots, reducedInts := polyExtractRationalRoots(ints)
		for _, r := range rationalRoots {
			key := polyRationalExactString(r)
			if idx, exists := groupIndex[key]; exists {
				groups[idx].count++
				continue
			}
			groupIndex[key] = len(groups)
			groups = append(groups, polyRootGroup{rat: new(big.Rat).Set(r), count: 1})
		}
		reduced = polyIntCoeffsToFloats(reducedInts)
	}
	roots := make([]PolynomialRoot, 0, degree)
	for _, g := range groups {
		value, _ := g.rat.Float64()
		roots = append(roots, PolynomialRoot{Real: value, Imag: 0, Exact: polyRationalExactString(g.rat), Multiplicity: g.count})
	}
	if len(reduced) >= 2 {
		for _, z := range polyDurandKerner(reduced, tolerance, maxIterations) {
			roots = append(roots, PolynomialRoot{Real: real(z), Imag: imag(z), Exact: "", Multiplicity: 1})
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if roots[i].Real != roots[j].Real {
			return roots[i].Real < roots[j].Real
		}
		return roots[i].Imag < roots[j].Imag
	})
	return PolynomialSolution{Degree: degree, Roots: roots}, nil
}

// polyComplexIPow 计算复数的整数次幂（base 的 k 次方，k>=0），用于 Durand–Kerner 的初始根生成。
func polyComplexIPow(base complex128, k int) complex128 {
	result := complex(1, 0)
	for i := 0; i < k; i++ {
		result *= base
	}
	return result
}

// polyEvalComplex 用 Horner 法在复数点 z 处求多项式的值；系数从最高次到最低次排列。
func polyEvalComplex(coeffs []float64, z complex128) complex128 {
	acc := complex(0, 0)
	for _, c := range coeffs {
		acc = acc*z + complex(c, 0)
	}
	return acc
}

// polyDurandKerner 用 Durand–Kerner 迭代在复数域求多项式的全部根。多项式系数从最高次到最低次
// 排列，初始根取 (0.4+0.9i)^k 以避免重合；迭代到增量小于 tolerance 或达到 maxIterations。未收敛
// 时按当前近似值返回（不报错）。
func polyDurandKerner(coeffs []float64, tolerance float64, maxIterations int) []complex128 {
	degree := len(coeffs) - 1
	if degree < 1 {
		return []complex128{}
	}
	monic := make([]float64, len(coeffs))
	lead := coeffs[0]
	for i, c := range coeffs {
		monic[i] = c / lead
	}
	roots := make([]complex128, degree)
	base := complex(0.4, 0.9)
	for k := 0; k < degree; k++ {
		roots[k] = polyComplexIPow(base, k)
	}
	for iter := 0; iter < maxIterations; iter++ {
		maxDelta := 0.0
		for k := 0; k < degree; k++ {
			num := polyEvalComplex(monic, roots[k])
			den := complex(1, 0)
			for j := 0; j < degree; j++ {
				if j == k {
					continue
				}
				den *= roots[k] - roots[j]
			}
			if den == 0 {
				continue
			}
			delta := num / den
			roots[k] -= delta
			if d := cmplx.Abs(delta); d > maxDelta {
				maxDelta = d
			}
		}
		if maxDelta <= tolerance {
			break
		}
	}
	return roots
}

// polyEvalRat 用 Horner 法在有理点 x 处精确求整系数多项式的值；系数从最高次到最低次排列。
func polyEvalRat(coeffs []*big.Int, x *big.Rat) *big.Rat {
	acc := new(big.Rat)
	for _, c := range coeffs {
		acc.Mul(acc, x)
		acc.Add(acc, new(big.Rat).SetInt(c))
	}
	return acc
}

// polySyntheticDivideLinear 用综合除法把整系数多项式除以一次因式 (x - root)，返回商的系数
// （有理数，从最高次到最低次排列）。调用前须确认 root 确为根（余数为 0）。
func polySyntheticDivideLinear(coeffs []*big.Int, root *big.Rat) []*big.Rat {
	quotient := make([]*big.Rat, 0, len(coeffs))
	acc := new(big.Rat)
	for _, c := range coeffs {
		acc.Mul(acc, root)
		acc.Add(acc, new(big.Rat).SetInt(c))
		quotient = append(quotient, new(big.Rat).Set(acc))
	}
	return quotient[:len(quotient)-1]
}

// polyRationalExactString 返回有理数的规范字符串表示：分母为 1 时只给出分子（如 "1"、"-2"），否则
// 给出 "分子/分母"（如 "-1/2"），与工具 exact 字段的约定一致（math/big 自带的 String 会把整数写成
// "1/1"）。
func polyRationalExactString(r *big.Rat) string {
	if r.Denom().Cmp(big.NewInt(1)) == 0 {
		return r.Num().String()
	}
	return r.Num().String() + "/" + r.Denom().String()
}

// polyRationalsToIntCoeffs 把有理数系数化为整系数（乘各分母的最小公倍数后再除以所有系数的最大
// 公约数），保持多项式根不变，供下一轮有理根候选重新生成使用。
func polyRationalsToIntCoeffs(rats []*big.Rat) []*big.Int {
	lcm := big.NewInt(1)
	for _, r := range rats {
		d := r.Denom()
		g := new(big.Int).GCD(nil, nil, lcm, d)
		lcm.Quo(lcm, g)
		lcm.Mul(lcm, d)
	}
	ints := make([]*big.Int, len(rats))
	gcdAll := new(big.Int)
	for i, r := range rats {
		v := new(big.Int).Mul(r.Num(), new(big.Int).Quo(lcm, r.Denom()))
		ints[i] = v
		gcdAll.GCD(nil, nil, gcdAll, new(big.Int).Abs(v))
	}
	if gcdAll.Cmp(big.NewInt(1)) > 0 {
		for i := range ints {
			ints[i].Quo(ints[i], gcdAll)
		}
	}
	return ints
}

// polyFactorizePositive 用试除法枚举正整数 n 的全部正因子；消耗 budget 次试除迭代，预算耗尽时
// 返回 ok=false（调用方据此放弃精确提取，避免极端系数导致卡死）。
func polyFactorizePositive(n int64, budget *int) ([]int64, bool) {
	if n < 0 {
		n = -n
	}
	divisors := make([]int64, 0)
	if n == 0 {
		return divisors, true
	}
	for i := int64(1); i*i <= n; i++ {
		*budget = *budget - 1
		if *budget <= 0 {
			return divisors, false
		}
		if n%i == 0 {
			divisors = append(divisors, i)
			if q := n / i; q != i {
				divisors = append(divisors, q)
			}
		}
	}
	return divisors, true
}

// polyRationalRootCandidates 依据有理根定理生成候选有理根 p/q：p 枚举常数项的因子（含正负号），
// q 枚举首项系数的正因子。常数项为 0 时仅返回 0（提取后重新生成）。任一系数超出 int64 表示范围
// 或迭代预算耗尽时返回 ok=false。
func polyRationalRootCandidates(coeffs []*big.Int, budget *int) ([]*big.Rat, bool) {
	n := len(coeffs)
	leading := coeffs[0]
	constant := coeffs[n-1]
	if constant.Sign() == 0 {
		return []*big.Rat{big.NewRat(0, 1)}, true
	}
	if !leading.IsInt64() || !constant.IsInt64() {
		return nil, false
	}
	ps, ok := polyFactorizePositive(constant.Int64(), budget)
	if !ok {
		return nil, false
	}
	qs, ok := polyFactorizePositive(leading.Int64(), budget)
	if !ok {
		return nil, false
	}
	seen := make(map[string]struct{})
	candidates := make([]*big.Rat, 0, 2*len(ps)*len(qs))
	for _, p := range ps {
		for _, q := range qs {
			for _, sign := range []int64{1, -1} {
				r := big.NewRat(sign*p, q)
				key := polyRationalExactString(r)
				if _, dup := seen[key]; dup {
					continue
				}
				seen[key] = struct{}{}
				candidates = append(candidates, r)
			}
		}
	}
	return candidates, true
}

// polyExtractRationalRoots 反复用有理根定理从整系数多项式精确提取有理根：每命中一个有理根即用
// 综合除法降次并保持整系数后再重新生成候选，从而同一有理根的多重出现会被逐一提取（供重数统计）。
// 返回提取到的有理根序列（按提取顺序，含重复）与约化后的整系数多项式。
func polyExtractRationalRoots(intCoeffs []*big.Int) ([]*big.Rat, []*big.Int) {
	roots := make([]*big.Rat, 0)
	coeffs := make([]*big.Int, len(intCoeffs))
	copy(coeffs, intCoeffs)
	budget := polyMaxFactorizeIterations
	for {
		for len(coeffs) > 1 && coeffs[0].Sign() == 0 {
			coeffs = coeffs[1:]
		}
		if len(coeffs) < 2 {
			break
		}
		candidates, ok := polyRationalRootCandidates(coeffs, &budget)
		if !ok {
			break
		}
		found := false
		for _, r := range candidates {
			if polyEvalRat(coeffs, r).Sign() == 0 {
				coeffs = polyRationalsToIntCoeffs(polySyntheticDivideLinear(coeffs, r))
				roots = append(roots, new(big.Rat).Set(r))
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return roots, coeffs
}

// polyIntegerCoefficients 尝试把浮点系数转换为整数值的 big.Int 表示：任一系数非有限、绝对值超过
// polyMaxExactCoefficient 或并非「整数值」（在相对误差阈值内）时返回 ok=false，调用方据此跳过精确
// 有理根提取、直接走数值求根。
func polyIntegerCoefficients(coeffs []float64) ([]*big.Int, bool) {
	ints := make([]*big.Int, 0, len(coeffs))
	for _, c := range coeffs {
		if math.IsNaN(c) || math.IsInf(c, 0) || math.Abs(c) > polyMaxExactCoefficient {
			return nil, false
		}
		rounded := math.Round(c)
		if math.Abs(c-rounded) > polyExactEpsilon*math.Max(1, math.Abs(c)) {
			return nil, false
		}
		ints = append(ints, big.NewInt(int64(rounded)))
	}
	return ints, true
}

// polyIntCoeffsToFloats 把整系数多项式转换为 float64 系数，供 Durand–Kerner 数值求根使用。
func polyIntCoeffsToFloats(ints []*big.Int) []float64 {
	out := make([]float64, len(ints))
	for i, v := range ints {
		out[i], _ = new(big.Rat).SetInt(v).Float64()
	}
	return out
}

// ============================================================================
// 线性方程组求解：SolveLinearSystem
// ============================================================================

// LinearSystemSolution 是线性方程组求解成功时返回的载荷：SolutionType 取 unique|infinite|none；
// unique 给 solution，infinite 给 particular 与 nullspace，none 时各向量为空。
type LinearSystemSolution struct {
	// SolutionType 解的类型：unique / infinite / none
	SolutionType string `json:"solutionType"`
	// Rank 矩阵 A 的秩
	Rank int `json:"rank"`
	// Solution 唯一解向量（非唯一解时为空数组）
	Solution []float64 `json:"solution"`
	// Particular 无穷多解的一个特解（自由变量取 0；非无穷多解时为空数组）
	Particular []float64 `json:"particular"`
	// Nullspace 零空间的基向量，每个自由变量对应一个（非无穷多解时为空数组）
	Nullspace [][]float64 `json:"nullspace"`
}

// SolveLinearSystem 求解线性方程组 a x = b。a 为系数矩阵（按行给出，必须为方阵），b 为常数列
// 向量。返回解的类型（unique / infinite / none）、矩阵 A 的秩，以及相应的解向量、特解与零空间基
// 向量。容差非正、矩阵为空 / 非方阵或 b 长度不符时返回英文错误。
func SolveLinearSystem(a [][]float64, b []float64, tolerance float64) (LinearSystemSolution, error) {
	if tolerance <= 0 {
		return LinearSystemSolution{}, errors.Newf("tolerance %g must be positive", tolerance)
	}
	if len(a) == 0 {
		return LinearSystemSolution{}, errors.New("matrix a must be non-empty")
	}
	rows := len(a)
	for _, row := range a {
		if len(row) != rows {
			return LinearSystemSolution{}, errors.Newf("matrix a must be square, got %d rows and %d columns", rows, len(row))
		}
	}
	if len(b) != rows {
		return LinearSystemSolution{}, errors.Newf("b length %d must equal the number of rows %d", len(b), rows)
	}
	cols := rows
	aug := make([][]float64, rows)
	for i := range aug {
		aug[i] = make([]float64, cols+1)
		copy(aug[i], a[i])
		aug[i][cols] = b[i]
	}
	pivots := linRowReduce(aug, cols, tolerance)
	for i := 0; i < rows; i++ {
		allZero := true
		for j := 0; j < cols; j++ {
			if math.Abs(aug[i][j]) > tolerance {
				allZero = false
				break
			}
		}
		if allZero && math.Abs(aug[i][cols]) > tolerance {
			return LinearSystemSolution{
				SolutionType: "none",
				Rank:         len(pivots),
				Solution:     make([]float64, 0),
				Particular:   make([]float64, 0),
				Nullspace:    make([][]float64, 0),
			}, nil
		}
	}
	if len(pivots) == cols {
		solution := make([]float64, cols)
		for k, c := range pivots {
			solution[c] = aug[k][cols]
		}
		return LinearSystemSolution{
			SolutionType: "unique",
			Rank:         len(pivots),
			Solution:     solution,
			Particular:   make([]float64, 0),
			Nullspace:    make([][]float64, 0),
		}, nil
	}
	particular := make([]float64, cols)
	for k, c := range pivots {
		particular[c] = aug[k][cols]
	}
	freeCols := make([]int, 0, cols-len(pivots))
	for c := 0; c < cols; c++ {
		isPivot := false
		for _, pc := range pivots {
			if pc == c {
				isPivot = true
				break
			}
		}
		if !isPivot {
			freeCols = append(freeCols, c)
		}
	}
	nullspace := make([][]float64, 0, len(freeCols))
	for _, f := range freeCols {
		vec := make([]float64, cols)
		vec[f] = 1
		for k, c := range pivots {
			vec[c] = -aug[k][f]
		}
		nullspace = append(nullspace, vec)
	}
	return LinearSystemSolution{
		SolutionType: "infinite",
		Rank:         len(pivots),
		Solution:     make([]float64, 0),
		Particular:   particular,
		Nullspace:    nullspace,
	}, nil
}

// linRowReduce 对增广矩阵做带部分主元的高斯-约当消元，就地化为简化行阶梯形（RREF）。aug 为
// rows x (cols+1) 的矩阵（最后一列为常数列），cols 为系数矩阵的列数；绝对值不超过 tolerance 的
// 主元视为零。返回主元所在列的序号（即矩阵 A 的主元列集合）。
func linRowReduce(aug [][]float64, cols int, tolerance float64) []int {
	rows := len(aug)
	pivots := make([]int, 0, cols)
	r := 0
	for c := 0; c < cols && r < rows; c++ {
		pivotRow := -1
		best := tolerance
		for i := r; i < rows; i++ {
			if v := math.Abs(aug[i][c]); v > best {
				best = v
				pivotRow = i
			}
		}
		if pivotRow == -1 {
			continue
		}
		aug[r], aug[pivotRow] = aug[pivotRow], aug[r]
		p := aug[r][c]
		for j := c; j <= cols; j++ {
			aug[r][j] /= p
		}
		for i := 0; i < rows; i++ {
			if i == r {
				continue
			}
			f := aug[i][c]
			if f == 0 {
				continue
			}
			for j := c; j <= cols; j++ {
				aug[i][j] -= f * aug[r][j]
			}
		}
		pivots = append(pivots, c)
		r++
	}
	return pivots
}

// linSolveDense 用带部分主元的高斯-约当消元求解稠密线性方程组 a x = b，返回解向量。主元绝对值
// 不超过 tolerance 视为奇异，返回 ok=false。
func linSolveDense(a [][]float64, b []float64, tolerance float64) ([]float64, bool) {
	n := len(a)
	aug := make([][]float64, n)
	for i := range aug {
		aug[i] = make([]float64, n+1)
		copy(aug[i], a[i])
		aug[i][n] = b[i]
	}
	for col := 0; col < n; col++ {
		pivotRow := col
		best := math.Abs(aug[col][col])
		for r := col + 1; r < n; r++ {
			if v := math.Abs(aug[r][col]); v > best {
				best = v
				pivotRow = r
			}
		}
		if best <= tolerance {
			return nil, false
		}
		aug[col], aug[pivotRow] = aug[pivotRow], aug[col]
		p := aug[col][col]
		for j := col; j <= n; j++ {
			aug[col][j] /= p
		}
		for r := 0; r < n; r++ {
			if r == col {
				continue
			}
			f := aug[r][col]
			if f == 0 {
				continue
			}
			for j := col; j <= n; j++ {
				aug[r][j] -= f * aug[col][j]
			}
		}
	}
	x := make([]float64, n)
	for i := range x {
		x[i] = aug[i][n]
	}
	return x, true
}

// ============================================================================
// 表达式求值共享：数学函数注入
// ============================================================================

// eqToFloat 把 govaluate 的求值结果归一化为 float64；非数值类型返回 ok=false。
func eqToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}

// eqUnaryMathFunc 把一个一元浮点函数包装为 govaluate 可用的表达式函数，参数个数或类型不符时返回
// 错误。
func eqUnaryMathFunc(fn func(float64) float64) govaluate.ExpressionFunction {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, errors.New("mathematical function expects exactly one argument")
		}
		v, ok := eqToFloat(args[0])
		if !ok {
			return nil, errors.New("mathematical function expects a numeric argument")
		}
		return fn(v), nil
	}
}

// eqMathFunctions 是注入到方程表达式的数学函数集合：govaluate 本身不提供数学函数，需在解析
// 表达式时随表达式一起注册。log 与 ln 均表示自然对数，log10 为常用对数。
var eqMathFunctions = map[string]govaluate.ExpressionFunction{
	"sin":   eqUnaryMathFunc(math.Sin),
	"cos":   eqUnaryMathFunc(math.Cos),
	"tan":   eqUnaryMathFunc(math.Tan),
	"asin":  eqUnaryMathFunc(math.Asin),
	"acos":  eqUnaryMathFunc(math.Acos),
	"atan":  eqUnaryMathFunc(math.Atan),
	"exp":   eqUnaryMathFunc(math.Exp),
	"log":   eqUnaryMathFunc(math.Log),
	"ln":    eqUnaryMathFunc(math.Log),
	"log10": eqUnaryMathFunc(math.Log10),
	"sqrt":  eqUnaryMathFunc(math.Sqrt),
	"abs":   eqUnaryMathFunc(math.Abs),
	"floor": eqUnaryMathFunc(math.Floor),
	"ceil":  eqUnaryMathFunc(math.Ceil),
	"round": eqUnaryMathFunc(math.Round),
	"pow": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, errors.New("pow expects exactly two arguments")
		}
		base, ok := eqToFloat(args[0])
		if !ok {
			return nil, errors.New("pow expects numeric arguments")
		}
		exponent, ok := eqToFloat(args[1])
		if !ok {
			return nil, errors.New("pow expects numeric arguments")
		}
		return math.Pow(base, exponent), nil
	},
	"min": func(args ...interface{}) (interface{}, error) {
		if len(args) == 0 {
			return nil, errors.New("min expects at least one argument")
		}
		best, ok := eqToFloat(args[0])
		if !ok {
			return nil, errors.New("min expects numeric arguments")
		}
		for _, arg := range args[1:] {
			v, ok := eqToFloat(arg)
			if !ok {
				return nil, errors.New("min expects numeric arguments")
			}
			if v < best {
				best = v
			}
		}
		return best, nil
	},
	"max": func(args ...interface{}) (interface{}, error) {
		if len(args) == 0 {
			return nil, errors.New("max expects at least one argument")
		}
		best, ok := eqToFloat(args[0])
		if !ok {
			return nil, errors.New("max expects numeric arguments")
		}
		for _, arg := range args[1:] {
			v, ok := eqToFloat(arg)
			if !ok {
				return nil, errors.New("max expects numeric arguments")
			}
			if v > best {
				best = v
			}
		}
		return best, nil
	},
}

// eqNewExpression 归一化表达式（把 ^ 视作幂运算替换为 **）并解析为可求值表达式，数学函数在解析
// 期注册到表达式内。
func eqNewExpression(expression string) (*govaluate.EvaluableExpression, error) {
	normalized := strings.ReplaceAll(expression, "^", "**")
	return govaluate.NewEvaluableExpressionWithFunctions(normalized, eqMathFunctions)
}

// ============================================================================
// 一般方程数值求根：FindRoots
// ============================================================================

// RootFindEntry 是求根输出中的单个根：根的近似值、该点的函数值与所用方法。
type RootFindEntry struct {
	// Root 根的近似值
	Root float64 `json:"root"`
	// FValue 根处的函数值（理论上接近 0）
	FValue float64 `json:"fValue"`
	// Method 求解方法：bisection 或 newton
	Method string `json:"method"`
}

// RootFindSolution 是求根成功时返回的载荷：roots 按根值升序排列，无根时为空数组。
type RootFindSolution struct {
	// Roots 全部找到的根
	Roots []RootFindEntry `json:"roots"`
}

// FindRoots 对一般方程 f(x)=0 做数值求根，表达式中的变量名固定为 x，^ 表示幂运算。可给定扫描区间
// interval（在区间内扫描变号子区间并用二分法细化）或给定初值 initial（用牛顿法迭代），至少提供
// 其中之一，二者都给时合并两条模式的结果。表达式的数学函数支持 sin cos tan exp log sqrt abs pow 等。
// 表达式为空、非法，interval / initial 均未提供，区间非正长度，容差非正或迭代上限小于 1 时返回
// 英文错误。
func FindRoots(expression string, interval *[2]float64, initial *float64, tolerance float64, maxIterations int) (RootFindSolution, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return RootFindSolution{}, errors.New("expression must be non-empty")
	}
	if tolerance <= 0 {
		return RootFindSolution{}, errors.Newf("tolerance %g must be positive", tolerance)
	}
	if maxIterations < 1 {
		return RootFindSolution{}, errors.Newf("maxIterations %d must be at least 1", maxIterations)
	}
	if interval == nil && initial == nil {
		return RootFindSolution{}, errors.New("at least one of interval or initial must be provided")
	}
	if interval != nil && interval[0] >= interval[1] {
		return RootFindSolution{}, errors.Newf("interval [%g, %g] must satisfy lower < upper", interval[0], interval[1])
	}
	expr, err := eqNewExpression(expression)
	if err != nil {
		return RootFindSolution{}, errors.Newf("invalid expression %q: %s", expression, err.Error())
	}
	f := func(x float64) float64 {
		raw, err := expr.Evaluate(map[string]interface{}{"x": x})
		if err != nil {
			return math.NaN()
		}
		value, ok := eqToFloat(raw)
		if !ok {
			return math.NaN()
		}
		return value
	}
	candidates := make([]RootFindEntry, 0)
	if interval != nil {
		candidates = append(candidates, rootIntervalCandidates(f, interval[0], interval[1], tolerance, maxIterations)...)
	}
	if initial != nil {
		if root, ok := rootNewtonRefine(f, *initial, tolerance, maxIterations); ok {
			candidates = append(candidates, RootFindEntry{Root: root, FValue: f(root), Method: "newton"})
		}
	}
	roots := rootDedupCandidates(candidates)
	sort.Slice(roots, func(i, j int) bool { return roots[i].Root < roots[j].Root })
	return RootFindSolution{Roots: roots}, nil
}

// rootIntervalCandidates 在 [lo, hi] 上均匀切分 rootFindSubintervals 个子区间扫描 f 的符号变化：
// 端点恰好为 0 也视为命中，端点异号的子区间用二分法细化到容差内，结果方法标记为 bisection。
func rootIntervalCandidates(f func(float64) float64, lo, hi, tolerance float64, maxIterations int) []RootFindEntry {
	out := make([]RootFindEntry, 0)
	step := (hi - lo) / float64(rootFindSubintervals)
	for i := 0; i < rootFindSubintervals; i++ {
		left := lo + step*float64(i)
		right := left + step
		if i == rootFindSubintervals-1 {
			right = hi
		}
		fLeft := f(left)
		fRight := f(right)
		if math.IsNaN(fLeft) || math.IsNaN(fRight) {
			continue
		}
		if fLeft == 0 {
			out = append(out, RootFindEntry{Root: left, FValue: 0, Method: "bisection"})
		}
		if fRight == 0 {
			out = append(out, RootFindEntry{Root: right, FValue: 0, Method: "bisection"})
		}
		if fLeft != 0 && fRight != 0 && math.Signbit(fLeft) != math.Signbit(fRight) {
			root := rootBisectionRefine(f, left, right, tolerance, maxIterations)
			out = append(out, RootFindEntry{Root: root, FValue: f(root), Method: "bisection"})
		}
	}
	return out
}

// rootBisectionRefine 在已知端点异号的区间 [lo, hi] 上用二分法细化到函数值或区间宽度小于容差。
func rootBisectionRefine(f func(float64) float64, lo, hi, tolerance float64, maxIterations int) float64 {
	fLeft := f(lo)
	for i := 0; i < maxIterations; i++ {
		mid := (lo + hi) / 2
		fMid := f(mid)
		if math.Abs(fMid) <= tolerance || (hi-lo)/2 <= tolerance {
			return mid
		}
		if math.Signbit(fMid) == math.Signbit(fLeft) {
			lo = mid
			fLeft = fMid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// rootNewtonRefine 从 x0 出发用牛顿法求根，导数用中心差分近似；收敛返回 (根, true)，否则返回当前
// 近似值与 false（用于与区间模式合并时丢弃无效结果）。
func rootNewtonRefine(f func(float64) float64, x0, tolerance float64, maxIterations int) (float64, bool) {
	x := x0
	for i := 0; i < maxIterations; i++ {
		fx := f(x)
		if math.IsNaN(fx) {
			return x, false
		}
		if math.Abs(fx) <= tolerance {
			return x, true
		}
		h := rootNewtonStep * math.Max(1, math.Abs(x))
		derivative := (f(x+h) - f(x-h)) / (2 * h)
		if derivative == 0 || math.IsNaN(derivative) || math.IsInf(derivative, 0) {
			return x, false
		}
		next := x - fx/derivative
		if math.Abs(next-x) <= tolerance {
			return next, true
		}
		x = next
	}
	return x, false
}

// rootDedupCandidates 按相对阈值合并相邻 / 重复的根，保留首次出现的方法标记与顺序。
func rootDedupCandidates(candidates []RootFindEntry) []RootFindEntry {
	out := make([]RootFindEntry, 0, len(candidates))
	for _, c := range candidates {
		duplicate := false
		for _, e := range out {
			if rootNearlyEqual(c.Root, e.Root) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, c)
		}
	}
	return out
}

// rootNearlyEqual 判断两个根是否在相对容差内视为同一个根。
func rootNearlyEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return diff <= rootFindMergeTolerance*scale
}

// ============================================================================
// 非线性方程组求解：SolveNonlinearSystem
// ============================================================================

// NonlinearSolution 是非线性方程组求解成功时返回的载荷：solution 为变量名到解的映射，residual
// 为各方程绝对值残差的最大值，converged 表示是否在迭代上限内收敛。
type NonlinearSolution struct {
	// Solution 变量名到近似解的映射
	Solution map[string]float64 `json:"solution"`
	// Residual 各方程 f 绝对值残差的最大值
	Residual float64 `json:"residual"`
	// Iterations 实际执行的牛顿迭代次数
	Iterations int `json:"iterations"`
	// Converged 是否在迭代上限内收敛
	Converged bool `json:"converged"`
}

// SolveNonlinearSystem 求解非线性方程组 F(x)=0（牛顿-拉夫逊法，雅可比矩阵用中心差分数值近似）。
// equations 每项为约定等于 0 的表达式，variables 给出变量名，initial 给出迭代初值；三者长度必须
// 一致。容差非正、迭代上限小于 1、任一入参为空、长度不一致、变量名为空或重名、表达式非法时返回
// 英文错误；雅可比奇异返回英文错误；迭代上限内未收敛不报错，仅将 converged 置为 false。
func SolveNonlinearSystem(equations []string, variables []string, initial []float64, tolerance float64, maxIterations int) (NonlinearSolution, error) {
	if tolerance <= 0 {
		return NonlinearSolution{}, errors.Newf("tolerance %g must be positive", tolerance)
	}
	if maxIterations < 1 {
		return NonlinearSolution{}, errors.Newf("maxIterations %d must be at least 1", maxIterations)
	}
	if len(equations) == 0 {
		return NonlinearSolution{}, errors.New("equations must be non-empty")
	}
	if len(variables) == 0 {
		return NonlinearSolution{}, errors.New("variables must be non-empty")
	}
	if len(initial) == 0 {
		return NonlinearSolution{}, errors.New("initial must be non-empty")
	}
	if len(equations) != len(variables) {
		return NonlinearSolution{}, errors.Newf("equations (%d) and variables (%d) must have the same length",
			len(equations), len(variables))
	}
	if len(variables) != len(initial) {
		return NonlinearSolution{}, errors.Newf("variables (%d) and initial (%d) must have the same length",
			len(variables), len(initial))
	}
	names := make([]string, len(variables))
	seen := make(map[string]struct{}, len(variables))
	for i, raw := range variables {
		name := strings.TrimSpace(raw)
		if name == "" {
			return NonlinearSolution{}, errors.New("variable names must be non-empty")
		}
		if _, dup := seen[name]; dup {
			return NonlinearSolution{}, errors.Newf("duplicate variable name %q", name)
		}
		seen[name] = struct{}{}
		names[i] = name
	}
	expressions := make([]*govaluate.EvaluableExpression, len(equations))
	for i, equation := range equations {
		expr, err := eqNewExpression(equation)
		if err != nil {
			return NonlinearSolution{}, errors.Newf("invalid expression %q: %s", equation, err.Error())
		}
		expressions[i] = expr
	}
	evaluate := func(x []float64) ([]float64, error) {
		params := make(map[string]interface{}, len(names))
		for i, name := range names {
			params[name] = x[i]
		}
		values := make([]float64, len(expressions))
		for i, expr := range expressions {
			raw, err := expr.Evaluate(params)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			value, ok := eqToFloat(raw)
			if !ok {
				return nil, errors.Newf("equation %d did not evaluate to a number", i+1)
			}
			values[i] = value
		}
		return values, nil
	}
	jacobian := func(x []float64) ([][]float64, error) {
		n := len(x)
		matrix := make([][]float64, n)
		for i := range matrix {
			matrix[i] = make([]float64, n)
		}
		for j := 0; j < n; j++ {
			h := rootNewtonStep * math.Max(1, math.Abs(x[j]))
			plus := make([]float64, n)
			copy(plus, x)
			plus[j] += h
			minus := make([]float64, n)
			copy(minus, x)
			minus[j] -= h
			fPlus, err := evaluate(plus)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			fMinus, err := evaluate(minus)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			for i := 0; i < n; i++ {
				matrix[i][j] = (fPlus[i] - fMinus[i]) / (2 * h)
			}
		}
		return matrix, nil
	}
	x := make([]float64, len(initial))
	copy(x, initial)
	converged := false
	iterations := 0
	for iterations = 0; iterations < maxIterations; iterations++ {
		values, err := evaluate(x)
		if err != nil {
			return NonlinearSolution{}, errors.WithStack(err)
		}
		if vecMaxAbs(values) <= tolerance {
			converged = true
			break
		}
		matrix, err := jacobian(x)
		if err != nil {
			return NonlinearSolution{}, errors.WithStack(err)
		}
		delta, ok := linSolveDense(matrix, vecNegate(values), tolerance)
		if !ok {
			return NonlinearSolution{}, errors.Newf("singular Jacobian at iteration %d", iterations)
		}
		for i := range x {
			x[i] += delta[i]
		}
		if vecMaxAbs(delta) <= tolerance {
			converged = true
			break
		}
	}
	finalValues, err := evaluate(x)
	if err != nil {
		return NonlinearSolution{}, errors.WithStack(err)
	}
	solution := make(map[string]float64, len(names))
	for i, name := range names {
		solution[name] = x[i]
	}
	return NonlinearSolution{
		Solution:   solution,
		Residual:   vecMaxAbs(finalValues),
		Iterations: iterations,
		Converged:  converged,
	}, nil
}

// vecNegate 返回向量各分量的相反数。
func vecNegate(v []float64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = -v[i]
	}
	return out
}

// vecMaxAbs 返回向量各分量绝对值的最大值，用作收敛判据中的残余度量。
func vecMaxAbs(v []float64) float64 {
	best := 0.0
	for _, x := range v {
		if a := math.Abs(x); a > best {
			best = a
		}
	}
	return best
}
