package survey

import (
	"math"
	"testing"

	"gravityreduce/internal/gravity"
	"gravityreduce/internal/units"
)

// 测试固定参数：中纬度、常用岩性密度；阈值取勘探上常见的 mGal/km 量级。
const (
	testPhi = 45.0
	testRho = 2.67
	gradThr = 30.0 // mGal/km
	intTol  = 0.5  // mGal
	tolNum  = 1e-9
)

// bouguerAt 独立复算给定测点的布格异常（mGal），用于交叉验证服务输出确实
// 来自真实单点归算，而非占位值。
func bouguerAt(p PointInput) float64 {
	return gravity.Reduce(gravity.Observation{
		GobsMS2: p.GobsMS2, H: p.H, Phi: p.Phi, Rho: p.Rho,
	}).BouguerAnomalyMGal
}

// mkPoint 构造一个“想要的布格异常”可精确指定的测点：
// 反解 gobs 使该点经真实归算后恰得 targetAnomalyMGal。
// 所有点同纬度、同密度，异常差异只来自目标值与高程。
func mkPoint(id string, station, e, n, h, targetAnomalyMGal float64) PointInput {
	gammaMGal := units.MS2ToMGal(gravity.NormalGravity(testPhi))
	fa := gravity.FreeAirCorrection(h)
	b := gravity.BouguerSlabCorrection(testRho, h)
	// Δg_B = gobs − γ + FA − B  ⇒  gobs = Δg_B + γ − FA + B
	gobsMGal := targetAnomalyMGal + gammaMGal - fa + b
	return PointInput{
		ID: id, GobsMS2: units.MGalToMS2(gobsMGal), H: h, Phi: testPhi, Rho: testRho,
		EastingM: e, NorthingM: n, StationM: station, HasStation: true,
	}
}

// withHeight 保持观测量不变、只改高程（模拟“把某个点高程调离邻点”）。
func withHeight(p PointInput, h float64) PointInput {
	p.H = h
	return p
}

// withShift 整体平移平面坐标，观测物理量一概不动。
func withShift(p PointInput, dE, dN float64) PointInput {
	p.EastingM += dE
	p.NorthingM += dN
	return p
}

func lineShifted(l LineInput, dE, dN float64) LineInput {
	out := LineInput{ID: l.ID, Points: make([]PointInput, len(l.Points))}
	for i, p := range l.Points {
		out.Points[i] = withShift(p, dE, dN)
	}
	return out
}

func stdReq(lines ...LineInput) Request {
	return Request{
		Lines:                      lines,
		GradientThresholdMGalPerKm: gradThr,
		IntersectionToleranceMGal:  intTol,
	}
}

// horizLine 东西向直线 y=0，异常沿里程线性（每 100 m 增 anomalyStep mGal）。
func horizLine(id string, xs, ys, h, anomalyStep float64) LineInput {
	stations := []float64{0, 100, 200}
	anoms := []float64{ys, ys + anomalyStep, ys + 2*anomalyStep}
	pts := make([]PointInput, 3)
	for i := range stations {
		pts[i] = mkPoint(id+"-"+itoaS(i), stations[i], xs+stations[i], 0, h, anoms[i])
	}
	return LineInput{ID: id, Points: pts}
}

func itoaS(i int) string {
	return string(rune('0' + i))
}

// findPair 在报告中按两条线 ID 找配对结果。
func findPair(rep Report, a, b string) PairReport {
	for _, p := range rep.Pairs {
		if p.LineAID == a && p.LineBID == b || p.LineAID == b && p.LineBID == a {
			return p
		}
	}
	return PairReport{}
}

func findLine(rep Report, id string) LineReport {
	for _, l := range rep.Lines {
		if l.ID == id {
			return l
		}
	}
	return LineReport{}
}

// ---- 基础：逐点归算确实复用真实单点链条，且按里程理顺次序 ----

