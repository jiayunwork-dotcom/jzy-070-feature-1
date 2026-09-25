// 测线两两交点差诊断：先按两条线的几何走向求平面交点并判定它是否落在两条线的
// 测量范围内，再在每条线上按交点位置对相邻测点的布格异常做线性插值，最后两值相减。
//
// 几何上不相交、平行、共线（无唯一交点）、交点落在测量段之外、某条线无法参与
// 插值等情形，都不产生交点差，而是如实标注为“无有效交点”，绝不硬凑一个数。
package survey

import "math"

// 无有效交点的机器可读原因码。
const (
	ReasonLineUnavailable       = "line_unavailable"
	ReasonNoHorizontalExtent    = "no_horizontal_extent"
	ReasonParallelDistinct      = "parallel_distinct"
	ReasonCollinear             = "collinear_overlap_or_coincident"
	ReasonCrossingOutsideSurvey = "crossing_outside_surveyed_range"
	ReasonNoSegmentCrossing     = "no_segment_crossing"
)

// crossingEps 用于在支撑线交点恰好压在测量段端点附近时的范围判定（相对容限）。
const crossingEps = geomEps

// diagnosePairs 对所有测线两两求交点差。
func diagnosePairs(results []LineResult, tolMGal float64) []PairResult {
	var pairs []PairResult
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			pairs = append(pairs, diagnosePair(results[i], results[j], tolMGal))
		}
	}
	return pairs
}

func diagnosePair(ra, rb LineResult, tolMGal float64) PairResult {
	pair := PairResult{
		LineAID:   ra.LineID,
		LineBID:   rb.LineID,
		Status:    PairStatusNoValidCrossing,
		Crossings: []Crossing{},
	}

	if !ra.Gradient.Available || !rb.Gradient.Available {
		return pair.mark(ReasonLineUnavailable,
			"测线 "+unavailableNames(ra, rb)+" 无法定序或仅含单点，交点处插值需要相邻测点对")
	}
	pa := orderedViews(ra)
	pb := orderedViews(rb)

	extA := polylineExtent(pa)
	extB := polylineExtent(pb)
	if extA < zeroDistanceEpsM || extB < zeroDistanceEpsM {
		return pair.mark(ReasonNoHorizontalExtent,
			"至少一条测线所有测点平面位置重合（无水平走向），几何上无法确定唯一交点")
	}

	hits, _, _ := polylineCrossings(pa, pb)
	if len(hits) > 0 {
		pair.Status = PairStatusIntersects
		pair.Reason = ""
		pair.ReasonDetail = ""
		for _, h := range hits {
			vA := interpAnomaly(pa, h.ai, h.ta)
			vB := interpAnomaly(pb, h.bi, h.tb)
			diff := vA - vB // 交点差约定为 A 线插值 − B 线插值
			pair.Crossings = append(pair.Crossings, Crossing{
				Easting:                 h.e,
				Northing:                h.n,
				LineASegmentFromPointID: pa[h.ai].Point.ID,
				LineASegmentToPointID:   pa[h.ai+1].Point.ID,
				LineBSegmentFromPointID: pb[h.bi].Point.ID,
				LineBSegmentToPointID:   pb[h.bi+1].Point.ID,
				InterpAMGal:             vA,
				InterpBMGal:             vB,
				DifferenceMGal:          diff,
				ToleranceMGal:           tolMGal,
				WithinTolerance:         math.Abs(diff) <= tolMGal,
			})
		}
		return pair
	}

	// 没有任何段命中：结合两线是否为直线（走向唯一）对“无有效交点”做几何分类。
	straightA, straightB := straightPolyline(pa), straightPolyline(pb)
	if straightA && straightB {
		kind, _, ta, tb := supportLineRelation(pa, pb)
		switch kind {
		case "parallel_distinct":
			return pair.mark(ReasonParallelDistinct,
				"两条测线均为直线但走向平行且不共线，任意延长都不会相交")
		case "parallel_collinear":
			if collinearRangesOverlap(pa, pb) {
				return pair.mark(ReasonCollinear,
					"两条直线测线共线且测量段重叠，交点不唯一")
			}
			return pair.mark(ReasonCollinear,
				"两条直线测线共线但测量段不相接，没有交点")
		default:
			// 支撑线相交却无段命中：交点必在至少一条线的测量段之外。
			inA := ta >= -crossingEps*extentParamScale(pa) && ta <= 1+crossingEps*extentParamScale(pa)
			inB := tb >= -crossingEps*extentParamScale(pb) && tb <= 1+crossingEps*extentParamScale(pb)
			which := ""
			if !inA {
				which += ra.LineID + " "
			}
			if !inB {
				which += rb.LineID
			}
			return pair.mark(ReasonCrossingOutsideSurvey,
				"支撑线交点在测线 "+which+" 的首末测点范围之外（延长线相交，测量范围内不相交）")
		}
	}
	return pair.mark(ReasonNoSegmentCrossing,
		"两条折线测线的测量段之间不存在落在各自测量范围内的交点（至少一条线存在折点）")
}

// LineBIDSafe 占位已移除：PairResult 在 diagnosePair 中直接初始化。

func (p PairResult) mark(code, detail string) PairResult {
	p.Status = PairStatusNoValidCrossing
	p.Reason = code
	p.ReasonDetail = detail
	p.Crossings = []Crossing{}
	return p
}

func unavailableNames(ra, rb LineResult) string {
	var names []string
	if !ra.Gradient.Available {
		names = append(names, ra.LineID)
	}
	if !rb.Gradient.Available {
		names = append(names, rb.LineID)
	}
	out := ""
	for i, n := range names {
		if i > 0 {
			out += "、"
		}
		out += n
	}
	return out
}

func orderedViews(lr LineResult) []orderedPoint {
	out := make([]orderedPoint, len(lr.Ordered))
	for i, v := range lr.Ordered {
		out[i] = orderedPoint{Point: v.Point, Result: v.Result}
	}
	return out
}

// polylineExtent 折线相邻段长度之和；为 0 表示所有测点平面重合。
func polylineExtent(pts []orderedPoint) float64 {
	total := 0.0
	for i := 0; i+1 < len(pts); i++ {
		total += distXY(xyOf(pts[i]), xyOf(pts[i+1]))
	}
	return total
}

// extentParamScale 支撑线参数范围判定的坐标量级（参数按首末段全长归一）。
func extentParamScale(pts []orderedPoint) float64 {
	return math.Max(1.0, distXY(xyOf(pts[0]), xyOf(pts[len(pts)-1])))
}
