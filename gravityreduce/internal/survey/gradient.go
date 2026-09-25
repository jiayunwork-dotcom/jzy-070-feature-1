// 沿测线的布格异常水平梯度检查。
//
// 测点按里程排好后，对每个相邻段计算：
//
//	g = (Δg_B[To] − Δg_B[From]) / 水平距离(To, From)   单位 mGal/m
//
// 地下介质通常渐变，小段梯度异乎寻常地大，多半是粗差、串点或坐标记错。
// 同位置重复布点（距离为 0）是退化数据异常：梯度无定义，绝不能算成无穷大后
// 崩掉，而要作为明确的数据异常单列报出。
package survey

import "math"

// 距离小于该阈值（米）即判为同位置重复布点。
const zeroDistanceEpsM = 1e-9

// gradientDiagnosis 对已按里程排序的测点逐段做梯度诊断。
func gradientDiagnosis(lineID string, ordered []orderedPoint, thresholdMGalPerM float64) GradientReport {
	rep := GradientReport{
		Available:         true,
		ThresholdMGalPerM: thresholdMGalPerM,
		Segments:          []GradientSegment{},
		DataAnomalies:     []ZeroDistanceAnomaly{},
	}

	for i := 0; i+1 < len(ordered); i++ {
		from, to := ordered[i], ordered[i+1]
		dist := horizontalDistanceM(from.Point, to.Point)

		if dist < zeroDistanceEpsM {
			rep.DataAnomalies = append(rep.DataAnomalies, ZeroDistanceAnomaly{
				LineID:      lineID,
				FromPointID: from.Point.ID,
				ToPointID:   to.Point.ID,
				FromStation: *from.Point.Station,
				ToStation:   *to.Point.Station,
				Reason: "两个相邻测点平面位置重合，水平距离为 0，水平梯度无定义" +
					"（可能为重复布点或坐标记错）；该段不计算梯度",
			})
			continue
		}

		grad := (to.Result.BouguerAnomalyMGal - from.Result.BouguerAnomalyMGal) / dist
		seg := GradientSegment{
			FromPointID:       from.Point.ID,
			ToPointID:         to.Point.ID,
			FromStation:       *from.Point.Station,
			ToStation:         *to.Point.Station,
			DistanceM:         dist,
			GradientMGalPerM:  grad,
			ThresholdMGalPerM: thresholdMGalPerM,
		}
		if math.Abs(grad) > thresholdMGalPerM {
			seg.Suspicious = true
			seg.ExceedsByMGalPerM = math.Abs(grad) - thresholdMGalPerM
		}
		rep.Segments = append(rep.Segments, seg)
	}

	return rep
}

// horizontalDistanceM 计算两测点在平面直角坐标系中的水平距离（米）。
func horizontalDistanceM(a, b Point) float64 {
	de, dn := b.Easting-a.Easting, b.Northing-a.Northing
	return math.Hypot(de, dn)
}
