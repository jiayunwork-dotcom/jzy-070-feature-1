package survey

import (
	"math"
	"sort"
	"strconv"

	"gravityreduce/internal/gravity"
)

// orderedPoint 携带排序后的测点及其逐点真实归算结果。
type orderedPoint struct {
	in  PointInput
	res gravity.Result
}

// reduceAndOrder 对一条线逐点复用既有单点归算，并按里程理顺测点先后。
//
// 返回：
//   - ordered：测点序列。可定序时按里程升序；否则保留输入顺序。
//   - reasons：降级原因（单点、里程缺失/重复），非空表示该线诊断降级，
//     但逐点归算与统计始终照做，绝不用看似正常其实无意义的结果搪塞。
//
// 注意：归算只取决于纬度、高程、密度，与平面坐标完全无关；
// 平面坐标（easting/northing）只在梯度与求交诊断中使用。
func reduceAndOrder(line LineInput) ([]orderedPoint, []Reason) {
	pts := make([]orderedPoint, len(line.Points))
	for i, p := range line.Points {
		// 逐点复用既有单点物理链条，不重写任何公式。
		r := gravity.Reduce(gravity.Observation{
			GobsMS2: p.GobsMS2,
			H:       p.H,
			Phi:     p.Phi,
			Rho:     p.Rho,
		})
		pts[i] = orderedPoint{in: p, res: r}
	}

	var reasons []Reason
	if len(pts) < 2 {
		reasons = append(reasons, reasonFor(ReasonSinglePoint))
		return pts, reasons
	}

	// 里程定序：每个点都必须有里程，且里程互不相同，次序才唯一。
	missing := false
	stationOf := make([]float64, len(pts))
	for i, p := range pts {
		if !p.in.HasStation {
			missing = true
			break
		}
		stationOf[i] = p.in.StationM
	}
	if missing {
		reasons = append(reasons, reasonFor(ReasonStationMissing))
		return pts, reasons
	}
	if dup := findDuplicateStation(stationOf); dup != nil {
		r := reasonFor(ReasonStationDuplicate)
		r.Message = "存在重复里程（station_m=" + trimFloat(*dup) + "），测点先后次序不唯一，梯度与交点诊断已降级跳过"
		reasons = append(reasons, r)
		return pts, reasons
	}

	// 测点未必按里程顺序送进来——这里统一理顺，绝不假定输入有序。
	sort.SliceStable(pts, func(i, j int) bool {
		return pts[i].in.StationM < pts[j].in.StationM
	})
	return pts, nil
}

// findDuplicateStation 在里程序列中找一个重复值；无重复返回 nil。
func findDuplicateStation(stations []float64) *float64 {
	seen := make(map[float64]struct{}, len(stations))
	for _, s := range stations {
		if _, ok := seen[s]; ok {
			v := s
			return &v
		}
		seen[s] = struct{}{}
	}
	return nil
}

// trimFloat 把浮点里程格式化成简短字符串，仅用于中文原因描述。
func trimFloat(x float64) string {
	return strconv.FormatFloat(x, 'g', -1, 64)
}

// toPointView 把内部点结果转成报告用的 PointResult。
func toPointView(p orderedPoint) PointResult {
	return PointResult{
		ID:                 p.in.ID,
		StationM:           p.in.StationM,
		HasStation:         p.in.HasStation,
		EastingM:           p.in.EastingM,
		NorthingM:          p.in.NorthingM,
		HM:                 p.in.H,
		BouguerAnomalyMGal: p.res.BouguerAnomalyMGal,
		BouguerAnomalyMS2:  p.res.BouguerAnomalyMS2,
	}
}

// computeStatistics 由该线各点真实归算结果计算统计画像。
// 标准差采用样本标准差（n−1）；单点时标准差为 0（已在降级原因中说明）。
func computeStatistics(pts []orderedPoint) Statistics {
	var s Statistics
	s.PointCount = len(pts)
	if len(pts) == 0 {
		return s
	}

	sum := 0.0
	minIdx, maxIdx := 0, 0
	for i, p := range pts {
		v := p.res.BouguerAnomalyMGal
		sum += v
		if v < pts[minIdx].res.BouguerAnomalyMGal {
			minIdx = i
		}
		if v > pts[maxIdx].res.BouguerAnomalyMGal {
			maxIdx = i
		}
	}
	mean := sum / float64(len(pts))
	s.MeanMGal = mean

	sumSq := 0.0
	for _, p := range pts {
		d := p.res.BouguerAnomalyMGal - mean
		sumSq += d * d
	}
	if n := len(pts); n >= 2 {
		s.StdMGal = math.Sqrt(sumSq / float64(n-1))
	}

	s.Min = Extremum{PointID: pts[minIdx].in.ID, ValueMGal: pts[minIdx].res.BouguerAnomalyMGal}
	s.Max = Extremum{PointID: pts[maxIdx].in.ID, ValueMGal: pts[maxIdx].res.BouguerAnomalyMGal}
	return s
}

// horizontalDistanceM 两点平面水平距离（米）。
func horizontalDistanceM(a, b orderedPoint) float64 {
	de := b.in.EastingM - a.in.EastingM
	dn := b.in.NorthingM - a.in.NorthingM
	return math.Hypot(de, dn)
}
