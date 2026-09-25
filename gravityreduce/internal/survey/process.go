// 批量归算与质量诊断的编排入口：组织排序 → 逐点归算（复用单点链条）→
// 沿测线梯度检查 → 线内统计 → 测线两两交点差。
// 各步骤的具体职责分别在 ordering / gradient / statistics / geometry / intersections 中。
package survey

// Process 对一批测线完成批量归算与质控诊断。
//
// 调用前应先用 Validate 校验；本函数假定输入已无硬错误。
// 对缺里程、重复里程、单点测线、零距离重复布点等撑不起部分诊断的数据情形，
// 不返回错误，而是在对应线级结果中带原因降级（Status=degraded、Notes 说明），
// 逐点归算与统计画像始终照给——绝不回一个看似正常其实没意义的结果。
func Process(lines []Line, th Thresholds) BatchResult {
	out := BatchResult{
		Thresholds: th,
		Lines:      make([]LineResult, 0, len(lines)),
	}

	for _, line := range lines {
		lr := LineResult{
			LineID: line.ID,
			Status: StatusOK,
			Notes:  []string{},
		}

		ordered, basis, notes := orderLine(line)
		lr.OrderingBasis = basis
		lr.Notes = append(lr.Notes, notes...)

		lr.Ordered = make([]PointView, len(ordered))
		for i, op := range ordered {
			lr.Ordered[i] = PointView{Point: op.Point, Order: i, Result: op.Result}
		}

		// 统计画像只依赖各点真实归算结果，任何情形都给出。
		lr.Stats = lineStatistics(ordered)

		if basis == orderingBasisStation {
			lr.Gradient = gradientDiagnosis(line.ID, ordered, th.GradientMGalPerM)
			// 零距离是明确数据异常，同样触发降级并逐条说明。
			for _, za := range lr.Gradient.DataAnomalies {
				lr.Notes = append(lr.Notes,
					"数据异常：测点 "+za.FromPointID+" 与 "+za.ToPointID+" 平面位置重合（"+za.Reason+"）")
			}
			if len(lr.Gradient.DataAnomalies) > 0 {
				lr.Status = StatusDegraded
			}
		} else {
			lr.Gradient = GradientReport{
				Available:     false,
				Segments:      []GradientSegment{},
				DataAnomalies: []ZeroDistanceAnomaly{},
			}
			lr.Status = StatusDegraded
		}

		out.Lines = append(out.Lines, lr)
	}

	out.Pairs = diagnosePairs(out.Lines, th.IntersectionTolMGal)
	return out
}
