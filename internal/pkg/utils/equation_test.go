// 本文件覆盖 internal/pkg/utils 中四个纯计算能力的算法与域级用例：SolvePolynomial（一元多项式求根）、
// SolveLinearSystem（线性方程组求解）、FindRoots（一般方程数值求根）与 SolveNonlinearSystem
// （非线性方程组求解）。全部用例直接调用 utils 导出函数，不经过任何适配器或运行期上下文；错误断言
// 针对 utils 抛出的无工具名前缀的错误。

package utils

import (
	// 标准库
	"math"
	"strings"
	"testing"
)

// equationTestNearlyEqual 判断两个浮点数是否在相对容差内相等，供各能力的数值断言复用。
func equationTestNearlyEqual(a, b float64) bool {
	diff := math.Abs(a - b)
	scale := math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
	return diff <= 1e-9*scale
}

// equationTestSliceNearlyEqual 判断两个浮点切片长度一致且逐元素在相对容差内相等，空切片视为相等。
func equationTestSliceNearlyEqual(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !equationTestNearlyEqual(got[i], want[i]) {
			return false
		}
	}
	return true
}

// ============================================================================
// SolvePolynomial
// ============================================================================

// TestSolvePolynomial_Normal 覆盖正常路径：整系数精确有理根、复数根、重根、多重有理根与小数系数的
// 数值求根路径，并校验根的排序确定性。
func TestSolvePolynomial_Normal(t *testing.T) {
	cases := []struct {
		name         string
		coefficients []float64
		degree       int
		roots        []PolynomialRoot
	}{
		{
			name:         "x^2-1 exact",
			coefficients: []float64{1, 0, -1},
			degree:       2,
			roots: []PolynomialRoot{
				{Real: -1, Imag: 0, Exact: "-1", Multiplicity: 1},
				{Real: 1, Imag: 0, Exact: "1", Multiplicity: 1},
			},
		},
		{
			name:         "2x^2-3x+1 rational roots",
			coefficients: []float64{2, -3, 1},
			degree:       2,
			roots: []PolynomialRoot{
				{Real: 0.5, Imag: 0, Exact: "1/2", Multiplicity: 1},
				{Real: 1, Imag: 0, Exact: "1", Multiplicity: 1},
			},
		},
		{
			name:         "x^2+1 complex roots",
			coefficients: []float64{1, 0, 1},
			degree:       2,
			roots: []PolynomialRoot{
				{Real: 0, Imag: -1, Exact: "", Multiplicity: 1},
				{Real: 0, Imag: 1, Exact: "", Multiplicity: 1},
			},
		},
		{
			name:         "x^2-2x+1 repeated root",
			coefficients: []float64{1, -2, 1},
			degree:       2,
			roots: []PolynomialRoot{
				{Real: 1, Imag: 0, Exact: "1", Multiplicity: 2},
			},
		},
		{
			name:         "cubic with three rational roots",
			coefficients: []float64{1, -6, 11, -6},
			degree:       3,
			roots: []PolynomialRoot{
				{Real: 1, Imag: 0, Exact: "1", Multiplicity: 1},
				{Real: 2, Imag: 0, Exact: "2", Multiplicity: 1},
				{Real: 3, Imag: 0, Exact: "3", Multiplicity: 1},
			},
		},
		{
			name:         "non-integer coefficients numeric path",
			coefficients: []float64{1, -2.5, 1},
			degree:       2,
			roots: []PolynomialRoot{
				{Real: 0.5, Imag: 0, Exact: "", Multiplicity: 1},
				{Real: 2, Imag: 0, Exact: "", Multiplicity: 1},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SolvePolynomial(tc.coefficients, DefaultPolySolveTolerance, DefaultPolySolveMaxIterations)
			if err != nil {
				t.Fatalf("SolvePolynomial(%v) error = %v", tc.coefficients, err)
			}
			if got.Degree != tc.degree {
				t.Errorf("degree = %d, want %d", got.Degree, tc.degree)
			}
			if len(got.Roots) != len(tc.roots) {
				t.Fatalf("roots = %+v, want %d entries", got.Roots, len(tc.roots))
			}
			for _, want := range tc.roots {
				matched := false
				for _, root := range got.Roots {
					if root.Exact == want.Exact && root.Multiplicity == want.Multiplicity &&
						equationTestNearlyEqual(root.Real, want.Real) && equationTestNearlyEqual(root.Imag, want.Imag) {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("root %+v not found in %+v", want, got.Roots)
				}
			}
			for i := 1; i < len(got.Roots); i++ {
				if got.Roots[i].Real < got.Roots[i-1].Real-1e-9 {
					t.Errorf("roots not sorted by real part: %+v", got.Roots)
				}
			}
		})
	}
}

// TestSolvePolynomial_Errors 覆盖错误路径：空数组、全零系数、常数多项式、非正的容差与过小的迭代上限。
func TestSolvePolynomial_Errors(t *testing.T) {
	cases := []struct {
		name         string
		coefficients []float64
		tolerance    float64
		maxIter      int
		wantHint     string
	}{
		{"empty", []float64{}, DefaultPolySolveTolerance, DefaultPolySolveMaxIterations, "coefficients must be non-empty"},
		{"missing", nil, DefaultPolySolveTolerance, DefaultPolySolveMaxIterations, "coefficients must be non-empty"},
		{"all zero", []float64{0, 0, 0}, DefaultPolySolveTolerance, DefaultPolySolveMaxIterations, "must not be all zero"},
		{"constant polynomial", []float64{5}, DefaultPolySolveTolerance, DefaultPolySolveMaxIterations, "polynomial degree"},
		{"tolerance not positive", []float64{1, 0, -1}, -1, DefaultPolySolveMaxIterations, "tolerance"},
		{"max iterations too small", []float64{1, 0, -1}, DefaultPolySolveTolerance, 0, "maxIterations"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SolvePolynomial(tc.coefficients, tc.tolerance, tc.maxIter)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// ============================================================================
// SolveLinearSystem
// ============================================================================

// TestSolveLinearSystem_Normal 覆盖正常路径：唯一解、无穷多解（含零空间基向量）与无解。
func TestSolveLinearSystem_Normal(t *testing.T) {
	cases := []struct {
		name          string
		a             [][]float64
		b             []float64
		wantType      string
		wantRank      int
		wantSolution  []float64
		wantParticul  []float64
		wantNullspace [][]float64
	}{
		{
			name:          "spec example unique",
			a:             [][]float64{{2, 1}, {1, -1}},
			b:             []float64{1, 2},
			wantType:      "unique",
			wantRank:      2,
			wantSolution:  []float64{1, -1},
			wantParticul:  []float64{},
			wantNullspace: [][]float64{},
		},
		{
			name:          "identity unique",
			a:             [][]float64{{1, 0}, {0, 1}},
			b:             []float64{3, 4},
			wantType:      "unique",
			wantRank:      2,
			wantSolution:  []float64{3, 4},
			wantParticul:  []float64{},
			wantNullspace: [][]float64{},
		},
		{
			name:          "infinite solutions",
			a:             [][]float64{{1, 1}, {2, 2}},
			b:             []float64{1, 2},
			wantType:      "infinite",
			wantRank:      1,
			wantSolution:  []float64{},
			wantParticul:  []float64{1, 0},
			wantNullspace: [][]float64{{-1, 1}},
		},
		{
			name:          "no solution",
			a:             [][]float64{{1, 1}, {2, 2}},
			b:             []float64{1, 3},
			wantType:      "none",
			wantRank:      1,
			wantSolution:  []float64{},
			wantParticul:  []float64{},
			wantNullspace: [][]float64{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SolveLinearSystem(tc.a, tc.b, DefaultLinearSolveTolerance)
			if err != nil {
				t.Fatalf("SolveLinearSystem(%v, %v) error = %v", tc.a, tc.b, err)
			}
			if got.SolutionType != tc.wantType {
				t.Errorf("solutionType = %q, want %q", got.SolutionType, tc.wantType)
			}
			if got.Rank != tc.wantRank {
				t.Errorf("rank = %d, want %d", got.Rank, tc.wantRank)
			}
			if !equationTestSliceNearlyEqual(got.Solution, tc.wantSolution) {
				t.Errorf("solution = %+v, want %+v", got.Solution, tc.wantSolution)
			}
			if !equationTestSliceNearlyEqual(got.Particular, tc.wantParticul) {
				t.Errorf("particular = %+v, want %+v", got.Particular, tc.wantParticul)
			}
			if len(got.Nullspace) != len(tc.wantNullspace) {
				t.Fatalf("nullspace = %+v, want %d vectors", got.Nullspace, len(tc.wantNullspace))
			}
			for i := range tc.wantNullspace {
				if !equationTestSliceNearlyEqual(got.Nullspace[i], tc.wantNullspace[i]) {
					t.Errorf("nullspace[%d] = %+v, want %+v", i, got.Nullspace[i], tc.wantNullspace[i])
				}
			}
		})
	}
}

// TestSolveLinearSystem_Errors 覆盖错误路径：空矩阵、非方阵、行列长度不一致、b 长度不符与非正容差。
func TestSolveLinearSystem_Errors(t *testing.T) {
	cases := []struct {
		name     string
		a        [][]float64
		b        []float64
		tol      float64
		wantHint string
	}{
		{"empty a", [][]float64{}, []float64{}, DefaultLinearSolveTolerance, "must be non-empty"},
		{"non-square", [][]float64{{1, 2, 3}, {4, 5, 6}}, []float64{1, 2}, DefaultLinearSolveTolerance, "must be square"},
		{"row length mismatch", [][]float64{{1, 2}, {3}}, []float64{1, 2}, DefaultLinearSolveTolerance, "must be square"},
		{"b length mismatch", [][]float64{{1, 2}, {3, 4}}, []float64{1}, DefaultLinearSolveTolerance, "must equal the number of rows"},
		{"tolerance not positive", [][]float64{{1, 2}, {3, 4}}, []float64{1, 2}, 0, "must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SolveLinearSystem(tc.a, tc.b, tc.tol)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// ============================================================================
// FindRoots
// ============================================================================

// TestFindRoots_Normal 覆盖正常路径：^ 归一化为幂、多根区间扫描、牛顿法初值与双模式合并，并校验根
// 升序、函数值接近 0 与方法标记。
func TestFindRoots_Normal(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		interval   *[2]float64
		initial    *float64
		wantRoots  []float64
		wantMethod string
	}{
		{
			name:       "x^2-4 via interval",
			expression: "x^2-4",
			interval:   &[2]float64{-5, 5},
			wantRoots:  []float64{-2, 2},
			wantMethod: "bisection",
		},
		{
			name:       "x^3-x via interval",
			expression: "x^3-x",
			interval:   &[2]float64{-5, 5},
			wantRoots:  []float64{-1, 0, 1},
			wantMethod: "bisection",
		},
		{
			name:       "x^2-2 via newton",
			expression: "x^2-2",
			initial:    floatPtr(1),
			wantRoots:  []float64{math.Sqrt(2)},
			wantMethod: "newton",
		},
		{
			name:       "x^2-2 via both modes",
			expression: "x^2-2",
			interval:   &[2]float64{-5, 5},
			initial:    floatPtr(1),
			wantRoots:  []float64{-math.Sqrt(2), math.Sqrt(2)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FindRoots(tc.expression, tc.interval, tc.initial, DefaultRootFindTolerance, DefaultRootFindMaxIterations)
			if err != nil {
				t.Fatalf("FindRoots(%q) error = %v", tc.expression, err)
			}
			if len(got.Roots) != len(tc.wantRoots) {
				t.Fatalf("roots = %+v, want %d entries", got.Roots, len(tc.wantRoots))
			}
			for i, want := range tc.wantRoots {
				root := got.Roots[i]
				if !equationTestNearlyEqual(root.Root, want) {
					t.Errorf("roots[%d].root = %v, want %v", i, root.Root, want)
				}
				if math.Abs(root.FValue) > 1e-6 {
					t.Errorf("roots[%d].fValue = %v, want near 0", i, root.FValue)
				}
				if tc.wantMethod != "" && root.Method != tc.wantMethod {
					t.Errorf("roots[%d].method = %q, want %q", i, root.Method, tc.wantMethod)
				}
			}
			for i := 1; i < len(got.Roots); i++ {
				if got.Roots[i].Root < got.Roots[i-1].Root {
					t.Errorf("roots not sorted ascending: %+v", got.Roots)
				}
			}
		})
	}
}

// TestFindRoots_Errors 覆盖错误路径：既无区间也无初值、空/非法表达式、逆序或零长度区间与非正容差。
func TestFindRoots_Errors(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		interval   *[2]float64
		initial    *float64
		tol        float64
		wantHint   string
	}{
		{"no interval or initial", "x^2-4", nil, nil, DefaultRootFindTolerance, "at least one of interval or initial"},
		{"empty expression", "", &[2]float64{-1, 1}, nil, DefaultRootFindTolerance, "expression must be non-empty"},
		{"invalid expression", "x^2 -", &[2]float64{-1, 1}, nil, DefaultRootFindTolerance, "invalid expression"},
		{"reversed interval", "x^2-4", &[2]float64{5, -5}, nil, DefaultRootFindTolerance, "must satisfy lower < upper"},
		{"zero length interval", "x^2-4", &[2]float64{1, 1}, nil, DefaultRootFindTolerance, "must satisfy lower < upper"},
		{"tolerance not positive", "x^2-4", &[2]float64{-5, 5}, nil, -1, "must be positive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FindRoots(tc.expression, tc.interval, tc.initial, tc.tol, DefaultRootFindMaxIterations)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}

// floatPtr 返回局部浮点量的指针，供 FindRoots 的 initial 可选入参构造使用。
func floatPtr(v float64) *float64 {
	return &v
}

// ============================================================================
// SolveNonlinearSystem
// ============================================================================

// TestSolveNonlinearSystem_Normal 覆盖正常路径：圆与直线的交点方程组，校验解、残余与收敛标记。
func TestSolveNonlinearSystem_Normal(t *testing.T) {
	got, err := SolveNonlinearSystem(
		[]string{"x^2+y^2-1", "x-y"},
		[]string{"x", "y"},
		[]float64{1, 1},
		DefaultNonlinearSolveTolerance,
		DefaultNonlinearSolveMaxIterations,
	)
	if err != nil {
		t.Fatalf("SolveNonlinearSystem error = %v", err)
	}
	if !got.Converged {
		t.Errorf("converged = false, want true (residual=%v)", got.Residual)
	}
	if got.Residual > 1e-6 {
		t.Errorf("residual = %v, want near 0", got.Residual)
	}
	if got.Iterations < 1 {
		t.Errorf("iterations = %d, want at least 1", got.Iterations)
	}
	want := math.Sqrt(0.5)
	if !equationTestNearlyEqual(got.Solution["x"], want) || !equationTestNearlyEqual(got.Solution["y"], want) {
		t.Errorf("solution = %+v, want x=y=%v", got.Solution, want)
	}
}

// TestSolveNonlinearSystem_Errors 覆盖错误路径：长度不一致、空方程组与变量重名。
func TestSolveNonlinearSystem_Errors(t *testing.T) {
	cases := []struct {
		name      string
		equations []string
		variables []string
		initial   []float64
		wantHint  string
	}{
		{"length mismatch", []string{"x^2-1", "y-1"}, []string{"x"}, []float64{1}, "must have the same length"},
		{"empty equations", []string{}, []string{}, []float64{}, "equations must be non-empty"},
		{"duplicate variables", []string{"x+y", "x-y"}, []string{"x", "x"}, []float64{1, 1}, "duplicate variable name"},
		{"initial length mismatch", []string{"x+y"}, []string{"x"}, []float64{}, "initial must be non-empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SolveNonlinearSystem(tc.equations, tc.variables, tc.initial,
				DefaultNonlinearSolveTolerance, DefaultNonlinearSolveMaxIterations)
			if err == nil {
				t.Fatal("error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), tc.wantHint) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantHint)
			}
		})
	}
}
