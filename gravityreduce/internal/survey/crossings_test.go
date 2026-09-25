package survey

import (
	"math"
	"testing"
)

// 两条十字交叉的测线：L1 东西向（y=0，x∈[0,400]），L2 南北向（x=300，y∈[-100,100]）。
// 几何交点为 (300, 0)，落在两条线的测点段中间（L1 段 200→400 的 t=0.5；L2 段 -100→100 的 t=0.5）。
func crossLines(anomL1, anomL2 []float64, h1, h2 float64) []Line {
	const l1y, l2x = 0.0, 300.0
	p1 := []Point{
		mkPointH("L1-a", 0, l1y, 0, anomL1[0], h1),
		mkPointH("L1-b", 200, l1y, 200, anomL1[1], h1),
		mkPointH("L1-c", 400, l1y, 400, anomL1[2], h1),
	}
	p2 := []Point{
		mkPointH("L2-a", l2x, -100, 0, anomL2[0], h2),
		mkPointH("L2-b", l2x, 100, 200, anomL2[1], h2),
	}
	return []Line{{ID: "L1", Points: p1}, {ID: "L2", Points: p2}}
}

// TestCrossingEqualAnomalyZeroDifference 验收关系：两线交点附近布格异常相等，
// 交点差应为 0（含非中点位置的插值，不能是端点碰巧相等）。
func TestCrossingEqualAnomalyZeroDifference(t *testing.T) {
	// L1 异常线性：0,2,4 → 在 x=300 处插值 3。
	// L2 两测点异常同为 3 → 任意位置插值都是 3。
	lines := crossLines([]float64{0, 2, 4}, []float64{3, 3}, testH, testH)
	res := mustProcess(t, lines, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 0.5})
	pair := findPair(t, res, "L1", "L2")
	if pair.Status != PairStatusIntersects {
		t.Fatalf("应有有效交点：status=%s reason=%s %s", pair.Status, pair.Reason, pair.ReasonDetail)
	}
	if len(pair.Crossings) != 1 {
		t.Fatalf("应恰好 1 个交点，实际 %d", len(pair.Crossings))
	}
	cr := pair.Crossings[0]
	if math.Abs(cr.Easting-300) > 1e-6 || math.Abs(cr.Northing-0) > 1e-6 {
		t.Fatalf("交点坐标应为 (300,0)，实际 (%v,%v)", cr.Easting, cr.Northing)
	}
	if math.Abs(cr.InterpAMGal-3) > 1e-6 || math.Abs(cr.InterpBMGal-3) > 1e-6 {
		t.Fatalf("两线交点插值都应为 3：A=%v B=%v", cr.InterpAMGal, cr.InterpBMGal)
	}
	if math.Abs(cr.DifferenceMGal) > 1e-9 {
		t.Fatalf("等值交点差应为 0，实际 %v", cr.DifferenceMGal)
	}
	if !cr.WithinTolerance {
		t.Fatalf("零交点差应在容限内")
	}
	// 插值所用段必须是包含交点的那一对相邻测点。
	if cr.LineASegmentFromPointID != "L1-b" || cr.LineASegmentToPointID != "L1-c" ||
		cr.LineBSegmentFromPointID != "L2-a" || cr.LineBSegmentToPointID != "L2-b" {
		t.Fatalf("插值测点段错误：A %s→%s B %s→%s",
			cr.LineASegmentFromPointID, cr.LineASegmentToPointID,
			cr.LineBSegmentFromPointID, cr.LineBSegmentToPointID)
	}
}

// TestCrossingInterpolatesAtNonMidpoint 交点不在段正中时也必须按位置线性插值。
func TestCrossingInterpolatesAtNonMidpoint(t *testing.T) {
	// L2 的点改为 y=-150 与 y=150 仍关于 0 对称；把交点改到非中点：
	// L2 用 y∈[-200,100]：点 (-? )，交点 y=0 落在 t=200/300=2/3。
	p1 := []Point{
		mkPoint("L1-a", 0, 0, 0, 0),
		mkPoint("L1-b", 200, 0, 200, 2),
		mkPoint("L1-c", 400, 0, 400, 4),
	}
	// L2 异常线性 6（y=-200，里程0）→ 9（y=100，里程300），交点 t=2/3 → 8。
	p2 := []Point{
		mkPoint("L2-a", 300, -200, 0, 6),
		mkPoint("L2-b", 300, 100, 300, 9),
	}
	res := mustProcess(t, []Line{{ID: "L1", Points: p1}, {ID: "L2", Points: p2}},
		Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 100})
	cr := findPair(t, res, "L1", "L2").Crossings[0]
	if math.Abs(cr.InterpAMGal-3) > 1e-9 {
		t.Fatalf("L1 非中点插值应为 3，实际 %v", cr.InterpAMGal)
	}
	if math.Abs(cr.InterpBMGal-8) > 1e-9 {
		t.Fatalf("L2 在 t=2/3 处插值应为 8，实际 %v", cr.InterpBMGal)
	}
	if math.Abs(cr.DifferenceMGal-(3-8)) > 1e-9 {
		t.Fatalf("交点差应为 -5，实际 %v", cr.DifferenceMGal)
	}
}

