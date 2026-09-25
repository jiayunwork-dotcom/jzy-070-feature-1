// 每条测线布格异常的整体统计画像：点数、均值、总体标准差、
// 最大值/最小值及各自对应的测点。所有统计量都由该线各点的真实归算结果算出，
// 不允许用固定值或近似占位。
package survey

import "math"

// lineStatistics 计算一条线（按任意顺序给出均可）的布格异常统计画像。
// 调用方保证 points 非空；单点时标准差为 0，最小/最大均落在该点。
func lineStatistics(ordered []orderedPoint) Statistics {
	n := len(ordered)
	stats := Statistics{Count: n}

	sum := 0.0
	minIdx, maxIdx := 0, 0
	for i, p := range ordered {
		v := p.Result.BouguerAnomalyMGal
		sum += v
		if v < ordered[minIdx].Result.BouguerAnomalyMGal {
			minIdx = i
		}
		if v > ordered[maxIdx].Result.BouguerAnomalyMGal {
			maxIdx = i
		}
	}
	mean := sum / float64(n)

	var sq float64
	for _, p := range ordered {
		d := p.Result.BouguerAnomalyMGal - mean
		sq += d * d
	}

	stats.MeanMGal = mean
	stats.StdMGal = math.Sqrt(sq / float64(n))
	stats.MinMGal = ordered[minIdx].Result.BouguerAnomalyMGal
	stats.MinPointID = ordered[minIdx].Point.ID
	stats.MaxMGal = ordered[maxIdx].Result.BouguerAnomalyMGal
	stats.MaxPointID = ordered[maxIdx].Point.ID
	return stats
}
