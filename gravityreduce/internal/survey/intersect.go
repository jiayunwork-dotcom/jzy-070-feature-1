package survey

import "math"

// xy 是平面点/二维向量；坐标单位均为米。
type xy struct{ x, y float64 }

func pointOf(p orderedPoint) xy { return xy{p.in.EastingM, p.in.NorthingM} }

func sub(a, b xy) xy { return xy{a.x - b.x, a.y - b.y} }

// cross 二维叉积 z 分量（平行四边形有向面积）。
func cross(a, b xy) float64 { return a.x*b.y - a.y*b.x }

// seg 是一条测线上排序相邻、且平面长度非零的测点段。
type seg struct {
	from, to orderedPoint
	a, b, d  xy
	length   float64
}

// onSegmentEps 为交点落在段端上时允许的数值容差（按段长的相对比例）。
const onSegmentEps = 1e-9

// lineSegments 取排序后相邻测点构成的非零段；零距离段在梯度诊断中已作为
// 数据异常报出，求交时跳过（没有几何走向，无法承担交点与插值）。
func lineSegments(pts []orderedPoint) []seg {
	out := make([]seg, 0, len(pts)-1)
	for i := 0; i+1 < len(pts); i++ {
		a, b := pointOf(pts[i]), pointOf(pts[i+1])
		d := sub(b, a)
		l := math.Hypot(d.x, d.y)
		if l <= zeroDistanceEpsilonM {
			continue
		}
		out = append(out, seg{from: pts[i], to: pts[i+1], a: a, b: b, d: d, length: l})
	}
	return out
}

// hit 是一对段的求交命中：交点位置与两侧段参数。
type hit struct {
	p      xy
	tA, tB float64
	ai, bi int // 命中的段在各自线 nonZero 段列表中的下标
}

// segmentIntersect 求两条非零线段的交点。
// 返回 (hit, true) 当且仅当交点同时落在两段的实测范围内（含端点）。
// 平行/共线（denom≈0）一律返回 false：共线交点不唯一，绝不硬凑一个点。
func segmentIntersect(s1, s2 seg, i, j int) (hit, bool) {
	denom := cross(s1.d, s2.d)
	scale := math.Max(1.0, s1.length) * math.Max(1.0, s2.length)
	if math.Abs(denom) <= onSegmentEps*scale {
		return hit{}, false // 平行或共线：没有唯一交点
	}
	diff := sub(s2.a, s1.a)
	t := cross(diff, s2.d) / denom
	u := cross(diff, s1.d) / denom
	eps := onSegmentEps
	if t < -eps || t > 1+eps || u < -eps || u > 1+eps {
		return hit{}, false // 交点在至少一段的范围之外
	}
	t = clamp01(t)
	u = clamp01(u)
	return hit{p: xy{s1.a.x + t*s1.d.x, s1.a.y + t*s1.d.y}, tA: t, tB: u, ai: i, bi: j}, true
}

func clamp01(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t
}

func samePoint(a, b xy) bool {
	return math.Abs(a.x-b.x) <= zeroDistanceEpsilonM && math.Abs(a.y-b.y) <= zeroDistanceEpsilonM
}

// findIntersections 求两条折线（非零段序列）所有落在实测段上的唯一交点。
func findIntersections(segsA, segsB []seg) []hit {
	var hits []hit
	for i := range segsA {
		for j := range segsB {
			if h, ok := segmentIntersect(segsA[i], segsB[j], i, j); ok {
				dup := false
				for _, k := range hits {
					if samePoint(k.p, h.p) {
						dup = true
						break
					}
				}
				if !dup {
					hits = append(hits, h)
				}
			}
		}
	}
	return hits
}

// interpolateAt 用相邻测点的布格异常在交点处做线性插值。
// 异常只在这一步被使用：以段两端真实归算结果按段上线性参数 t 加权，
// 绝不使用固定值或近似占位。
func interpolateAt(lineID string, s seg, t float64) InterpolatedSide {
	vA := s.from.res.BouguerAnomalyMGal
	vB := s.to.res.BouguerAnomalyMGal
	return InterpolatedSide{
		LineID:        lineID,
		SegmentFromID: s.from.in.ID,
		SegmentToID:   s.to.in.ID,
		ParameterT:    t,
		AnomalyMGal:   vA + t*(vB-vA),
	}
}
