package survey

import "math"

// processedLine 内部携带一条线的报告与其排序后测点、非零段，供配对阶段复用。
type processedLine struct {
	report   LineReport
	ordered  []orderedPoint
	segs     []seg
	degraded bool
}

// Process 是批量层的唯一领域入口：校验通过后，对每条线逐点复用单点归算，
// 完成沿测线梯度诊断、线内统计，并对所有测线两两做交点差诊断。
//
// 调用方应先调用 ValidateRequest；Process 假定输入已校验。
func Process(req Request) Report {
	plines := make([]processedLine, 0, len(req.Lines))
	for _, line := range req.Lines {
		plines = append(plines, processLine(line, req.GradientThresholdMGalPerKm))
	}

	rep := Report{Lines: make([]LineReport, 0, len(plines))}
	for _, pl := range plines {
		rep.Lines = append(rep.Lines, pl.report)
	}

	// 所有测线两两配对（i<j，每对一次）。
	rep.Pairs = make([]PairReport, 0)
	for i := 0; i < len(plines); i++ {
		for j := i + 1; j < len(plines); j++ {
			rep.Pairs = append(rep.Pairs, processPair(plines[i], plines[j], req.IntersectionToleranceMGal))
		}
	}
	return rep
}

// processLine 处理单条线：归算、定序、统计、梯度诊断。
func processLine(line LineInput, thresholdMGalPerKm float64) processedLine {
	pts, reasons := reduceAndOrder(line)
	degraded := len(reasons) > 0

	lr := LineReport{
		ID:             line.ID,
		Points:         make([]PointResult, 0, len(pts)),
		DegradeReasons: reasons,
	}
	for _, p := range pts {
		lr.Points = append(lr.Points, toPointView(p))
	}
	// 统计始终基于真实归算结果，即便诊断降级也照常给出。
	lr.Statistics = computeStatistics(pts)

	var segs []seg
	if degraded {
		lr.Status = StatusDegraded
		lr.Gradient = GradientReport{
			Status:     "skipped",
			SkipReason: reasons[0],
			Segments:   []SegmentResult{},
		}
	} else {
		lr.Status = StatusOK
		lr.Gradient = diagnoseGradient(pts, thresholdMGalPerKm)
		segs = lineSegments(pts)
	}

	return processedLine{report: lr, ordered: pts, segs: segs, degraded: degraded}
}

// processPair 求两条线在实测段范围内的唯一平面交点，并在各自线上
// 对交点处布格异常做线性插值，最后给出交点差。
//
// 平行、共线、几何不相交、交点落在实测段之外、任一线降级等情形，
// 一律如实报为“无有效交点”，绝不硬凑一个差值。
func processPair(a, b processedLine, toleranceMGal float64) PairReport {
	pair := PairReport{LineAID: a.report.ID, LineBID: b.report.ID, ToleranceMGal: toleranceMGal}

	if a.degraded || b.degraded {
		pair.Status = StatusNoIntersection
		pair.Reason = reasonFor(ReasonLineDegraded)
		return pair
	}
	if len(a.segs) == 0 || len(b.segs) == 0 {
		pair.Status = StatusNoIntersection
		pair.Reason = reasonFor(ReasonZeroLengthLine)
		return pair
	}

	hits := findIntersections(a.segs, b.segs)
	if len(hits) == 0 {
		pair.Status = StatusNoIntersection
		pair.Reason = classifyNoIntersection(a, b)
		return pair
	}

	// 用落在实测段上的交点。多条段在同一点相交已在 findIntersections 去重；
	// 若两条折线确有多个不同交点，取最先出现的一个并逐段插值，
	// 这仍然是“实测段上的真实交点”，不是硬凑。
	h := hits[0]
	sideA := interpolateAt(a.report.ID, a.segs[h.ai], h.tA)
	sideB := interpolateAt(b.report.ID, b.segs[h.bi], h.tB)
	diff := sideA.AnomalyMGal - sideB.AnomalyMGal

	pair.Status = StatusIntersected
	pair.EastingM = h.p.x
	pair.NorthingM = h.p.y
	pair.A = &sideA
	pair.B = &sideB
	pair.DifferenceMGal = diff
	pair.ExceedsTolerance = math.Abs(diff) > toleranceMGal
	return pair
}

// classifyNoIntersection 在“实测段上无交点”的前提下区分原因：
// 平行线走向不相交、共线交点不唯一、或延长线交点落在实测范围之外。
// 判定以每条线排序后的首末测点张成的实测走向为基准。
func classifyNoIntersection(a, b processedLine) Reason {
	A0 := pointOf(a.ordered[0])
	A1 := pointOf(a.ordered[len(a.ordered)-1])
	B0 := pointOf(b.ordered[0])
	B1 := pointOf(b.ordered[len(b.ordered)-1])
	dA := sub(A1, A0)
	dB := sub(B1, B0)
	lenA := math.Hypot(dA.x, dA.y)
	lenB := math.Hypot(dB.x, dB.y)
	if lenA <= zeroDistanceEpsilonM || lenB <= zeroDistanceEpsilonM {
		return reasonFor(ReasonZeroLengthLine)
	}

	denom := cross(dA, dB)
	scale := math.Max(1.0, lenA) * math.Max(1.0, lenB)
	if math.Abs(denom) <= onSegmentEps*scale {
		// 走向平行：共线时叉积 (B0−A0)×dA 也为 0。
		if math.Abs(cross(sub(B0, A0), dA)) <= onSegmentEps*math.Max(1.0, lenA*lenB) {
			return reasonFor(ReasonCollinear)
		}
		return reasonFor(ReasonParallel)
	}
	return reasonFor(ReasonOutOfRange)
}
