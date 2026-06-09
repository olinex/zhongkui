package ml

import (
	"math/big"
)

// BrichClusterFeature 表示BIRCH聚类算法的聚类特征
// 它使用三个统计量来高效表示一个数据聚类：
// - N: 数据点的数量
// - LineSum: 所有数据点的线性和（一阶矩）
// - SquareSum: 所有数据点的平方和（二阶矩）
// 这种表示方法允许在不存储所有原始数据点的情况下计算聚类的基本统计特性
type BrichClusterFeature struct {
	N         *big.Int // 数据点数量
	LineSum   *big.Int // 数据点线性和，用于计算均值
	SquareSum *big.Int // 数据点平方和，用于计算方差和标准差
}

// NewBrichClusterFeature 创建并返回一个新的聚类特征实例
// 返回的聚类特征的所有字段都初始化为0
// 这个函数用于创建一个空的聚类特征，可以后续添加数据点
func NewBrichClusterFeature() *BrichClusterFeature {
	return &BrichClusterFeature{
		N:         big.NewInt(0),
		LineSum:   big.NewInt(0),
		SquareSum: big.NewInt(0),
	}
}

// Reset 将聚类特征重置为零状态
// 这个方法将N、LineSum和SquareSum都设置为0
// 注意：这个方法重用现有的big.Int对象，只是重置它们的值，而不是创建新对象
func (c *BrichClusterFeature) Reset() {
	c.N.SetInt64(0)
	c.LineSum.SetInt64(0)
	c.SquareSum.SetInt64(0)
}

// Add 向聚类特征添加原始统计量
// 参数：
//   - n: 要添加的数据点数量
//   - ls: 要添加的数据点线性和
//   - ss: 要添加的数据点平方和
//
// 这个方法将给定的统计量累加到当前的聚类特征中
// 用于合并两个聚类特征的统计信息或添加批量的数据点统计
func (c *BrichClusterFeature) Add(n, ls, ss *big.Int) {
	c.N.Add(c.N, n)
	c.LineSum.Add(c.LineSum, ls)
	c.SquareSum.Add(c.SquareSum, ss)
}

// AddInt64 向聚类特征添加一个整数值的数据点
// 参数：
//   - value: 要添加的数据点值（必须为非负数）
//
// 这个方法会自动计算：
//   - 数据点数量增加1
//   - 线性和增加value
//   - 平方和增加value²
//
// 如果value为负数，方法会panic
// 这是添加单个数据点最方便的方法
func (c *BrichClusterFeature) AddInt64(value int64) {
	if value < 0 {
		panic("cluster feature cannot add negative value")
	}
	ls := big.NewInt(value)
	c.Add(
		big.NewInt(1),
		ls,
		new(big.Int).Mul(ls, ls),
	)
}

// Append 将另一个聚类特征的统计量追加到当前聚类特征
// 参数：
//   - cf: 要追加的聚类特征
//
// 这个方法将另一个聚类特征的N、LineSum和SquareSum累加到当前聚类特征
// 用于合并两个聚类，是BIRCH算法中聚类合并的核心操作
func (c *BrichClusterFeature) Append(cf *BrichClusterFeature) {
	c.Add(cf.N, cf.LineSum, cf.SquareSum)
}

// Centroid 计算并返回聚类的质心（均值）
// 返回值：聚类的质心，即所有数据点的平均值
// 质心计算公式：centroid = LineSum / N
// 如果聚类为空（N=0），返回0
// 质心是聚类的中心位置，用于表示聚类的典型位置
func (c *BrichClusterFeature) Centroid() *big.Float {
	if c.N.Int64() == 0 {
		return big.NewFloat(0)
	}
	n := new(big.Float).SetInt(c.N)
	ls := new(big.Float).SetInt(c.LineSum)
	// LS / N
	return n.Quo(
		ls,
		n,
	)
}

// Radius 计算并返回聚类的半径（标准差）
// 返回值：聚类的半径，即数据点相对于质心的平均距离
// 半径计算公式：radius = sqrt( (SS - 2*centroid*LS + N*centroid²) / N )
// 其中centroid是聚类的质心
// 如果聚类为空（N=0）或只有一个数据点（N=1），返回0
// 半径表示聚类的紧密程度，半径越小表示聚类越紧凑
func (c *BrichClusterFeature) Radius() *big.Float {
	// 空簇半径为0
	if c.N.Int64() == 0 {
		return big.NewFloat(0)
	}
	n := new(big.Float).SetInt(c.N)
	ls := new(big.Float).SetInt(c.LineSum)
	ss := new(big.Float).SetInt(c.SquareSum)
	// LS / N
	centroid := c.Centroid()
	// sqrt( (SS - 2*centroid*LS + N*centroid²) / N )
	// 2*centroid*LS
	t1 := new(big.Float).Mul(
		centroid,
		ls,
	)
	t1.Mul(big.NewFloat(2), t1)
	// N*centroid²
	t2 := new(big.Float).Mul(
		centroid,
		centroid,
	)
	t2.Mul(n, t2)
	// sqrt( (SS - t1 + t2) / N )
	t1.Sub(ss, t1)
	t1.Add(t1, t2)
	t1.Quo(t1, n)
	t1.Sqrt(t1)
	return t1
}

// Compare 计算当前聚类特征与另一个聚类特征的相似度
// 参数：
//   - other: 要比较的另一个聚类特征
//
// 返回值：相似度值，范围在[0, 1]之间
//
//	1表示完全相同（或两个空聚类）
//	0表示完全不相似（质心距离大于两半径之和）
//
// 相似度计算公式：
//  1. 计算两个聚类质心的欧氏距离
//  2. 计算最大可能距离 = 当前聚类半径 + 其他聚类半径
//  3. 如果最大可能距离为0（两个聚类都只有一个点或为空）：
//     - 如果质心距离也为0，相似度为1
//     - 否则相似度为0
//  4. 归一化距离 = 质心距离 / 最大可能距离（限制在[0,1]）
//  5. 相似度 = 1 - 归一化距离
//
// 这个方法用于BIRCH算法中判断两个聚类是否应该合并
func (c *BrichClusterFeature) Compare(other *BrichClusterFeature) *big.Float {
	// 计算质心
	curCentroid := c.Centroid()
	otherCentroid := other.Centroid()

	// 计算质心的欧氏距离
	distance := new(big.Float)
	distance.Sub(curCentroid, otherCentroid)
	distance.Abs(distance)

	// 簇间最大距离
	maxDistance := new(big.Float)
	curRadius := c.Radius()
	otherRadius := other.Radius()
	maxDistance.Add(curRadius, otherRadius)
	zero := big.NewFloat(0)

	// 避免除零
	if maxDistance.Cmp(zero) == 0 {
		if distance.Cmp(zero) == 0 {
			return big.NewFloat(1)
		} else {
			return zero
		}
	}

	// 归一化到 [0, 1]，距离越小相似度越高
	distance.Quo(distance, maxDistance)
	maxDistance.SetFloat64(1)
	if distance.Cmp(maxDistance) > 0 {
		distance = maxDistance
	}

	// 相似度 = 1 - 归一化距离
	similarity := new(big.Float).Sub(maxDistance, distance)
	return similarity
}