// TestCrossingSystematicElevationShift 验收关系：把一条线整体抬高一个高程常量，
// 交点处异常系统性偏移，交点差应等于可预期的非零值并超容限。
func TestCrossingSystematicElevationShift(t *testing.T) {
	shiftH := 50.0
	lines := crossLines([]float64{0, 2, 4}, []float64{3, 3}, testH, testH)
	res0 := mustProcess(t, lines, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 0.5})
	if d := findPair(t, res0, "L1", "L2").Crossings[0].DifferenceMGal; math.Abs(d) > 1e-9 {
		t.Fatalf("抬高前交点差应为 0，实际 %v", d)
	}

	// 把 L1 所有测点整体抬高 50 m（gobs 保持不变 → 异常系统性上移 elevCoef·Δh）。
	shifted := crossLines([]float64{0, 2, 4}, []float64{3, 3}, testH, testH)
	for i := range shifted[0].Points {
		shifted[0].Points[i] = reshapeElevation(shifted[0].Points[i], testH+shiftH)
	}
	res1 := mustProcess(t, shifted, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 0.5})
	cr := findPair(t, res1, "L1", "L2").Crossings[0]
	want := elevCoef * shiftH // (0.3086−0.04193·ρ)·50 ≈ 9.84 mGal
	if math.Abs(cr.DifferenceMGal-want) > 1e-6 {
		t.Fatalf("系统抬高后交点差应为 %v，实际 %v", want, cr.DifferenceMGal)
	}
	if cr.WithinTolerance {
		t.Fatalf("系统偏移量 %v 应超出容限 0.5", cr.DifferenceMGal)
	}
}