func TestPointReductionsAreRealAndOrdered(t *testing.T) {
	// 故意乱序送入：里程 200 的点排在最前。
	l := LineInput{ID: "L", Points: []PointInput{
		mkPoint("p2", 200, 200, 0, 100, 14),
		mkPoint("p0", 0, 0, 0, 100, 10),
		mkPoint("p1", 100, 100, 0, 100, 12),
	}}
	rep := Process(stdReq(l))
	lr := findLine(rep, "L")
	if lr.Status != StatusOK {
		t.Fatalf("测线应为 ok，实际 %s reasons=%v", lr.Status, lr.DegradeReasons)
	}
	gotIDs := []string{lr.Points[0].ID, lr.Points[1].ID, lr.Points[2].ID}
	wantIDs := []string{"p0", "p1", "p2"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("测点未按里程理顺：%v，期望 %v", gotIDs, wantIDs)
		}
	}
	// 每个点的异常必须等于对该点单独做一次 gravity.Reduce 的结果（按 ID 对应，不依赖输入次序）。
	byID := map[string]PointInput{}
	for _, p := range l.Points {
		byID[p.ID] = p
	}
	for _, vp := range lr.Points {
		want := bouguerAt(byID[vp.ID])
		if math.Abs(vp.BouguerAnomalyMGal-want) > tolNum {
			t.Fatalf("点 %s 异常未走真实归算：got %.10f want %.10f", vp.ID, vp.BouguerAnomalyMGal, want)
		}
	}
}

// ---- 验收关系 1：整体平面平移不变性 ----
// 把一条测线整体沿东向、北向平移常量，各点布格异常与线内梯度诊断都不应改变
// （归算只取决于纬度、高程、密度，与平面位置无关）。
func TestTranslationInvariance(t *testing.T) {
	base := horizLine("L", 0, 10, 100, 2.0) // 20 mGal/km 的正常梯度，低于阈值 30
	moved := lineShifted(base, 1234.5, -678.9)
	moved.ID = "L2"
	req := stdReq(base, moved)
	rep := Process(req)
	if len(rep.Pairs) != 1 {
		t.Fatalf("两条线应产生一个配对，实际 %d", len(rep.Pairs))
	}

	a := findLine(rep, "L")
	b := findLine(rep, "L2")
	if len(a.Points) != len(b.Points) {
		t.Fatal("平移不应改变点数")
	}
	for i := range a.Points {
		if math.Abs(a.Points[i].BouguerAnomalyMGal-b.Points[i].BouguerAnomalyMGal) > tolNum {
			t.Fatalf("平移后第 %d 点布格异常改变：%v vs %v", i, a.Points[i].BouguerAnomalyMGal, b.Points[i].BouguerAnomalyMGal)
		}
		if a.Points[i].ID != b.Points[i].ID {
			t.Fatalf("平移不应改变点序：%s vs %s", a.Points[i].ID, b.Points[i].ID)
		}
	}
	if a.Gradient.SuspiciousCount != b.Gradient.SuspiciousCount ||
		a.Gradient.CoincidentCount != b.Gradient.CoincidentCount ||
		len(a.Gradient.Segments) != len(b.Gradient.Segments) {
		t.Fatalf("平移后梯度诊断结构改变：%+v vs %+v", a.Gradient, b.Gradient)
	}
	for i := range a.Gradient.Segments {
		sa, sb := a.Gradient.Segments[i], b.Gradient.Segments[i]
		if math.Abs(sa.GradientMGalPerKm-sb.GradientMGalPerKm) > tolNum ||
			sa.Suspicious != sb.Suspicious ||
			math.Abs(sa.ExcessMGalPerKm-sb.ExcessMGalPerKm) > tolNum ||
			math.Abs(sa.HorizontalDistanceM-sb.HorizontalDistanceM) > tolNum {
			t.Fatalf("平移后第 %d 段梯度诊断改变：%+v vs %+v", i, sa, sb)
		}
	}
}

