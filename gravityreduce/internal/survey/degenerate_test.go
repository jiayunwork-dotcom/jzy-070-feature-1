package survey

import (
	"math"
	"strings"
	"testing"
)

// 撑不起诊断的输入必须被带原因地拒绝（硬错误）或降级说明（软情形），
// 不能回一个看似正常其实没意义的结果。

// TestSinglePointLineDegraded 单点测线：归算与统计照给，梯度/交点诊断降级。
func TestSinglePointLineDegraded(t *testing.T) {
	res := mustProcess(t,
		[]Line{{ID: "solo", Points: []Point{mkPoint("p", 5, 6, 0, 1)}}},
		Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := res.Lines[0]
	if lr.Status != StatusDegraded || lr.Gradient.Available {
		t.Fatalf("单点线应降级且梯度不可用：status=%q", lr.Status)
	}
	if len(lr.Notes) == 0 || !strings.Contains(lr.Notes[0], "1 个测点") {
		t.Fatalf("降级必须带原因：%v", lr.Notes)
	}
	// 归算结果与统计仍真实给出。
	if len(lr.Ordered) != 1 || lr.Stats.Count != 1 {
		t.Fatalf("单点线仍应给出该点归算与统计")
	}
	// 单点线与任何线都不产生交点差，并说明原因。
	other := eastWestLine("other", 5, []float64{0, 100}, []float64{0, 1})
	res2 := mustProcess(t,
		[]Line{{ID: "solo", Points: []Point{mkPoint("p", 5, 6, 0, 1)}}, other},
		Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	pair := findPair(t, res2, "solo", "other")
	if pair.Status != PairStatusNoValidCrossing || pair.Reason != ReasonLineUnavailable {
		t.Fatalf("单点线参与的线对应无有效交点（线不可用）：%s %q", pair.Status, pair.Reason)
	}
}

// TestMissingStationsDegraded 缺里程 → 无法定序 → 梯度与交点降级，但归算/统计仍给。
func TestMissingStationsDegraded(t *testing.T) {
	noStation := func(p Point) Point { p.Station = nil; return p }
	line := Line{ID: "L", Points: []Point{
		noStation(mkPoint("a", 0, 0, 0, 0)),
		noStation(mkPoint("b", 100, 0, 100, 1)),
		noStation(mkPoint("c", 200, 0, 200, 2)),
	}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := res.Lines[0]
	if lr.Status != StatusDegraded || lr.Gradient.Available {
		t.Fatalf("缺里程应降级且梯度不可用")
	}
	if !strings.Contains(strings.Join(lr.Notes, " "), "里程") {
		t.Fatalf("降级原因应提到里程：%v", lr.Notes)
	}
	// 点按送入顺序保留，归算与统计照给。
	if lr.Ordered[0].Point.ID != "a" || lr.Stats.Count != 3 {
		t.Fatalf("缺里程时应保留送入顺序并给出统计")
	}
}

// TestDuplicateStationsDegraded 重复里程无法唯一确定先后，同样降级。
func TestDuplicateStationsDegraded(t *testing.T) {
	line := Line{ID: "L", Points: []Point{
		mkPoint("a", 0, 0, 0, 0),
		mkPoint("b", 100, 0, 100, 1),
		mkPoint("c", 105, 5, 100, 2), // 与 b 同里程
		mkPoint("d", 200, 0, 200, 3),
	}}
	res := mustProcess(t, []Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	lr := res.Lines[0]
	if lr.Status != StatusDegraded || lr.Gradient.Available {
		t.Fatalf("重复里程应降级且梯度不可用")
	}
	if !strings.Contains(strings.Join(lr.Notes, " "), "里程") {
		t.Fatalf("降级原因应说明里程重复：%v", lr.Notes)
	}
}

// TestPartialStationsRejected 里程只标一部分：硬错误拒绝并定位到具体线。
func TestPartialStationsRejected(t *testing.T) {
	missing := mkPoint("c", 200, 0, 200, 2)
	missing.Station = nil
	line := Line{ID: "L", Points: []Point{
		mkPoint("a", 0, 0, 0, 0),
		mkPoint("b", 100, 0, 100, 1),
		missing,
	}}
	err := Validate([]Line{line}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	if err == nil {
		t.Fatalf("部分里程应被拒绝")
	}
	fe, ok := err.(FieldError)
	if !ok || !strings.Contains(fe.Field, "lines[0]") || !strings.Contains(fe.Reason, "里程") {
		t.Fatalf("应返回带定位与原因的 FieldError，实际 %v", err)
	}
}

// TestNonFiniteCoordinatesRejected 坐标为 NaN/±Inf 等非有限值必须硬拒绝，
// 并把错误字段定位到具体测点的 easting/northing。
func TestNonFiniteCoordinatesRejected(t *testing.T) {
	th := Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1}

	p := mkPoint("p", 0, 0, 0, 0)
	p.Easting = math.NaN()
	if err := Validate([]Line{{ID: "L", Points: []Point{p}}}, th); err == nil ||
		!strings.HasSuffix(err.(FieldError).Field, ".easting") {
		t.Fatalf("东向坐标 NaN 应以 .easting 拒绝，实际 %v", err)
	}

	p = mkPoint("p", 0, 0, 0, 0)
	p.Northing = math.Inf(1)
	if err := Validate([]Line{{ID: "L", Points: []Point{p}}}, th); err == nil ||
		!strings.HasSuffix(err.(FieldError).Field, ".northing") {
		t.Fatalf("北向坐标 +Inf 应以 .northing 拒绝，实际 %v", err)
	}
}

// TestInvalidObservationRejected 单点物理量不合法在线级入口拒绝，
// 校验规则直接复用 validate.Point。
func TestInvalidObservationRejected(t *testing.T) {
	th := Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1}
	cases := []struct {
		modify func(p *Point)
		suffix string
	}{
		{func(p *Point) { p.Obs.Rho = 0 }, ".rho"},
		{func(p *Point) { p.Obs.Phi = 91 }, ".phi"},
		{func(p *Point) { p.Obs.GobsMS2 = -1 }, ".gobs"},
	}
	for i, c := range cases {
		p := mkPoint("p", 0, 0, 0, 0)
		c.modify(&p)
		err := Validate([]Line{{ID: "L", Points: []Point{p}}}, th)
		if err == nil {
			t.Fatalf("case %d 应被拒绝", i)
		}
		fe, ok := err.(FieldError)
		if !ok || !strings.HasSuffix(fe.Field, c.suffix) {
			t.Fatalf("case %d 错误字段应以 %s 结尾，实际 %v", i, c.suffix, err)
		}
	}
}

// TestAllPointsCoincidentNoHorizontalExtent 多点但平面位置全部重合：
// 无水平走向，与任何线都不产生交点差。
func TestAllPointsCoincidentNoHorizontalExtent(t *testing.T) {
	coincident := Line{ID: "C", Points: []Point{
		mkPoint("a", 10, 10, 0, 0),
		mkPoint("b", 10, 10, 100, 1),
	}}
	other := eastWestLine("L", 10, []float64{0, 200}, []float64{0, 1})
	res := mustProcess(t, []Line{coincident, other},
		Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1})
	pair := findPair(t, res, "C", "L")
	if pair.Status != PairStatusNoValidCrossing || pair.Reason != ReasonNoHorizontalExtent {
		t.Fatalf("无水平走向的线对应无有效交点：status=%s reason=%q", pair.Status, pair.Reason)
	}
}

// TestBatchLevelValidationErrors 批级与线级硬错误：空批、缺线号、线号重复、点号重复、空线。
func TestBatchLevelValidationErrors(t *testing.T) {
	th := Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 1}
	goodPoint := func(id string) Point { return mkPoint(id, 0, 0, 0, 0) }
	cases := []struct {
		name  string
		lines []Line
		field string
	}{
		{"空批", nil, "lines"},
		{"缺线号", []Line{{Points: []Point{goodPoint("p")}}}, ".id"},
		{"线号重复", []Line{
			{ID: "X", Points: []Point{goodPoint("p1")}},
			{ID: "X", Points: []Point{goodPoint("p2")}},
		}, ".id"},
		{"空线", []Line{{ID: "X", Points: nil}}, ".points"},
		{"点号重复", []Line{{ID: "X", Points: []Point{goodPoint("p"), goodPoint("p")}}}, ".id"},
	}
	for _, cs := range cases {
		err := Validate(cs.lines, th)
		if err == nil {
			t.Fatalf("case %s 应被拒绝", cs.name)
		}
		fe, ok := err.(FieldError)
		if !ok || !strings.HasSuffix(fe.Field, cs.field) {
			t.Fatalf("case %s 错误字段应以 %s 结尾，实际 %v", cs.name, cs.field, err)
		}
	}
}

// TestThresholdValidation 阈值必须为正/非负有限数。
func TestThresholdValidation(t *testing.T) {
	lines := []Line{{ID: "L", Points: []Point{mkPoint("p", 0, 0, 0, 0)}}}
	if err := Validate(lines, Thresholds{GradientMGalPerM: 0, IntersectionTolMGal: 1}); err == nil {
		t.Fatalf("梯度阈值为零应拒绝")
	}
	if err := Validate(lines, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: -1}); err == nil {
		t.Fatalf("负交点差容限应拒绝")
	}
	if err := Validate(lines, Thresholds{GradientMGalPerM: math.NaN(), IntersectionTolMGal: 1}); err == nil {
		t.Fatalf("NaN 梯度阈值应拒绝")
	}
}