// TestParallelLinesNoValidCrossing 验收关系：平行线无论测点怎么摆都报“无有效交点”。
func TestParallelLinesNoValidCrossing(t *testing.T) {
	// 两条东西向直线，y 分别为 0 和 50；同长、错开、一长一短三种摆法都不相交。
	makePair := func(x1a, x1b, x2a, x2b float64) []Line {
		l1 := eastWestLine("L1", 0, []float64{x1a, x1b}, []float64{0, 1})
		l2 := eastWestLine("L2", 50, []float64{x2a, x2b}, []float64{0, 1})
		// eastWestLine 用 x 当里程；重新编号避免跨线 id 相同的困扰（id 仅需线内唯一）。
		l2.Points[0].ID, l2.Points[1].ID = "L2-a", "L2-b"
		l1.Points[0].ID, l1.Points[1].ID = "L1-a", "L1-b"
		return []Line{l1, l2}
	}
	cases := []struct {
		x1a, x1b, x2a, x2b float64
	}{
		{0, 200, 0, 200},   // 完全对齐
		{0, 200, 300, 500}, // 错开
		{0, 400, 100, 200}, // 一长一短
	}
	for i, cs := range cases {
		res := mustProcess(t, makePair(cs.x1a, cs.x1b, cs.x2a, cs.x2b),
			Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
		pair := findPair(t, res, "L1", "L2")
		if pair.Status != PairStatusNoValidCrossing {
			t.Fatalf("case %d 平行线不应有交点，实际 %s", i, pair.Status)
		}
		if pair.Reason != ReasonParallelDistinct {
			t.Fatalf("case %d 原因应为平行不共线，实际 %q", i, pair.Reason)
		}
		if len(pair.Crossings) != 0 {
			t.Fatalf("case %d 无有效交点时 crossings 必须为空", i)
		}
	}
}

// TestSupportingLineIntersectionOutsideRange 支撑线相交但交点落在测量段之外。
func TestSupportingLineIntersectionOutsideRange(t *testing.T) {
	// L1: (0,0)→(200,0)；L2: (300,100)→(400,200)，支撑线交点在 (100,100)？
	// L2 方向 (100,100)，直线参数：(300+t·100, 100+t·100)，令 y=0 → t=−1 → x=200。
	// 即交点 (200,0) 压在 L1 端点、但 L2 上 t=−1 在段外 → 无有效交点。
	l1 := eastWestLine("L1", 0, []float64{0, 200}, []float64{0, 1})
	l2 := lineFromXY("L2", []xy{{300, 100}, {400, 200}}, []float64{0, 1})
	res := mustProcess(t, []Line{l1, l2}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	pair := findPair(t, res, "L1", "L2")
	if pair.Status != PairStatusNoValidCrossing || pair.Reason != ReasonCrossingOutsideSurvey {
		t.Fatalf("应报交点在测量段之外：status=%s reason=%q detail=%s", pair.Status, pair.Reason, pair.ReasonDetail)
	}
}

// TestCollinearLinesNoValidCrossing 共线重叠/共线相离都不产生交点差；端点相接给出唯一交点。
func TestCollinearLinesNoValidCrossing(t *testing.T) {
	th := Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1}

	// 共线重叠（x 区间 [0,200] 与 [100,300]）→ 无唯一交点。
	res := mustProcess(t, []Line{
		eastWestLine("L1", 0, []float64{0, 200}, []float64{0, 1}),
		eastWestLine("L2", 0, []float64{100, 300}, []float64{0, 1}),
	}, th)
	pair := findPair(t, res, "L1", "L2")
	if pair.Status != PairStatusNoValidCrossing || pair.Reason != ReasonCollinear {
		t.Fatalf("共线重叠应无唯一交点：status=%s reason=%q", pair.Status, pair.Reason)
	}

	// 共线相离（[0,100] 与 [200,300]）→ 无交点。
	res = mustProcess(t, []Line{
		eastWestLine("L1", 0, []float64{0, 100}, []float64{0, 1}),
		eastWestLine("L2", 0, []float64{200, 300}, []float64{0, 1}),
	}, th)
	pair = findPair(t, res, "L1", "L2")
	if pair.Status != PairStatusNoValidCrossing || pair.Reason != ReasonCollinear {
		t.Fatalf("共线相离应无交点：status=%s reason=%q", pair.Status, pair.Reason)
	}

	// 端点相接（[0,100] 与 [100,200]）→ 恰有一个唯一交点 (100,0)。
	res = mustProcess(t, []Line{
		eastWestLine("L1", 0, []float64{0, 100}, []float64{2, 5}),
		eastWestLine("L2", 0, []float64{100, 200}, []float64{5, 8}),
	}, th)
	pair = findPair(t, res, "L1", "L2")
	if pair.Status != PairStatusIntersects || len(pair.Crossings) != 1 {
		t.Fatalf("端点相接应有唯一交点：status=%s n=%d", pair.Status, len(pair.Crossings))
	}
	cr := pair.Crossings[0]
	if math.Abs(cr.Easting-100) > 1e-6 || math.Abs(cr.Northing) > 1e-6 {
		t.Fatalf("相接交点应为 (100,0)，实际 (%v,%v)", cr.Easting, cr.Northing)
	}
	// 端点处插值就是端点测点的异常，两线在相接点都为 5 → 交点差 0。
	if math.Abs(cr.InterpAMGal-5) > 1e-9 || math.Abs(cr.InterpBMGal-5) > 1e-9 ||
		math.Abs(cr.DifferenceMGal) > 1e-9 {
		t.Fatalf("相接点两线异常均为 5、差为 0：A=%v B=%v diff=%v",
			cr.InterpAMGal, cr.InterpBMGal, cr.DifferenceMGal)
	}
}

// TestIntersectionAtMeasuredPoint 交点恰好是某条线上的已测点（t=0 端点）。
func TestIntersectionAtMeasuredPoint(t *testing.T) {
	// L2 从 (200,-100) 到 (200,100)，交点 (200,0) 是 L1 的已测点 b（里程200）。
	p1 := []Point{
		mkPoint("L1-a", 0, 0, 0, 0),
		mkPoint("L1-b", 200, 0, 200, 4),
		mkPoint("L1-c", 400, 0, 400, 8),
	}
	p2 := []Point{
		mkPoint("L2-a", 200, -100, 0, 1),
		mkPoint("L2-b", 200, 100, 200, 7),
	}
	res := mustProcess(t, []Line{{ID: "L1", Points: p1}, {ID: "L2", Points: p2}},
		Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 100})
	cr := findPair(t, res, "L1", "L2").Crossings[0]
	if math.Abs(cr.InterpAMGal-4) > 1e-9 || math.Abs(cr.InterpBMGal-4) > 1e-9 {
		t.Fatalf("已测点处插值：L1 应取测点值 4，L2 中点应为 4，实际 A=%v B=%v",
			cr.InterpAMGal, cr.InterpBMGal)
	}
}