// ---- 验收关系 2：中部居中插入与线性趋势一致的点，不新增可疑段 ----
func TestMidpointInsertionNoNewSuspicion(t *testing.T) {
	mk := func(ids []string, stations, anoms []float64) LineInput {
		pts := make([]PointInput, len(stations))
		for i := range stations {
			pts[i] = mkPoint(ids[i], stations[i], stations[i], 0, 0, anoms[i])
		}
		return LineInput{ID: "L", Points: pts}
	}

	three := mk([]string{"p0", "p2", "p4"},
		[]float64{0, 100, 200}, []float64{10, 12, 14})
	five := mk([]string{"p0", "p1", "p2", "p3", "p4"},
		[]float64{0, 50, 100, 150, 200}, []float64{10, 11, 12, 13, 14})

	r3 := Process(stdReq(three))
	r5 := Process(stdReq(five))
	g3 := findLine(r3, "L").Gradient
	g5 := findLine(r5, "L").Gradient
	if g3.SuspiciousCount != 0 {
		t.Fatalf("前提错误：原线不应有可疑段，实际 %d", g3.SuspiciousCount)
	}
	if g5.SuspiciousCount != 0 {
		t.Fatalf("居中插入与线性趋势一致的点不应新增可疑段，实际 %d 个", g5.SuspiciousCount)
	}
	if len(g5.Segments) != 4 {
		t.Fatalf("插入后应有 4 段，实际 %d", len(g5.Segments))
	}
	for i, s := range g5.Segments {
		if math.Abs(s.GradientMGalPerKm-20.0) > 1e-7 {
			t.Fatalf("第 %d 段梯度应为 20 mGal/km，实际 %v", i, s.GradientMGalPerKm)
		}
		if s.Kind != SegmentNormal || s.HorizontalDistanceM != 50 {
			t.Fatalf("第 %d 段应为 50 m 正常段，实际 kind=%s dist=%v", i, s.Kind, s.HorizontalDistanceM)
		}
	}
}

// ---- 验收关系 3：把某点高程明显调离邻点，应在其两侧都触发梯度告警 ----
func TestHeightOutlierTriggersBothSides(t *testing.T) {
	l := LineInput{ID: "L", Points: []PointInput{
		mkPoint("p0", 0, 0, 0, 100, 10),
		withHeight(mkPoint("p1", 100, 100, 0, 100, 10), 400), // 观测量不变，高程 100→400
		mkPoint("p2", 200, 200, 0, 100, 10),
	}}
	rep := Process(stdReq(l))
	g := findLine(rep, "L").Gradient
	if g.SuspiciousCount != 2 {
		t.Fatalf("异常点两侧应各触发一个可疑段，实际 %d", g.SuspiciousCount)
	}
	// 独立预期：高程抬高 Δh 使布格异常增大 (0.3086−0.04193ρ)·Δh。
	delta := (gravity.FreeAirGradient - gravity.BouguerSlabCoefficient*testRho) * 300
	wantGrad := delta / 0.1 // mGal/km（段长 100 m = 0.1 km）
	for i, s := range g.Segments {
		if !s.Suspicious {
			t.Fatalf("第 %d 段应可疑", i)
		}
		if math.Abs(math.Abs(s.GradientMGalPerKm)-wantGrad) > 1e-6 {
			t.Fatalf("第 %d 段梯度应为 %.6f，实际 %v", i, wantGrad, s.GradientMGalPerKm)
		}
		if math.Abs(s.ExcessMGalPerKm-(math.Abs(s.GradientMGalPerKm)-gradThr)) > 1e-6 {
			t.Fatalf("第 %d 段超出量不符：%v", i, s.ExcessMGalPerKm)
		}
	}
	// 两段应一正一负：先抬升、再回落。
	if g.Segments[0].GradientMGalPerKm*g.Segments[1].GradientMGalPerKm >= 0 {
		t.Fatal("异常点两侧梯度符号应相反")
	}
}

// ---- 退化输入：同位置重复布点（零距离），报明确数据异常而不是除零崩溃 ----
func TestZeroDistanceReportedAsAnomaly(t *testing.T) {
	l := LineInput{ID: "L", Points: []PointInput{
		mkPoint("p0", 0, 100, 100, 100, 10),
		mkPoint("p1", 100, 100, 100, 100, 13), // 与 p0 平面重合
		mkPoint("p2", 200, 200, 100, 100, 12),
	}}
	rep := Process(stdReq(l))
	g := findLine(rep, "L").Gradient
	if g.CoincidentCount != 1 {
		t.Fatalf("应有 1 个零距离退化段，实际 %d", g.CoincidentCount)
	}
	if g.SuspiciousCount != 0 {
		t.Fatalf("零距离段不应计入梯度可疑段，实际 %d", g.SuspiciousCount)
	}
	var coin *SegmentResult
	for i := range g.Segments {
		if g.Segments[i].FromID == "p0" && g.Segments[i].ToID == "p1" {
			coin = &g.Segments[i]
		}
	}
	if coin == nil {
		t.Fatal("未找到 p0→p1 段")
	}
	if coin.Kind != SegmentCoincident {
		t.Fatalf("该段应为 coincident，实际 %s", coin.Kind)
	}
	if math.IsInf(coin.GradientMGalPerKm, 0) || math.IsNaN(coin.GradientMGalPerKm) {
		t.Fatal("零距离段梯度绝不能是 Inf/NaN")
	}
	if coin.GradientMGalPerKm != 0 {
		t.Fatalf("零距离段梯度占位应为 0，实际 %v", coin.GradientMGalPerKm)
	}
	// 其余正常段照常诊断。
	if len(g.Segments) != 2 {
		t.Fatalf("应有 2 段，实际 %d", len(g.Segments))
	}
}

