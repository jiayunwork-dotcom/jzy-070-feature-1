// 平面几何：把每条测线视为按里程连接各测点的折线，求两条折线的平面交点。
//
// 勘探上主测线与联络测线极少恰好在已测点上相交，交点通常落在各自某一相邻测点段的
// 中间。这里逐段（线段 × 线段）求交，并在交点处对相邻测点的布格异常做线性插值。
//
// 几何上不相交、平行、共线（无唯一交点）、交点落在测量段之外这几类情形，
// 一律不产生交点差，由调用方如实标注为“无有效交点”，绝不硬凑。
package survey

import "math"

// geomEps 是相对容限：叉积比较按“方向长度乘积 × geomEps”判定平行/共线，
// 参数落在段外按 pad 放宽，以吸收正常浮点尾差。
const geomEps = 1e-9

type xyPoint struct {
	e, n float64
}

// segHit 是一次线段对求交命中的唯一交点。
type segHit struct {
	e, n   float64
	ai, bi int     // 命中的段在各自折线中的序号（点 i → i+1）
	ta, tb float64 // 交点在两段上的位置参数 [0,1]
}

// segmentCross 求线段 A(a0→a1) 与 B(b0→b1) 的交点。
//
// 返回：
//   - hit!=nil：两线段有唯一交点且落在两段（含端点）测量范围内；
//   - collinear==true：两线段共线（重合或共线相离），没有唯一交点；
//   - parallel==true：平行但不共线；
//   - 三者皆否：支撑线相交但交点在段外（outside）。
//
// ta、tb 为交点在两段方向上的位置参数（outside 时也给出，供调试/扩展）。
func segmentCross(a0, a1, b0, b1 xyPoint) (hit *segHit, collinear, parallel bool, ta, tb float64) {
	ra := xyPoint{a1.e - a0.e, a1.n - a0.n}
	rb := xyPoint{b1.e - b0.e, b1.n - b0.n}
	qp := xyPoint{b0.e - a0.e, b0.n - a0.n}

	rCrossS := cross(ra, rb)
	parScale := math.Max(1.0, math.Abs(ra.e)*math.Abs(rb.n)+math.Abs(ra.n)*math.Abs(rb.e))

	if math.Abs(rCrossS) <= geomEps*parScale {
		// 平行：再判共线（C−A 与 r 平行）。
		colScale := math.Max(1.0, math.Abs(qp.e)*math.Abs(ra.n)+math.Abs(qp.n)*math.Abs(ra.e))
		if math.Abs(cross(qp, ra)) <= geomEps*colScale {
			return nil, true, false, 0, 0
		}
		return nil, false, true, 0, 0
	}

	t := cross(qp, rb) / rCrossS
	u := cross(qp, ra) / rCrossS
	pad := geomEps * math.Max(1.0, math.Max(math.Abs(ra.e), math.Abs(ra.n)))
	if t < -pad || t > 1+pad || u < -pad || u > 1+pad {
		return nil, false, false, t, u // 支撑线相交，但交点落在测量段之外
	}

	t = clamp01(t)
	u = clamp01(u)
	return &segHit{
		e:  a0.e + t*ra.e,
		n:  a0.n + t*ra.n,
		ta: t,
		tb: u,
	}, false, false, t, u
}

