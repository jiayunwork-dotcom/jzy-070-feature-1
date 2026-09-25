package api

import (
	"math"
	"net/http"
	"testing"
)

func ptr(x float64) *float64 { return &x }

// apiPoint 构造一条线上的测点 DTO（请求体）。
func apiPoint(id string, station, e, n, h float64) map[string]any {
	return map[string]any{
		"id":         id,
		"gobs":       9.8060,
		"h":          h,
		"phi":        45.0,
		"rho":        2.67,
		"easting_m":  e,
		"northing_m": n,
		"station_m":  station,
	}
}

func linesBody(gradThr, intTol float64, lines ...map[string]any) map[string]any {
	return map[string]any{
		"gradient_threshold_mgal_per_km": gradThr,
		"intersection_tolerance_mgal":    intTol,
		"lines":                          lines,
	}
}

func lineDTO(id string, pts ...map[string]any) map[string]any {
	return map[string]any{"id": id, "points": pts}
}

func TestLinesReduceHappyPath(t *testing.T) {
	body := linesBody(30, 0.5,
		lineDTO("A",
			apiPoint("A0", 0, 0, 0, 100),
			apiPoint("A1", 100, 100, 0, 100),
		),
		lineDTO("B",
			apiPoint("B0", 0, 50, -50, 100),
			apiPoint("B1", 100, 50, 50, 100),
		),
	)
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}

	lines := out["lines"].([]any)
	if len(lines) != 2 {
		t.Fatalf("应有 2 条线，实际 %d", len(lines))
	}
	l0 := lines[0].(map[string]any)
	if l0["status"] != "ok" {
		t.Fatalf("线状态应为 ok，实际 %v", l0["status"])
	}
	pts := l0["points"].([]any)
	// 每个点必须带真实归算出的布格异常（mGal 与 m/s² 都在）。
	for _, hp := range pts {
		p := hp.(map[string]any)
		an := p["bouguer_anomaly"].(map[string]any)
		if _, ok := an["mgal"].(float64); !ok {
			t.Fatalf("点 %s 缺少真实布格异常 mGal", p["id"])
		}
		if _, ok := an["m_s2"].(float64); !ok {
			t.Fatalf("点 %s 缺少真实布格异常 m/s²", p["id"])
		}
	}
	stats := l0["statistics"].(map[string]any)
	if stats["point_count"].(float64) != 2 {
		t.Fatalf("统计点数应为 2，实际 %v", stats["point_count"])
	}

	pairs := out["pairs"].([]any)
	if len(pairs) != 1 {
		t.Fatalf("两线应产生 1 个配对，实际 %d", len(pairs))
	}
	pair := pairs[0].(map[string]any)
	if pair["status"] != "intersected" {
		t.Fatalf("应相交，实际 %v", pair["status"])
	}
	if math.Abs(pair["easting_m"].(float64)-50) > 1e-6 {
		t.Fatalf("交点东向应为 50，实际 %v", pair["easting_m"])
	}
}

func TestLinesMissingCoordinateRejected(t *testing.T) {
	p := apiPoint("A0", 0, 0, 0, 100)
	delete(p, "easting_m") // 缺平面坐标：测线组织不起来，必须带原因拒绝
	body := linesBody(30, 0.5, lineDTO("A", p))
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺坐标应 422，实际 %d: %v", code, out)
	}
	e := out["error"].(map[string]any)
	if e["field"] != "lines[0].points[0].easting_m" {
		t.Fatalf("错误字段应定位到具体点坐标，实际 %v", e["field"])
	}
}

func TestLinesMissingThresholdRejected(t *testing.T) {
	body := linesBody(30, 0.5, lineDTO("A", apiPoint("A0", 0, 0, 0, 100)))
	delete(body, "intersection_tolerance_mgal")
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺容差应 422，实际 %d: %v", code, out)
	}
	if out["error"].(map[string]any)["field"] != "intersection_tolerance_mgal" {
		t.Fatalf("错误字段应为容差字段，实际 %v", out)
	}
}

func TestLinesRejectsUnknownField(t *testing.T) {
	body := linesBody(30, 0.5, lineDTO("A", apiPoint("A0", 0, 0, 0, 100)))
	body["dispatch"] = "not-allowed" // 本服务不做派工调度，未知字段直接拒绝
	code, _ := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusBadRequest {
		t.Fatalf("未知字段应 400，实际 %d", code)
	}
}