// ---- 线内统计画像：必须由真实归算结果计算 ----
func TestLineStatistics(t *testing.T) {
	l := LineInput{ID: "L", Points: []PointInput{
		mkPoint("p0", 0, 0, 0, 100, 10),
		mkPoint("p1", 100, 100, 0, 100, 12),
		mkPoint("p2", 200, 200, 0, 100, 14),
	}}
	rep := Process(stdReq(l))
	st := findLine(rep, "L").Statistics
	if st.PointCount != 3 {
		t.Fatalf("点数应为 3，实际 %d", st.PointCount)
	}
	if math.Abs(st.MeanMGal-12) > 1e-9 {
		t.Fatalf("均值应为 12，实际 %v", st.MeanMGal)
	}
	// 样本标准差（n−1）：sqrt(((4+0+4)/2)) = 2。
	if math.Abs(st.StdMGal-2) > 1e-9 {
		t.Fatalf("样本标准差应为 2，实际 %v", st.StdMGal)
	}
	if st.Min.PointID != "p0" || math.Abs(st.Min.ValueMGal-10) > 1e-9 {
		t.Fatalf("最小值定位错误：%+v", st.Min)
	}
	if st.Max.PointID != "p2" || math.Abs(st.Max.ValueMGal-14) > 1e-9 {
		t.Fatalf("最大值定位错误：%+v", st.Max)
	}
}

// ---- 交点几何：两条正交测线，交点处异常相等 → 交点差为 0 ----
// 每条线取两个测点，使几何交点 (100,0) 落在两侧测点段的正中（t=0.5），
// 真正考验“交点落在测点中间时的线性插值”。
func crossingLines() (LineInput, LineInput) {
	a := LineInput{ID: "A", Points: []PointInput{
		mkPoint("A0", 0, 0, 0, 100, 20),
		mkPoint("A2", 200, 200, 0, 100, 20),
	}}
	b := LineInput{ID: "B", Points: []PointInput{
		mkPoint("B0", 0, 100, -100, 100, 20),
		mkPoint("B2", 200, 100, 100, 100, 20),
	}}
	return a, b
}

func TestIntersectionEqualAnomalyZeroDifference(t *testing.T) {
	a, b := crossingLines()
	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusIntersected {
		t.Fatalf("应有有效交点，实际 %s reason=%v", p.Status, p.Reason)
	}
	if math.Abs(p.EastingM-100) > 1e-9 || math.Abs(p.NorthingM-0) > 1e-9 {
		t.Fatalf("交点坐标错误：(%v,%v)", p.EastingM, p.NorthingM)
	}
	if p.A == nil || p.B == nil {
		t.Fatal("两侧插值结果必须给出")
	}
	if math.Abs(p.A.ParameterT-0.5) > 1e-9 || math.Abs(p.B.ParameterT-0.5) > 1e-9 {
		t.Fatalf("交点应位于两侧中段正中，t=(%v,%v)", p.A.ParameterT, p.B.ParameterT)
	}
	if math.Abs(p.A.AnomalyMGal-20) > 1e-9 || math.Abs(p.B.AnomalyMGal-20) > 1e-9 {
		t.Fatalf("两侧插值应为 20，实际 %v / %v", p.A.AnomalyMGal, p.B.AnomalyMGal)
	}
	if math.Abs(p.DifferenceMGal) > 1e-9 {
		t.Fatalf("等值交点差应为 0，实际 %v", p.DifferenceMGal)
	}
	if p.ExceedsTolerance {
		t.Fatal("零交点差不应超容差")
	}
}

