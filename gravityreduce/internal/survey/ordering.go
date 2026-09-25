// 测线的组织与排序：野外测点未必按里程顺序送进来，梯度与交点诊断前
// 必须先理顺次序；缺里程、里程重复等撑不起定序的情形在此带原因降级。
package survey

import (
	"fmt"
	"sort"
)

// orderingBasisStation 表示本层支持的唯一定序依据：沿线里程/桩号（米）。
const orderingBasisStation = "station_m"

// orderLine 把一条线的测点按里程升序排好，并逐点复用 gravity.Reduce 完成归算。
//
// 返回值：
//   - ordered：排好序的测点（含真实归算结果）；无法定序时按送入顺序给出；
//   - basis：定序依据，无法定序时为 ""；
//   - notes：降级原因（缺里程、重复里程、单点），无降级时为 nil。
func orderLine(line Line) (ordered []orderedPoint, basis string, notes []string) {
	hasStation := len(line.Points) > 0 && line.Points[0].Station != nil

	// 先对每个点跑单点归算——归算只取决于纬度、高程、密度，与排序无关。
	ordered = make([]orderedPoint, len(line.Points))
	for i, p := range line.Points {
		ordered[i] = orderedPoint{Point: p, Result: reducePoint(p)}
	}

	if len(ordered) < 2 {
		notes = append(notes, fmt.Sprintf("测线 %q 只有 %d 个测点，无法形成相邻段，梯度检查与交点插值诊断降级（逐点归算与统计仍给出）",
			line.ID, len(ordered)))
		return ordered, "", notes
	}

	if !hasStation {
		notes = append(notes, fmt.Sprintf(
			"测线 %q 未提供任何测点的里程/桩号（station_m），无法确定沿线先后次序，梯度检查与交点插值诊断降级",
			line.ID))
		return ordered, "", notes
	}

	// 稳定排序：里程相同时保留送入相对次序，随后把重复里程作为数据异常降级。
	sort.SliceStable(ordered, func(i, j int) bool {
		return *ordered[i].Point.Station < *ordered[j].Point.Station
	})
	var dup []string
	for i := 1; i < len(ordered); i++ {
		if *ordered[i].Point.Station == *ordered[i-1].Point.Station {
			dup = append(dup, fmt.Sprintf("%s=%g m", ordered[i].Point.ID, *ordered[i].Point.Station))
		}
	}
	if len(dup) > 0 {
		// 只列前若干个，避免一条坏线刷爆响应。
		lbl := dup
		if len(lbl) > 5 {
			lbl = append(lbl[:5], fmt.Sprintf("等 %d 处", len(dup)))
		}
		notes = append(notes, fmt.Sprintf(
			"测线 %q 存在 %d 处里程/桩号相同的测点（%s），沿线先后次序无法唯一确定，梯度检查与交点插值诊断降级",
			line.ID, len(dup), joinLabels(lbl)))
		return ordered, "", notes
	}

	return ordered, orderingBasisStation, nil
}

func joinLabels(parts []string) string {
	out := ""
	for i, s := range parts {
		if i > 0 {
			out += "、"
		}
		out += s
	}
	return out
}
