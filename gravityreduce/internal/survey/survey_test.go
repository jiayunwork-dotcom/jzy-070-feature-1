package survey

import (
	"math"
	"testing"

	"gravityreduce/internal/gravity"
)

// ---- 测试数据构造辅助 ----------------------------------------------------
//
// 固定纬度/密度，通过给每个测点指定“期望布格异常”反推 gobs：
//
//	Δg_B = gobs_mgal − γ(phi) + (0.3086 − 0.04193·rho)·h
//
// 因此所有布格异常都仍由真实单点归算链条算出（非占位），
// 测试只是用反推的 gobs 把异常场构造成想要的形状。
const (
	testPhi = 45.0
	testRho = 2.67
	testH   = 100.0
)

var elevCoef = gravity.FreeAirGradient - gravity.BouguerSlabCoefficient*testRho // mGal/m

// mkPoint 构造一个测点：指定平面位置、里程与“期望布格异常”（mGal）。
func mkPoint(id string, e, n, station, anomalyMGal float64) Point {
	return mkPointH(id, e, n, station, anomalyMGal, testH)
}

// mkPointH 同 mkPoint，但允许指定高程（用于高程调离/系统抬高试验）。
func mkPointH(id string, e, n, station, anomalyMGal, h float64) Point {
	gammaMGal := gravity.NormalGravity(testPhi) * 1e5
	gobsMGal := anomalyMGal + gammaMGal - elevCoef*h
	return Point{
		ID:       id,
		Easting:  e,
		Northing: n,
		Station:  &station,
		Obs:      gravity.Observation{GobsMS2: gobsMGal * 1e-5, H: h, Phi: testPhi, Rho: testRho},
	}
}

// reshapeElevation 复制一个测点并把高程改为 h；gobs 保持原值不变。
// 这模拟野外真实的高程调离/整体抬高：观测重力没动，只是高程变了，
// 因而归算出的布格异常会系统性上移 elevCoef·(h−h0)，与 reshape 异常场的 mkPointH 不同。
func reshapeElevation(p Point, h float64) Point {
	p.Obs.H = h
	return p
}

func ptr(x float64) *float64 { return &x }