// TestPairsEnumeratedAllCombinations 三条线：全部两两组合都要过一遍。
func TestPairsEnumeratedAllCombinations(t *testing.T) {
	l1 := eastWestLine("L1", 0, []float64{0, 400}, []float64{0, 4})
	l2 := eastWestLine("L2", 100, []float64{0, 400}, []float64{0, 4})      // 与 L1 平行
	l3 := lineFromXY("L3", []xy{{100, -100}, {100, 100}}, []float64{0, 2}) // 与两条横线相交
	res := mustProcess(t, []Line{l1, l2, l3}, Thresholds{GradientMGalPerM: 1, IntersectionTolMGal: 100})
	if len(res.Pairs) != 3 {
		t.Fatalf("3 条线应有 C(3,2)=3 个线对结果，实际 %d", len(res.Pairs))
	}
	p12 := findPair(t, res, "L1", "L2")
	p13 := findPair(t, res, "L1", "L3")
	p23 := findPair(t, res, "L2", "L3")
	if p12.Status != PairStatusNoValidCrossing {
		t.Fatalf("L1/L2 平行应无交点")
	}
	if p13.Status != PairStatusIntersects || p23.Status != PairStatusIntersects {
		t.Fatalf("L3 应分别与 L1、L2 相交：%s %s", p13.Status, p23.Status)
	}
	for _, pr := range []PairResult{p13, p23} {
		cr := pr.Crossings[0]
		if cr.LineASegmentFromPointID == "" || cr.LineBSegmentFromPointID == "" {
			t.Fatalf("交点结果必须给出两侧插值所用测点段")
		}
	}
}