func TestLinesEmptyLinesRejected(t *testing.T) {
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce",
		map[string]any{"gradient_threshold_mgal_per_km": 30, "intersection_tolerance_mgal": 0.5, "lines": []any{}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("空测线集应 422，实际 %d: %v", code, out)
	}
}

// 单点测线：HTTP 上仍是 200（能归算），但线状态降级、配对如实报无交点。
func TestLinesSinglePointDegraded(t *testing.T) {
	body := linesBody(30, 0.5,
		lineDTO("S", apiPoint("S0", 0, 0, 0, 100)),
		lineDTO("O",
			apiPoint("O0", 0, 50, -50, 100),
			apiPoint("O1", 100, 50, 50, 100),
		),
	)
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("单点降级不应整体失败，期望 200，实际 %d: %v", code, out)
	}
	var s map[string]any
	for _, hl := range out["lines"].([]any) {
		l := hl.(map[string]any)
		if l["id"] == "S" {
			s = l
		}
	}
	if s["status"] != "degraded" {
		t.Fatalf("单点线应降级，实际 %v", s["status"])
	}
	reasons := s["degrade_reasons"].([]any)
	if reasons[0].(map[string]any)["code"] != "single_point" {
		t.Fatalf("降级原因应为 single_point，实际 %v", reasons)
	}
	g := s["gradient"].(map[string]any)
	if g["status"] != "skipped" {
		t.Fatalf("梯度应跳过，实际 %v", g["status"])
	}
	pair := out["pairs"].([]any)[0].(map[string]any)
	if pair["status"] != "no_intersection" {
		t.Fatalf("单点参与的配对应无有效交点，实际 %v", pair["status"])
	}
	if _, present := pair["difference_mgal"]; present {
		t.Fatal("无交点时绝不能硬凑交点差")
	}
}

// 平行线：HTTP 层应明确回 reason.code=parallel。
func TestLinesParallelNoIntersection(t *testing.T) {
	body := linesBody(30, 0.5,
		lineDTO("A",
			apiPoint("A0", 0, 0, 0, 100),
			apiPoint("A1", 100, 100, 0, 100),
		),
		lineDTO("B",
			apiPoint("B0", 0, 0, 50, 100),
			apiPoint("B1", 100, 100, 50, 100),
		),
	)
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	pair := out["pairs"].([]any)[0].(map[string]any)
	if pair["status"] != "no_intersection" {
		t.Fatalf("平行线应无交点，实际 %v", pair["status"])
	}
	if pair["reason"].(map[string]any)["code"] != "parallel" {
		t.Fatalf("原因应为 parallel，实际 %v", pair["reason"])
	}
}

// 零距离重复布点：以 coincident 段明确报出，不产生 Inf、不报错崩溃。
func TestLinesZeroDistanceCoincident(t *testing.T) {
	body := linesBody(30, 0.5,
		lineDTO("A",
			apiPoint("A0", 0, 100, 100, 100),
			apiPoint("A1", 100, 100, 100, 100), // 与 A0 同位置
			apiPoint("A2", 200, 200, 100, 100),
		),
	)
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	line := out["lines"].([]any)[0].(map[string]any)
	grad := line["gradient"].(map[string]any)
	if grad["coincident_count"].(float64) != 1 {
		t.Fatalf("应有 1 个零距离段，实际 %v", grad["coincident_count"])
	}
	segs := grad["segments"].([]any)
	var kinds []string
	for _, hs := range segs {
		kinds = append(kinds, hs.(map[string]any)["kind"].(string))
	}
	if kinds[0] != "coincident" {
		t.Fatalf("首段应为 coincident，实际 %v", kinds)
	}
}

// 旧接口完全不受影响：单点 / 扫描 / 示例仍在。
func TestLegacyEndpointsUnaffected(t *testing.T) {
	if code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce",
		map[string]any{"gobs": 9.8060, "h": 200.0, "phi": 45.0, "rho": 2.67}); code != http.StatusOK {
		t.Fatalf("单点接口异常 code=%d body=%v", code, out)
	}
	if code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/scan",
		map[string]any{"gobs": 9.8060, "phi": 45.0, "rho": 2.67, "heights": []float64{0, 100}}); code != http.StatusOK {
		t.Fatalf("扫描接口异常 code=%d body=%v", code, out)
	}
	if code, out := doJSON(t, setupRouter(), http.MethodGet, "/api/v1/sample", nil); code != http.StatusOK {
		t.Fatalf("示例接口异常 code=%d body=%v", code, out)
	}
}