func cross(a, b xyPoint) float64 { return a.e*b.n - a.n*b.e }

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func clampRange(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// polylineCrossings 枚举两条折线（均已按里程排序）所有线段对的有效交点，
// 并对几何上同一点的多个命中（相邻段在公共端点处会各报一次）去重。
// 同时回传是否出现过“平行不共线”或“共线”线段对，供线对级原因分类。
func polylineCrossings(a, b []orderedPoint) (hits []segHit, sawParallel, sawCollinear bool) {
	coordScale := coordMagnitudeScale(a, b)
	dedupTol := geomEps * coordScale

	for ai := 0; ai+1 < len(a); ai++ {
		a0 := xyOf(a[ai])
		a1 := xyOf(a[ai+1])
		if distXY(a0, a1) < zeroDistanceEpsM {
			continue // 零距离退化段不参与求交
		}
		for bi := 0; bi+1 < len(b); bi++ {
			b0 := xyOf(b[bi])
			b1 := xyOf(b[bi+1])
			if distXY(b0, b1) < zeroDistanceEpsM {
				continue
			}
			hit, col, par, _, _ := segmentCross(a0, a1, b0, b1)
			if col {
				// 共线线段只在“恰好端点相接”时给出唯一交点；有正长度重叠则无唯一交点。
				sawCollinear = true
				if p, ok := collinearTouchPoint(a0, a1, b0, b1); ok {
					p.bi = bi
					hits = appendDedup(hits, p, ai, bi, dedupTol)
				}
				continue
			}
			if par {
				sawParallel = true
				continue
			}
			if hit == nil {
				continue // 支撑线相交但交点在此线段对范围之外
			}
			hit.ai, hit.bi = ai, bi
			hits = appendDedup(hits, *hit, ai, bi, dedupTol)
		}
	}
	return hits, sawParallel, sawCollinear
}

// collinearTouchPoint 处理已知共线的两段：投影到 A 的单位方向上，
// 若两段公共部分恰为一个点（端点相接），返回该点及它在两段上的位置参数；
// 正长度重叠或相离则无唯一交点。
func collinearTouchPoint(a0, a1, b0, b1 xyPoint) (segHit, bool) {
	ra := xyPoint{a1.e - a0.e, a1.n - a0.n}
	la := math.Hypot(ra.e, ra.n)
	rb := xyPoint{b1.e - b0.e, b1.n - b0.n}
	lb := math.Hypot(rb.e, rb.n)
	if la < zeroDistanceEpsM || lb < zeroDistanceEpsM {
		return segHit{}, false
	}
	ux, uy := ra.e/la, ra.n/la
	proj := func(p xyPoint) float64 { return (p.e-a0.e)*ux + (p.n-a0.n)*uy }
	bc0, bc1 := proj(b0), proj(b1)
	lo := math.Max(0.0, math.Min(bc0, bc1))
	hi := math.Min(la, math.Max(bc0, bc1))
	gapTol := geomEps * math.Max(1.0, la)
	// 公共部分长度 = hi−lo：
	//	≈ 0（在容限内相触）      → 端点相接，给出唯一交点；
	//	> 容限（正长度重叠）     → 交点不唯一；
	//	< −容限（区间之间有间隙）→ 共线相离，无交点。
	if hi-lo >= -gapTol && hi-lo <= gapTol {
		// 公共退化为一点。注意 s 是沿 A 方向的距离（∈[0,la]），
		// 不能用只认 [0,1] 的 clamp01，否则 la>1 时相接点会被夹错到 1 米处。
		s := clampRange((lo+hi)/2.0, 0, la)
		t := s / la
		pe, pn := a0.e+s*ux, a0.n+s*uy
		// 求该点在 B 段上的参数：用 B 方向点积（共线，方向可能相反）。
		ubx, uby := rb.e/lb, rb.n/lb
		u := clamp01(((pe-b0.e)*ubx + (pn-b0.n)*uby) / lb)
		return segHit{e: pe, n: pn, ta: t, tb: u}, true
	}
	return segHit{}, false
}

func appendDedup(hits []segHit, h segHit, ai, bi int, tol float64) []segHit {
	for k := range hits {
		if math.Hypot(hits[k].e-h.e, hits[k].n-h.n) <= tol {
			return hits // 同一几何点已记录
		}
	}
	h.ai, h.bi = ai, bi
	return append(hits, h)
}

// straightPolyline 判断折线的所有顶点是否落在同一条支撑直线上（相对容限）。
// 退化情形（全长为 0）返回 false，交由上游按零距离异常处理。
func straightPolyline(pts []orderedPoint) bool {
	if len(pts) < 3 {
		return true
	}
	// 选一条非零方向作基准。
	var dir xyPoint
	base := 0
	for i := 0; i+1 < len(pts); i++ {
		d := xyPoint{pts[i+1].Point.Easting - pts[i].Point.Easting,
			pts[i+1].Point.Northing - pts[i].Point.Northing}
		if math.Hypot(d.e, d.n) >= zeroDistanceEpsM {
			dir, base = d, i
			break
		}
	}
	if math.Hypot(dir.e, dir.n) < zeroDistanceEpsM {
		return false
	}
	origin := pts[base]
	for i := 0; i+1 < len(pts); i++ {
		edge := xyPoint{pts[i+1].Point.Easting - pts[i].Point.Easting,
			pts[i+1].Point.Northing - pts[i].Point.Northing}
		if math.Hypot(edge.e, edge.n) < zeroDistanceEpsM {
			continue
		}
		rel := xyPoint{pts[i+1].Point.Easting - origin.Point.Easting,
			pts[i+1].Point.Northing - origin.Point.Northing}
		scale := math.Max(1.0, math.Hypot(rel.e, rel.n)*math.Hypot(dir.e, dir.n))
		if math.Abs(cross(rel, dir)) > geomEps*scale {
			return false
		}
	}
	return true
}

// supportLineRelation 对两条“均为直线”的折线，按支撑直线分类：
//
//	parallel_collinear / parallel_distinct / intersecting
//
// intersecting 时回传支撑线交点及在两条线首末点段上的位置参数。
func supportLineRelation(ptsA, ptsB []orderedPoint) (kind string, hit xyPoint, ta, tb float64) {
	a0, a1 := xyOf(ptsA[0]), xyOf(ptsA[len(ptsA)-1])
	b0, b1 := xyOf(ptsB[0]), xyOf(ptsB[len(ptsB)-1])
	ra := xyPoint{a1.e - a0.e, a1.n - a0.n}
	rb := xyPoint{b1.e - b0.e, b1.n - b0.n}
	qp := xyPoint{b0.e - a0.e, b0.n - a0.n}

	parScale := math.Max(1.0, math.Abs(ra.e)*math.Abs(rb.n)+math.Abs(ra.n)*math.Abs(rb.e))
	if math.Abs(cross(ra, rb)) <= geomEps*parScale {
		colScale := math.Max(1.0, math.Abs(qp.e)*math.Abs(ra.n)+math.Abs(qp.n)*math.Abs(ra.e))
		if math.Abs(cross(qp, ra)) <= geomEps*colScale {
			return "parallel_collinear", xyPoint{}, 0, 0
		}
		return "parallel_distinct", xyPoint{}, 0, 0
	}

	den := cross(ra, rb)
	ta = cross(qp, rb) / den
	tb = cross(qp, ra) / den
	return "intersecting", xyPoint{e: a0.e + ta*ra.e, n: a0.n + ta*ra.n}, ta, tb
}

// collinearRangesOverlap 判断两条共线直线折线的测量段是否有公共点（含端点相接）。
func collinearRangesOverlap(ptsA, ptsB []orderedPoint) bool {
	a0, a1 := xyOf(ptsA[0]), xyOf(ptsA[len(ptsA)-1])
	dir := xyPoint{a1.e - a0.e, a1.n - a0.n}
	l := math.Hypot(dir.e, dir.n)
	if l < zeroDistanceEpsM {
		return false
	}
	ux, uy := dir.e/l, dir.n/l
	proj := func(p xyPoint) float64 { return (p.e-a0.e)*ux + (p.n-a0.n)*uy }

	// 以 A 首末点投影为 A 的测量范围；B 取其全部顶点投影的最小/最大值。
	bMin, bMax := proj(xyOf(ptsB[0])), proj(xyOf(ptsB[0]))
	for _, p := range ptsB[1:] {
		v := proj(xyOf(p))
		bMin, bMax = math.Min(bMin, v), math.Max(bMax, v)
	}
	lo := math.Max(0.0, bMin)
	hi := math.Min(l, bMax)
	return hi >= lo-geomEps*math.Max(1.0, l)
}

func xyOf(p orderedPoint) xyPoint { return xyPoint{e: p.Point.Easting, n: p.Point.Northing} }

func distXY(a, b xyPoint) float64 { return math.Hypot(b.e-a.e, b.n-a.n) }

// coordMagnitudeScale 给出两线坐标的量级，用于交点去重的容限。
func coordMagnitudeScale(a, b []orderedPoint) float64 {
	s := 1.0
	upd := func(pts []orderedPoint) {
		for _, p := range pts {
			s = math.Max(s, math.Max(math.Abs(p.Point.Easting), math.Abs(p.Point.Northing)))
		}
	}
	upd(a)
	upd(b)
	return s
}

// interpAnomaly 在折线段 segIndex 上按位置参数 t 对布格异常做线性插值。
func interpAnomaly(pts []orderedPoint, segIndex int, t float64) float64 {
	v0 := pts[segIndex].Result.BouguerAnomalyMGal
	v1 := pts[segIndex+1].Result.BouguerAnomalyMGal
	return v0 + clamp01(t)*(v1-v0)
}