// ---- 交点几何：一条线整体抬高常量高程 → 交点差呈可预期的非零系统偏移 ----
func TestIntersectionSystematicHeightShift(t *testing.T) {
	a, b0 := crossingLines()
	// 把 B 整体抬高 100 m（观测量保持不变）。
	b := LineInput{ID: "B", Points: make([]PointInput, len(b0.Points))}
	for i, p := range b0.Points {
		b.Points[i] = withHeight(p, p.H+100)
	}

	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusIntersected {
		t.Fatalf("应有有效交点，实际 %s %v", p.Status, p.Reason)
	}
	// 抬高 Δh 使 B 各点异常系统性增大 c·Δh，c = 0.3086 − 0.04193ρ。
	shift := (gravity.FreeAirGradient - gravity.BouguerSlabCoefficient*testRho) * 100
	wantDiff := -shift // A − B
	if math.Abs(p.DifferenceMGal-wantDiff) > 1e-7 {
		t.Fatalf("系统偏移下交点差应为 %.6f，实际 %.6f", wantDiff, p.DifferenceMGal)
	}
	if !p.ExceedsTolerance {
		t.Fatalf("偏移 %.4f mGal 超过容差 %.2f，应标记超差", shift, intTol)
	}
	// 插值仍应取在两侧中段正中。
	if math.Abs(p.A.ParameterT-0.5) > 1e-9 || math.Abs(p.B.ParameterT-0.5) > 1e-9 {
		t.Fatalf("插值段位置错误：t=(%v,%v)", p.A.ParameterT, p.B.ParameterT)
	}
	if math.Abs(p.A.AnomalyMGal-20) > 1e-9 {
		t.Fatalf("A 侧插值应仍为 20，实际 %v", p.A.AnomalyMGal)
	}
	if math.Abs(p.B.AnomalyMGal-(20+shift)) > 1e-7 {
		t.Fatalf("B 侧插值应为 %.6f，实际 %v", 20+shift, p.B.AnomalyMGal)
	}
}

// ---- 验收关系 4：平行线无交点（无论测点怎么摆） ----
func TestParallelLinesNoIntersection(t *testing.T) {
	a := LineInput{ID: "A", Points: []PointInput{
		mkPoint("A0", 0, 0, 0, 100, 20),
		mkPoint("A1", 100, 100, 0, 100, 20),
		mkPoint("A2", 200, 200, 0, 100, 20),
	}}
	b := LineInput{ID: "B", Points: []PointInput{
		mkPoint("B0", 0, 0, 50, 100, 20),
		mkPoint("B1", 100, 100, 50, 100, 20),
		mkPoint("B2", 200, 200, 50, 100, 20),
	}}
	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusNoIntersection || p.Reason.Code != ReasonParallel {
		t.Fatalf("平行线应报无有效交点(parallel)，实际 status=%s reason=%v", p.Status, p.Reason)
	}
	if p.A != nil || p.B != nil || p.DifferenceMGal != 0 {
		t.Fatal("无交点时绝不能硬凑插值与交点差")
	}
}

// 共线：交点不唯一，不能凑数。
func TestCollinearLinesNoIntersection(t *testing.T) {
	a := LineInput{ID: "A", Points: []PointInput{
		mkPoint("A0", 0, 0, 0, 100, 20),
		mkPoint("A1", 100, 100, 0, 100, 20),
	}}
	b := LineInput{ID: "B", Points: []PointInput{
		mkPoint("B0", 0, -100, 0, 100, 20),
		mkPoint("B1", 100, 0, 0, 100, 20),
	}}
	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusNoIntersection || p.Reason.Code != ReasonCollinear {
		t.Fatalf("共线应报无有效交点(collinear)，实际 status=%s reason=%v", p.Status, p.Reason)
	}
}

// 延长线相交但交点落在实测段之外：无有效交点，如实说明。
func TestIntersectionOutOfRange(t *testing.T) {
	a := LineInput{ID: "A", Points: []PointInput{
		mkPoint("A0", 0, 0, 0, 100, 20),
		mkPoint("A1", 100, 100, 0, 100, 20),
	}}
	// B 为 x=200 的南北向短测线，与 A 的延长线交于 (200,0)，在 A 实测段之外。
	b := LineInput{ID: "B", Points: []PointInput{
		mkPoint("B0", 0, 200, -50, 100, 20),
		mkPoint("B1", 100, 200, 50, 100, 20),
	}}
	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusNoIntersection || p.Reason.Code != ReasonOutOfRange {
		t.Fatalf("应报交点越界，实际 status=%s reason=%v", p.Status, p.Reason)
	}
}