// ---- 平移不变性（验收关系）------------------------------------------------

// TestTranslationInvariance 整条线沿东向/北向平移常量：
// 各点布格异常、梯度段诊断、统计画像都不变；两条线一起平移，交点差也不变。
func TestTranslationInvariance(t *testing.T) {
	th := Thresholds{GradientMGalPerM: 0.02, IntersectionTolMGal: 0.5}
	base := []Line{
		eastWestLine("L1", 0, []float64{0, 100, 200, 300}, []float64{0, 1.1, 1.9, 12}), // 末段梯度过大
		{ID: "L2", Points: []Point{ // 斜穿的联络线
			mkPoint("L2-a", 150, -100, 0, 2),
			mkPoint("L2-b", 150, 100, 200, 5),
		}},
	}

	shift := func(lines []Line, de, dn float64) []Line {
		out := make([]Line, len(lines))
		for i, l := range lines {
			out[i] = Line{ID: l.ID, Points: make([]Point, len(l.Points))}
			for j, p := range l.Points {
				p.Easting += de
				p.Northing += dn
				out[i].Points[j] = p
			}
		}
		return out
	}

	r0 := mustProcess(t, base, th)
	for _, off := range []xy{{5000, 0}, {0, 8000}, {12345.678, -9876.543}} {
		rs := mustProcess(t, shift(base, off.e, off.n), th)
		if len(rs.Lines) != len(r0.Lines) {
			t.Fatalf("平移后测线数变了")
		}
		for k := range r0.Lines {
			a, b := r0.Lines[k], rs.Lines[k]
			if len(a.Ordered) != len(b.Ordered) {
				t.Fatalf("平移后 %s 点数变化", a.LineID)
			}
			for i := range a.Ordered {
				av := a.Ordered[i].Result.BouguerAnomalyMGal
				bv := b.Ordered[i].Result.BouguerAnomalyMGal
				if math.Abs(av-bv) > 1e-9 {
					t.Fatalf("平移后点 %s 布格异常改变：%v vs %v", a.Ordered[i].Point.ID, av, bv)
				}
			}
			if len(a.Gradient.Segments) != len(b.Gradient.Segments) {
				t.Fatalf("平移后 %s 梯度段数变化", a.LineID)
			}
			for i := range a.Gradient.Segments {
				s0, s1 := a.Gradient.Segments[i], b.Gradient.Segments[i]
				if math.Abs(s0.GradientMGalPerM-s1.GradientMGalPerM) > 1e-9 ||
					s0.Suspicious != s1.Suspicious {
					t.Fatalf("平移后 %s 段 %d 梯度诊断改变：%+v vs %+v",
						a.LineID, i, s0, s1)
				}
			}
			if math.Abs(a.Stats.MeanMGal-b.Stats.MeanMGal) > 1e-9 ||
				math.Abs(a.Stats.StdMGal-b.Stats.StdMGal) > 1e-9 {
				t.Fatalf("平移后 %s 统计画像改变", a.LineID)
			}
		}
		p0 := findPair(t, r0, "L1", "L2")
		p1 := findPair(t, rs, "L1", "L2")
		if p0.Status != p1.Status || len(p0.Crossings) != len(p1.Crossings) {
			t.Fatalf("平移后交点状态改变：%s vs %s", p0.Status, p1.Status)
		}
		for i := range p0.Crossings {
			if math.Abs(p0.Crossings[i].DifferenceMGal-p1.Crossings[i].DifferenceMGal) > 1e-9 {
				t.Fatalf("平移后交点差改变：%v vs %v",
					p0.Crossings[i].DifferenceMGal, p1.Crossings[i].DifferenceMGal)
			}
		}
	}
}