// eastWestLine 构造一条东西向直线测线：y 固定为 northing，x=station。
func eastWestLine(id string, northing float64, stations []float64, anomalies []float64) Line {
	pts := make([]Point, len(stations))
	for i, s := range stations {
		pts[i] = mkPoint(id+"-"+itoa(i), s, northing, s, anomalies[i])
	}
	return Line{ID: id, Points: pts}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

// lineByAnomaly 按 (里程, 异常) 与坐标函数构造一条线。
type xy struct{ e, n float64 }

func lineFromXY(id string, coords []xy, anomalies []float64) Line {
	pts := make([]Point, len(coords))
	for i, c := range coords {
		pts[i] = mkPoint(id+"-"+itoa(i), c.e, c.n, float64(i)*100, anomalies[i])
	}
	return Line{ID: id, Points: pts}
}

func mustProcess(t *testing.T, lines []Line, th Thresholds) BatchResult {
	t.Helper()
	if err := Validate(lines, th); err != nil {
		t.Fatalf("输入应通过校验，实际被拒：%v", err)
	}
	return Process(lines, th)
}

func findLine(t *testing.T, res BatchResult, id string) LineResult {
	t.Helper()
	for _, l := range res.Lines {
		if l.LineID == id {
			return l
		}
	}
	t.Fatalf("结果中找不到测线 %q", id)
	return LineResult{}
}

func findPair(t *testing.T, res BatchResult, a, b string) PairResult {
	t.Helper()
	for _, p := range res.Pairs {
		if p.LineAID == a && p.LineBID == b {
			return p
		}
	}
	t.Fatalf("结果中找不到线对 (%q,%q)", a, b)
	return PairResult{}
}

// ---- 定序 ----------------------------------------------------------------

func TestOrderingShuffledInputSortedByStation(t *testing.T) {
	// 故意乱序送入；平面坐标各不相同，证明排序只认里程而非送入次序。
	line := Line{ID: "L", Points: []Point{
		mkPoint("p2", 200, 0, 200, 2),
		mkPoint("p0", 0, 0, 0, 0),
		mkPoint("p3", 300, 0, 300, 3),
		mkPoint("p1", 100, 0, 100, 1),
	}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	if lr.OrderingBasis != "station_m" || lr.Status != StatusOK {
		t.Fatalf("应按里程正常定序：basis=%q status=%q notes=%v", lr.OrderingBasis, lr.Status, lr.Notes)
	}
	wantOrder := []string{"p0", "p1", "p2", "p3"}
	for i, want := range wantOrder {
		if lr.Ordered[i].Point.ID != want {
			t.Fatalf("排序后第 %d 个点应为 %s，实际 %s", i, want, lr.Ordered[i].Point.ID)
		}
	}
}

// ---- 沿测线梯度 -----------------------------------------------------------

func TestGradientLinearTrendNoSuspicious(t *testing.T) {
	// 每 100 m 异常增 1 mGal，梯度 0.01 mGal/m；阈值 0.05，不应告警。
	line := eastWestLine("L", 0, []float64{0, 100, 200, 300}, []float64{0, 1, 2, 3})
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 0.05, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	if !lr.Gradient.Available || len(lr.Gradient.Segments) != 3 {
		t.Fatalf("应有 3 个诊断段，实际 %d", len(lr.Gradient.Segments))
	}
	for i, s := range lr.Gradient.Segments {
		if math.Abs(s.GradientMGalPerM-0.01) > 1e-9 {
			t.Fatalf("段 %d 梯度应为 0.01，实际 %v", i, s.GradientMGalPerM)
		}
		if s.Suspicious {
			t.Fatalf("线性趋势段 %d 不应告警，超阈量 %v", i, s.ExceedsByMGalPerM)
		}
	}
}

// TestMidpointInsertionNoAlert 验收关系：在直线段中部插入里程居中、
// 观测量符合线性趋势的新测点，不应新增可疑段。
func TestMidpointInsertionNoAlert(t *testing.T) {
	base := eastWestLine("L", 0, []float64{0, 200}, []float64{0, 2})
	th := Thresholds{GradientMGalPerM: 0.05, IntersectionTolMGal: 1}
	res0 := mustProcess(t, []Line{base}, th)
	lr0 := findLine(t, res0, "L")
	for _, s := range lr0.Gradient.Segments {
		if s.Suspicious {
			t.Fatalf("原始测线不应有可疑段")
		}
	}

	// 在 (100, 0)、里程 100 处插入，期望异常恰为线性中点 1 mGal。
	withMid := Line{ID: "L", Points: []Point{
		mkPoint("a", 0, 0, 0, 0),
		mkPoint("mid", 100, 0, 100, 1),
		mkPoint("b", 200, 0, 200, 2),
	}}
	res1 := mustProcess(t, []Line{withMid}, th)
	lr1 := findLine(t, res1, "L")
	if len(lr1.Gradient.Segments) != 2 {
		t.Fatalf("插入后应有 2 段，实际 %d", len(lr1.Gradient.Segments))
	}
	for i, s := range lr1.Gradient.Segments {
		if s.Suspicious {
			t.Fatalf("居中插点后段 %d 不应告警（梯度 %v，超阈 %v）", i, s.GradientMGalPerM, s.ExceedsByMGalPerM)
		}
	}
}

// TestElevationSpikeAlertsBothSides 验收关系：把某点高程明显调离邻点，
// 应在它的左右两侧各触发一个梯度告警。
func TestElevationSpikeAlertsBothSides(t *testing.T) {
	// 三个点本来异常同为 0；把中间点高程抬高 100 m（gobs 保持不变），
	// 其异常上移 (0.3086−0.04193·2.67)·100 ≈ 19.68 mGal。
	p0 := mkPoint("a", 0, 0, 0, 0)
	p1 := reshapeElevation(mkPoint("spike", 100, 0, 100, 0), testH+100)
	p2 := mkPoint("b", 200, 0, 200, 0)
	line := Line{ID: "L", Points: []Point{p0, p1, p2}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 0.05, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	if len(lr.Gradient.Segments) != 2 {
		t.Fatalf("应有 2 段，实际 %d", len(lr.Gradient.Segments))
	}
	for i, s := range lr.Gradient.Segments {
		if !s.Suspicious {
			t.Fatalf("抬高段两侧段 %d 都应告警，梯度 %v", i, s.GradientMGalPerM)
		}
		if s.ExceedsByMGalPerM <= 0 {
			t.Fatalf("超阈量应为正，实际 %v", s.ExceedsByMGalPerM)
		}
	}
	// 左段梯度为正、右段梯度为负，且绝对值一致。
	g0 := lr.Gradient.Segments[0].GradientMGalPerM
	g1 := lr.Gradient.Segments[1].GradientMGalPerM
	if !(g0 > 0 && g1 < 0 && math.Abs(g0+g1) < 1e-9) {
		t.Fatalf("抬高点两侧梯度应符号相反、绝对值相等：%v %v", g0, g1)
	}
}

// TestZeroDistanceReportedNotInfinity 验收关系：同位置重复布点距离为 0，
// 必须作为明确数据异常报出，不能算出无穷大梯度或崩溃。
func TestZeroDistanceReportedNotInfinity(t *testing.T) {
	line := Line{ID: "L", Points: []Point{
		mkPoint("a", 1000, 2000, 0, 0),
		mkPoint("dup", 1000, 2000, 100, 5), // 同坐标、不同里程
		mkPoint("b", 1100, 2000, 200, 1),
	}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 0.05, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	if lr.Status != StatusDegraded {
		t.Fatalf("含零距离段应降级，实际 %q", lr.Status)
	}
	if len(lr.Gradient.DataAnomalies) != 1 {
		t.Fatalf("应报 1 个零距离数据异常，实际 %d", len(lr.Gradient.DataAnomalies))
	}
	za := lr.Gradient.DataAnomalies[0]
	if za.FromPointID != "a" || za.ToPointID != "dup" {
		t.Fatalf("零距离异常点对错误：%s→%s", za.FromPointID, za.ToPointID)
	}
	// 另一个正常段照算，且任何段梯度都不得是 Inf/NaN。
	if len(lr.Gradient.Segments) != 1 {
		t.Fatalf("正常段应照常给出 1 段，实际 %d", len(lr.Gradient.Segments))
	}
	for _, s := range lr.Gradient.Segments {
		if math.IsInf(s.GradientMGalPerM, 0) || math.IsNaN(s.GradientMGalPerM) {
			t.Fatalf("梯度绝不能为 Inf/NaN：%v", s.GradientMGalPerM)
		}
	}
}

// ---- 统计画像 -------------------------------------------------------------

func TestStatisticsFromRealReductions(t *testing.T) {
	// 异常值 0,1,2,3,10（10 为离群点）。
	line := eastWestLine("L", 0, []float64{0, 100, 200, 300, 400}, []float64{0, 1, 2, 3, 10})
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	st := lr.Stats
	if st.Count != 5 {
		t.Fatalf("点数应为 5，实际 %d", st.Count)
	}
	if math.Abs(st.MeanMGal-3.2) > 1e-9 {
		t.Fatalf("均值应为 3.2，实际 %v", st.MeanMGal)
	}
	var wantStd float64
	for _, v := range []float64{0, 1, 2, 3, 10} {
		wantStd += (v - 3.2) * (v - 3.2)
	}
	wantStd = math.Sqrt(wantStd / 5)
	if math.Abs(st.StdMGal-wantStd) > 1e-9 {
		t.Fatalf("标准差应为 %v，实际 %v", wantStd, st.StdMGal)
	}
	if math.Abs(st.MinMGal-0) > 1e-9 || st.MinPointID != "L-0" {
		t.Fatalf("最小值应落在 L-0：%v %s", st.MinMGal, st.MinPointID)
	}
	if math.Abs(st.MaxMGal-10) > 1e-9 || st.MaxPointID != "L-4" {
		t.Fatalf("最大值应落在离群点 L-4：%v %s", st.MaxMGal, st.MaxPointID)
	}
}

func TestStatisticsSinglePointStdZero(t *testing.T) {
	line := Line{ID: "L", Points: []Point{mkPoint("only", 0, 0, 0, 7)}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := findLine(t, res, "L")
	if lr.Stats.Count != 1 || lr.Stats.StdMGal != 0 ||
		lr.Stats.MinPointID != "only" || lr.Stats.MaxPointID != "only" {
		t.Fatalf("单点统计画像错误：%+v", lr.Stats)
	}
	if math.Abs(lr.Stats.MeanMGal-7) > 1e-9 {
		t.Fatalf("单点均值应为 7，实际 %v", lr.Stats.MeanMGal)
	}
}