// 交点恰好落在已测点上：直接取该点值（t=0/1），交点差仍为 0。
func TestIntersectionAtStationPoint(t *testing.T) {
	a := LineInput{ID: "A", Points: []PointInput{
		mkPoint("A0", 0, 0, 0, 100, 20),
		mkPoint("A1", 100, 100, 0, 100, 20),
		mkPoint("A2", 200, 200, 0, 100, 20),
	}}
	// B 的端点 B2 正好压在 A 的测点 A2=(200,0) 上。
	b := LineInput{ID: "B", Points: []PointInput{
		mkPoint("B0", 0, 200, -100, 100, 20),
		mkPoint("B1", 100, 200, -50, 100, 20),
		mkPoint("B2", 200, 200, 0, 100, 20),
	}}
	rep := Process(stdReq(a, b))
	p := findPair(rep, "A", "B")
	if p.Status != StatusIntersected {
		t.Fatalf("端点相交应为有效交点，实际 %s %v", p.Status, p.Reason)
	}
	if math.Abs(p.EastingM-200) > 1e-9 || math.Abs(p.NorthingM) > 1e-9 {
		t.Fatalf("交点应为 (200,0)，实际 (%v,%v)", p.EastingM, p.NorthingM)
	}
	if math.Abs(p.DifferenceMGal) > 1e-9 {
		t.Fatalf("等值端点交点差应为 0，实际 %v", p.DifferenceMGal)
	}
}

// ---- 降级输入：单点测线 ----
func TestSinglePointLineDegrades(t *testing.T) {
	single := LineInput{ID: "S", Points: []PointInput{mkPoint("s0", 0, 0, 0, 100, 11)}}
	other := LineInput{ID: "O", Points: []PointInput{
		mkPoint("O0", 0, 50, -50, 100, 11),
		mkPoint("O1", 100, 50, 50, 100, 11),
	}}
	rep := Process(stdReq(single, other))
	sl := findLine(rep, "S")
	if sl.Status != StatusDegraded {
		t.Fatalf("单点线应降级，实际 %s", sl.Status)
	}
	if len(sl.DegradeReasons) == 0 || sl.DegradeReasons[0].Code != ReasonSinglePoint {
		t.Fatalf("降级原因应为 single_point，实际 %v", sl.DegradeReasons)
	}
	if sl.Gradient.Status != "skipped" {
		t.Fatalf("梯度诊断应跳过，实际 %s", sl.Gradient.Status)
	}
	// 归算与统计仍要真实给出。
	if sl.Statistics.PointCount != 1 || math.Abs(sl.Statistics.MeanMGal-11) > 1e-9 {
		t.Fatalf("单点统计画像错误：%+v", sl.Statistics)
	}
	if math.Abs(sl.Points[0].BouguerAnomalyMGal-bouguerAt(single.Points[0])) > tolNum {
		t.Fatal("单点仍必须走真实归算")
	}
	// 交点诊断不能硬撑。
	p := findPair(rep, "S", "O")
	if p.Status != StatusNoIntersection || p.Reason.Code != ReasonLineDegraded {
		t.Fatalf("单点线参与配对应报降级无交点，实际 %s %v", p.Status, p.Reason)
	}
}

