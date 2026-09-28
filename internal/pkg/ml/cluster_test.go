package ml

import (
	"math/big"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestClusterFeature 验证 BIRCH 聚类特征对象的累加、重置、合并与相似度计算。
func TestClusterFeature(t *testing.T) {
	Convey("Call NewClusterFeature", t, func() {
		Convey("1. Should return empty ClusterFeature", func(c C) {
			cf := NewBrichClusterFeature()
			c.So(cf.N.Int64(), ShouldEqual, int64(0))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(0))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(0))
		})
		Convey("2. Should have non-nil fields", func(c C) {
			cf := NewBrichClusterFeature()
			c.So(cf.N, ShouldNotBeNil)
			c.So(cf.LineSum, ShouldNotBeNil)
			c.So(cf.SquareSum, ShouldNotBeNil)
		})
	})

	Convey("Call Reset", t, func() {
		Convey("1. Should reset all values to zero", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(10)
			cf.AddInt64(20)
			cf.Reset()
			c.So(cf.N.Int64(), ShouldEqual, int64(0))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(0))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(0))
		})
		Convey("2. Should reset after multiple operations", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(1)
			cf.AddInt64(2)
			cf.AddInt64(3)
			cf.Reset()
			cf.AddInt64(5)
			c.So(cf.N.Int64(), ShouldEqual, int64(1))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(5))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(25))
		})
	})

	Convey("Call Add", t, func() {
		Convey("1. Should add big.Int values", func(c C) {
			cf := NewBrichClusterFeature()
			cf.Add(big.NewInt(2), big.NewInt(10), big.NewInt(100))
			c.So(cf.N.Int64(), ShouldEqual, int64(2))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(10))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(100))
		})
		Convey("2. Should accumulate values correctly", func(c C) {
			cf := NewBrichClusterFeature()
			cf.Add(big.NewInt(1), big.NewInt(5), big.NewInt(25))
			cf.Add(big.NewInt(1), big.NewInt(10), big.NewInt(100))
			c.So(cf.N.Int64(), ShouldEqual, int64(2))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(15))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(125))
		})
		Convey("3. Should handle large values", func(c C) {
			cf := NewBrichClusterFeature()
			cf.Add(big.NewInt(1000000), big.NewInt(1000000000000), big.NewInt(1000000000000000000))
			c.So(cf.N.Int64(), ShouldEqual, int64(1000000))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(1000000000000))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(1000000000000000000))
		})
	})

	Convey("Call AddInt64", t, func() {
		Convey("1. Should add int64 value to ClusterFeature", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(10)
			c.So(cf.N.Int64(), ShouldEqual, int64(1))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(10))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(100))
			cf.AddInt64(20)
			c.So(cf.N.Int64(), ShouldEqual, int64(2))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(30))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(500))
		})
		Convey("2. Should panic when add negative int64 value", func(c C) {
			cf := NewBrichClusterFeature()
			c.So(
				func() {
					cf.AddInt64(-5)
				},
				ShouldPanicWith,
				"cluster feature cannot add negative value",
			)
		})
		Convey("3. Should handle zero value", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(0)
			c.So(cf.N.Int64(), ShouldEqual, int64(1))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(0))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(0))
		})
		Convey("4. Should add multiple values", func(c C) {
			cf := NewBrichClusterFeature()
			for i := int64(1); i <= 100; i++ {
				cf.AddInt64(i)
			}
			c.So(cf.N.Int64(), ShouldEqual, int64(100))
			c.So(cf.LineSum.Int64(), ShouldEqual, int64(5050))
			c.So(cf.SquareSum.Int64(), ShouldEqual, int64(338350))
		})
	})

	Convey("Call Append", t, func() {
		Convey("1. Should append ClusterFeature", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf1.Append(cf2)
			c.So(cf1.N.Int64(), ShouldEqual, int64(0))
			c.So(cf1.LineSum.Int64(), ShouldEqual, int64(0))
			c.So(cf1.SquareSum.Int64(), ShouldEqual, int64(0))
			cf2.AddInt64(10)
			cf1.Append(cf2)
			c.So(cf1.N.Int64(), ShouldEqual, int64(1))
			c.So(cf1.LineSum.Int64(), ShouldEqual, int64(10))
			c.So(cf1.SquareSum.Int64(), ShouldEqual, int64(100))
			cf1.AddInt64(20)
			cf1.Append(cf2)
			c.So(cf1.N.Int64(), ShouldEqual, int64(3))
			c.So(cf1.LineSum.Int64(), ShouldEqual, int64(40))
			c.So(cf1.SquareSum.Int64(), ShouldEqual, int64(600))
		})
		Convey("2. Should handle appending empty feature", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf1.AddInt64(5)
			cf2 := NewBrichClusterFeature()
			cf1.Append(cf2)
			c.So(cf1.N.Int64(), ShouldEqual, int64(1))
			c.So(cf1.LineSum.Int64(), ShouldEqual, int64(5))
			c.So(cf1.SquareSum.Int64(), ShouldEqual, int64(25))
		})
		Convey("3. Should handle appending multiple features", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf3 := NewBrichClusterFeature()
			cf2.AddInt64(1)
			cf3.AddInt64(2)
			cf1.Append(cf2)
			cf1.Append(cf3)
			c.So(cf1.N.Int64(), ShouldEqual, int64(2))
			c.So(cf1.LineSum.Int64(), ShouldEqual, int64(3))
			c.So(cf1.SquareSum.Int64(), ShouldEqual, int64(5))
		})
	})

	Convey("Call Centroid", t, func() {
		Convey("1. Empty cluster should return zero", func(c C) {
			cf := NewBrichClusterFeature()
			centroid := cf.Centroid()
			c.So(centroid.Cmp(big.NewFloat(0)), ShouldEqual, 0)
		})
		Convey("2. Single point should return the point value", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(10)
			centroid := cf.Centroid()
			c.So(centroid.Cmp(big.NewFloat(10)), ShouldEqual, 0)
		})
		Convey("3. Multiple points should return mean", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(10)
			cf.AddInt64(20)
			centroid := cf.Centroid()
			c.So(centroid.Cmp(big.NewFloat(15)), ShouldEqual, 0)
		})
		Convey("4. Points 1,2,3 should return mean of 2", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(1)
			cf.AddInt64(2)
			cf.AddInt64(3)
			centroid := cf.Centroid()
			c.So(centroid.Cmp(big.NewFloat(2)), ShouldEqual, 0)
		})
		Convey("5. Handle large numbers", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(1000000)
			cf.AddInt64(2000000)
			centroid := cf.Centroid()
			c.So(centroid.Cmp(big.NewFloat(1500000)), ShouldEqual, 0)
		})
	})

	Convey("Call Radius", t, func() {
		Convey("1. Empty cluster should return zero", func(c C) {
			cf := NewBrichClusterFeature()
			radius := cf.Radius()
			c.So(radius.Cmp(big.NewFloat(0.0)), ShouldEqual, 0)
		})
		Convey("2. Single point", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(10)
			radius := cf.Radius()
			c.So(radius.Cmp(big.NewFloat(0.0)), ShouldEqual, 0)
		})
		Convey("3. test 1,2,3", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(1)
			cf.AddInt64(2)
			cf.AddInt64(3)
			radius := cf.Radius()
			c.So(new(big.Float).Abs(new(big.Float).Sub(radius, big.NewFloat(0.816496580927726))).Cmp(big.NewFloat(0.000000000001)), ShouldEqual, -1)
		})
		Convey("4. Same values should have zero radius", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(5)
			cf.AddInt64(5)
			cf.AddInt64(5)
			radius := cf.Radius()
			c.So(radius.Cmp(big.NewFloat(0.0)), ShouldEqual, 0)
		})
		Convey("5. Wide spread values", func(c C) {
			cf := NewBrichClusterFeature()
			cf.AddInt64(0)
			cf.AddInt64(100)
			radius := cf.Radius()
			c.So(radius.Cmp(big.NewFloat(50.0)), ShouldEqual, 0)
		})
	})

	Convey("Call Compare", t, func() {
		Convey("1. Test empty cf compare", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			similarity := cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(1.0)), ShouldEqual, 0)
			similarity = cf2.Compare(cf1)
			c.So(similarity.Cmp(big.NewFloat(1.0)), ShouldEqual, 0)
		})
		Convey("2. Test compare empty and not empty", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf2.AddInt64(10)
			similarity := cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(0.0)), ShouldEqual, 0)
			similarity = cf2.Compare(cf1)
			c.So(similarity.Cmp(big.NewFloat(0.0)), ShouldEqual, 0)
		})
		Convey("3. Test compare not empty and not empty", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf1.AddInt64(10)
			cf2.AddInt64(20)
			similarity := cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(0)), ShouldEqual, 0)
			similarity = cf2.Compare(cf1)
			c.So(similarity.Cmp(big.NewFloat(0)), ShouldEqual, 0)
			cf1.AddInt64(20)
			cf2.AddInt64(10)
			similarity = cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(1)), ShouldEqual, 0)
			similarity = cf2.Compare(cf1)
			c.So(similarity.Cmp(big.NewFloat(1)), ShouldEqual, 0)
			cf1.AddInt64(1)
			cf2.AddInt64(2)
			similarity = cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(0.97)), ShouldEqual, 1)
			similarity = cf2.Compare(cf1)
			c.So(similarity.Cmp(big.NewFloat(0.97)), ShouldEqual, 1)
		})
		Convey("4. Test identical clusters", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf1.AddInt64(5)
			cf1.AddInt64(10)
			cf2.AddInt64(5)
			cf2.AddInt64(10)
			similarity := cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(1.0)), ShouldEqual, 0)
		})
		Convey("5. Test similarity is bounded between 0 and 1", func(c C) {
			cf1 := NewBrichClusterFeature()
			cf2 := NewBrichClusterFeature()
			cf1.AddInt64(1)
			cf2.AddInt64(1000)
			similarity := cf1.Compare(cf2)
			c.So(similarity.Cmp(big.NewFloat(0.0)), ShouldBeGreaterThanOrEqualTo, 0)
			c.So(similarity.Cmp(big.NewFloat(1.0)), ShouldBeLessThanOrEqualTo, 0)
		})
	})
}
