package survey

// zeroDistanceEpsilonM 是判定“同一位置重复布点”的平面距离阈值（米）。
// 低于此值即认为两点水平重合，梯度无定义，作为数据异常报出。
const zeroDistanceEpsilonM = 1e-9

// mGalPerMToMGalPerKm 是水平梯度单位换算：1 mGal/m = 1000 mGal/km。
const mGalPerMToMGalPerKm = 1000.0

// diagnoseGradient 对一条已按里程理顺的线逐段做水平梯度检查。
//
// 水平梯度 = 相邻两点布格异常差 / 两点水平距离（mGal/km）。
// 阈值由调用方给定；超过阈值的段标为可疑，并给出超出量。
//
// 退化情形：相邻测点平面位置重合（距离为 0）时，梯度在数学上无定义，
// 这里绝不除以零产生 Inf/NaN，而是把该段作为“coincident（同位置重复布点）”
// 数据异常明确报出，其余段照常诊断。
func diagnoseGradient(pts []orderedPoint, thresholdMGalPerKm float64) GradientReport {
	rep := GradientReport{Status: "ok", Segments: []SegmentResult{}}

	for i := 0; i+1 < len(pts); i++ {
		from, to := pts[i], pts[i+1]
		seg := SegmentResult{
			FromID:             from.in.ID,
			ToID:               to.in.ID,
			FromStationM:       from.in.StationM,
			ToStationM:         to.in.StationM,
			ThresholdMGalPerKm: thresholdMGalPerKm,
		}

		dist := horizontalDistanceM(from, to)
		seg.HorizontalDistanceM = dist
		delta := to.res.BouguerAnomalyMGal - from.res.BouguerAnomalyMGal
		seg.AnomalyDeltaMGal = delta

		if dist <= zeroDistanceEpsilonM {
			// 零距离：不能硬算梯度（否则除零成 Inf 后崩掉），报明确数据异常。
			seg.Kind = SegmentCoincident
			rep.CoincidentCount++
			rep.Segments = append(rep.Segments, seg)
			continue
		}

		seg.Kind = SegmentNormal
		grad := (delta / dist) * mGalPerMToMGalPerKm
		seg.GradientMGalPerKm = grad
		if absF(grad) > thresholdMGalPerKm {
			seg.Suspicious = true
			seg.ExcessMGalPerKm = absF(grad) - thresholdMGalPerKm
			rep.SuspiciousCount++
		}
		rep.Segments = append(rep.Segments, seg)
	}
	return rep
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