// ---- 降级输入：里程无法定序 ----
func TestMissingStationDegrades(t *testing.T) {
	p0 := mkPoint("p0", 0, 0, 0, 100, 10)
	p1 := mkPoint("p1", 100, 100, 0, 100, 12)
	p2 := mkPoint("p2", 200, 200, 0, 100, 14)
	p1.HasStation = false // 中间点缺里程
	l := LineInput{ID: "L", Points: []PointInput{p0, p1, p2}}

	rep := Process(stdReq(l))
	lr := findLine(rep, "L")
	if lr.Status != StatusDegraded {
		t.Fatalf("缺里程应降级，实际 %s", lr.Status)
	}
	if lr.DegradeReasons[0].Code != ReasonStationMissing {
		t.Fatalf("降级原因应为 station_missing，实际 %v", lr.DegradeReasons)
	}
	if lr.Gradient.Status != "skipped" || len(lr.Gradient.Segments) != 0 {
		t.Fatalf("梯度诊断应整体跳过，实际 %+v", lr.Gradient)
	}
	// 统计不依赖次序，仍真实给出。
	if lr.Statistics.PointCount != 3 || math.Abs(lr.Statistics.MeanMGal-12) > 1e-9 {
		t.Fatalf("缺里程时统计仍应有效，实际 %+v", lr.Statistics)
	}
}

// 重复里程：次序不唯一，同样降级。
func TestDuplicateStationDegrades(t *testing.T) {
	l := LineInput{ID: "L", Points: []PointInput{
		mkPoint("p0", 0, 0, 0, 100, 10),
		mkPoint("p1", 100, 100, 0, 100, 12),
		mkPoint("p2", 100, 200, 0, 100, 14), // 里程与 p1 相同
	}}
	rep := Process(stdReq(l))
	lr := findLine(rep, "L")
	if lr.Status != StatusDegraded || lr.DegradeReasons[0].Code != ReasonStationDuplicate {
		t.Fatalf("重复里程应降级 station_duplicate，实际 %s %v", lr.Status, lr.DegradeReasons)
	}
}

// ---- 请求级校验：无意义输入必须带原因拒绝 ----

func TestValidateRequestRejects(t *testing.T) {
	good := func() Request { return stdReq(horizLine("L", 0, 10, 100, 1)) }

	cases := []struct {
		name   string
		mutate func(Request) Request
		field  string
	}{
		{"无线", func(r Request) Request { r.Lines = nil; return r }, "lines"},
		{"阈值非正", func(r Request) Request { r.GradientThresholdMGalPerKm = 0; return r }, "gradient_threshold_mgal_per_km"},
		{"容差为负", func(r Request) Request { r.IntersectionToleranceMGal = -1; return r }, "intersection_tolerance_mgal"},
		{"线ID重复", func(r Request) Request {
			r.Lines = append(r.Lines, r.Lines[0])
			return r
		}, "lines[1].id"},
		{"点ID重复", func(r Request) Request {
			r.Lines[0].Points[2].ID = r.Lines[0].Points[0].ID
			return r
		}, "points[2].id"},
		{"坐标为NaN", func(r Request) Request {
			r.Lines[0].Points[0].EastingM = math.NaN()
			return r
		}, "easting_m"},
		{"物理量非法(密度0)", func(r Request) Request {
			r.Lines[0].Points[0].Rho = 0
			return r
		}, "rho"},
		{"里程为Inf", func(r Request) Request {
			r.Lines[0].Points[0].StationM = math.Inf(1)
			return r
		}, "station_m"},
		{"空测线", func(r Request) Request {
			r.Lines[0].Points = nil
			return r
		}, "points"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequest(tc.mutate(good()))
			if err == nil {
				t.Fatalf("%s：应被拒绝", tc.name)
			}
			msg := err.Error()
			if !contains(msg, tc.field) {
				t.Fatalf("%s：错误应定位到字段含 %q，实际 %v", tc.name, tc.field, err)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// 三条线：两两配对全部产生，顺序按 i<j。
func TestAllPairsGenerated(t *testing.T) {
	a, b := crossingLines()
	c := LineInput{ID: "C", Points: []PointInput{
		mkPoint("C0", 0, 50, -200, 100, 20),
		mkPoint("C2", 200, 50, 200, 100, 20),
	}}
	rep := Process(stdReq(a, b, c))
	if len(rep.Pairs) != 3 {
		t.Fatalf("三条线应有 3 个配对，实际 %d", len(rep.Pairs))
	}
	// A×C 的交点 (50,0) 也在两段之内。
	ac := findPair(rep, "A", "C")
	if ac.Status != StatusIntersected {
		t.Fatalf("A×C 应相交，实际 %s %v", ac.Status, ac.Reason)
	}
	if math.Abs(ac.EastingM-50) > 1e-9 {
		t.Fatalf("A×C 交点 x 应为 50，实际 %v", ac.EastingM)
	}
}
