package api

import (
	"math"
	"net/http"
	"testing"
)

// crossingLinesBody 的两个高程参数分别控制 L1、L2 整条线的统一高程；
// 交点 (200,0) 是两条线各段的中点，整线抬高 Δh 时交点处异常整体上移
// (0.3086−0.04193·ρ)·Δh。
func crossingLinesBody(h1, h2 float64) map[string]any {
	pt := func(id string, e, n, s, h float64) map[string]any {
		return map[string]any{"id": id, "easting_m": e, "northing_m": n, "station_m": s,
			"gobs": 9.8060, "h": h, "phi": 45.0, "rho": 2.67}
	}
	return map[string]any{
		"gradient_threshold_mgal_per_m": 0.05,
		"intersection_tolerance_mgal":   0.5,
		"lines": []any{
			map[string]any{"id": "L1", "points": []any{
				pt("a", 0, 0, 0, h1), pt("b", 200, 0, 200, h1), pt("c", 400, 0, 400, h1),
			}},
			map[string]any{"id": "L2", "points": []any{
				pt("d", 200, -100, 0, h2), pt("e", 200, 100, 200, h2),
			}},
		},
	}
}

func TestLinesEndpointHappyPath(t *testing.T) {
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", crossingLinesBody(100, 100))
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	lines := out["lines"].([]any)
	if len(lines) != 2 {
		t.Fatalf("应回 2 条线，实际 %d", len(lines))
	}
	for _, hl := range lines {
		l := hl.(map[string]any)
		if l["status"] != "ok" {
			t.Fatalf("线 %s 状态应为 ok，实际 %v（notes=%v）", l["id"], l["status"], l["notes"])
		}
		pts := l["points"].([]any)
		if len(pts) != 2 && len(pts) != 3 {
			t.Fatalf("点数异常：%d", len(pts))
		}
		// 每个点都必须复用单点归算给出布格异常，而不是占位值。
		for _, hp := range pts {
			p := hp.(map[string]any)
			an := p["bouguer_anomaly"].(map[string]any)
			if an["mgal"] == nil {
				t.Fatalf("点 %s 缺布格异常", p["id"])
			}
			ng := p["normal_gravity"].(map[string]any)
			if math.Abs(ng["mgal"].(float64)-980619.920257) > 1e-3 {
				t.Fatalf("点 %s 正常重力应复用单点链条（约 980619.92），实际 %v", p["id"], ng["mgal"])
			}
		}
		// 统计画像必须有真实点数。
		st := l["statistics"].(map[string]any)
		if int(st["count"].(float64)) != len(pts) {
			t.Fatalf("统计点数不匹配")
		}
	}

	pairs := out["pairs"].([]any)
	if len(pairs) != 1 {
		t.Fatalf("2 条线应有 1 个线对，实际 %d", len(pairs))
	}
	pair := pairs[0].(map[string]any)
	if pair["status"] != "intersects" {
		t.Fatalf("两线应相交：%v", pair)
	}
	cr := pair["crossings"].([]any)[0].(map[string]any)
	if math.Abs(cr["easting_m"].(float64)-200) > 1e-6 || math.Abs(cr["northing_m"].(float64)) > 1e-6 {
		t.Fatalf("交点应为 (200,0)，实际 %v,%v", cr["easting_m"], cr["northing_m"])
	}
}

func TestLinesEndpointSystematicShiftReflected(t *testing.T) {
	baseCode, base := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", crossingLinesBody(100, 100))
	if baseCode != http.StatusOK {
		t.Fatalf("基线请求失败：%d %v", baseCode, base)
	}
	baseDiff := base["pairs"].([]any)[0].(map[string]any)["crossings"].([]any)[0].(map[string]any)["difference_mgal"].(float64)

	// L2 的 e 点整体抬高 50 m（gobs 不变），L1/L2 在交点处出现系统性异常差。
	shiftCode, shifted := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", crossingLinesBody(100, 150))
	if shiftCode != http.StatusOK {
		t.Fatalf("抬高请求失败：%d %v", shiftCode, shifted)
	}
	shiftDiff := shifted["pairs"].([]any)[0].(map[string]any)["crossings"].([]any)[0].(map[string]any)["difference_mgal"].(float64)

	want := (0.3086 - 0.04193*2.67) * 50
	if math.Abs(math.Abs(shiftDiff-baseDiff)-want) > 1e-4 {
		t.Fatalf("抬高 50 m 后交点差变化应为 %v，实际基线 %v → %v", want, baseDiff, shiftDiff)
	}
	within := shifted["pairs"].([]any)[0].(map[string]any)["crossings"].([]any)[0].(map[string]any)["within_tolerance"].(bool)
	if within {
		t.Fatalf("系统偏移应超出 0.5 mGal 容限")
	}
}

func TestLinesEndpointParallelNoCrossing(t *testing.T) {
	pt := func(id string, e, n float64) map[string]any {
		return map[string]any{"id": id, "easting_m": e, "northing_m": n, "station_m": e,
			"gobs": 9.8060, "h": 100, "phi": 45.0, "rho": 2.67}
	}
	body := map[string]any{
		"gradient_threshold_mgal_per_m": 1.0,
		"intersection_tolerance_mgal":   1.0,
		"lines": []any{
			map[string]any{"id": "L1", "points": []any{pt("a", 0, 0), pt("b", 200, 0)}},
			map[string]any{"id": "L2", "points": []any{pt("c", 0, 50), pt("d", 200, 50)}},
		},
	}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	pair := out["pairs"].([]any)[0].(map[string]any)
	if pair["status"] != "no_valid_crossing" {
		t.Fatalf("平行线应无有效交点，实际 %v", pair["status"])
	}
	if pair["reason"] != "parallel_distinct" {
		t.Fatalf("原因码应为 parallel_distinct，实际 %v", pair["reason"])
	}
	if len(pair["crossings"].([]any)) != 0 {
		t.Fatalf("无有效交点时不得硬凑 crossings")
	}
}

func TestLinesEndpointMissingCoordinateRejected(t *testing.T) {
	body := map[string]any{
		"gradient_threshold_mgal_per_m": 1.0,
		"intersection_tolerance_mgal":   1.0,
		"lines": []any{
			map[string]any{"id": "L1", "points": []any{
				map[string]any{"id": "a", "northing_m": 0.0, "station_m": 0.0,
					"gobs": 9.8060, "h": 100, "phi": 45.0, "rho": 2.67}, // 缺 easting_m
			}},
		},
	}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺坐标应返回 422，实际 %d: %v", code, out)
	}
	errObj := out["error"].(map[string]any)
	if errObj["field"] != "lines[0].points[0].easting_m" {
		t.Fatalf("错误字段应定位到缺失坐标，实际 %v", errObj["field"])
	}
	if errObj["message"] == "" {
		t.Fatalf("错误必须带原因")
	}
}

func TestLinesEndpointMissingThresholdRejected(t *testing.T) {
	body := map[string]any{
		"intersection_tolerance_mgal": 1.0,
		"lines": []any{
			map[string]any{"id": "L1", "points": []any{
				map[string]any{"id": "a", "easting_m": 0.0, "northing_m": 0.0, "station_m": 0.0,
					"gobs": 9.8060, "h": 100, "phi": 45.0, "rho": 2.67},
			}},
		},
	}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺梯度阈值应 422，实际 %d: %v", code, out)
	}
	if out["error"].(map[string]any)["field"] != "gradient_threshold_mgal_per_m" {
		t.Fatalf("错误字段应为梯度阈值：%v", out)
	}
}

func TestLinesEndpointUnknownFieldRejected(t *testing.T) {
	body := crossingLinesBody(100, 100)
	body["dispatch"] = "nope" // 本服务不做派工调度，未知字段必须拒绝
	code, _ := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusBadRequest {
		t.Fatalf("未知字段应 400，实际 %d", code)
	}
}

func TestLinesEndpointSinglePointDegradedNotRejected(t *testing.T) {
	body := map[string]any{
		"gradient_threshold_mgal_per_m": 1.0,
		"intersection_tolerance_mgal":   1.0,
		"lines": []any{
			map[string]any{"id": "solo", "points": []any{
				map[string]any{"id": "a", "easting_m": 0.0, "northing_m": 0.0, "station_m": 0.0,
					"gobs": 9.8060, "h": 100, "phi": 45.0, "rho": 2.67},
			}},
		},
	}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("单点线不是硬错误，应 200 降级，实际 %d: %v", code, out)
	}
	l := out["lines"].([]any)[0].(map[string]any)
	if l["status"] != "degraded" {
		t.Fatalf("单点线应降级，实际 %v", l["status"])
	}
	if l["gradient"].(map[string]any)["available"] != false {
		t.Fatalf("单点线梯度诊断应标记为不可用")
	}
	notes := l["notes"].([]any)
	if len(notes) == 0 {
		t.Fatalf("降级必须带原因说明")
	}
}

func TestLinesEndpointZeroDistanceDataAnomaly(t *testing.T) {
	dup := func(id string, e, n, s float64) map[string]any {
		return map[string]any{"id": id, "easting_m": e, "northing_m": n, "station_m": s,
			"gobs": 9.8060, "h": 100, "phi": 45.0, "rho": 2.67}
	}
	body := map[string]any{
		"gradient_threshold_mgal_per_m": 0.05,
		"intersection_tolerance_mgal":   1.0,
		"lines": []any{
			map[string]any{"id": "L1", "points": []any{
				dup("a", 0, 0, 0), dup("dup", 0, 0, 100), dup("b", 100, 0, 200),
			}},
		},
	}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/lines/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("零距离是数据异常而非请求错误，应 200，实际 %d: %v", code, out)
	}
	l := out["lines"].([]any)[0].(map[string]any)
	if l["status"] != "degraded" {
		t.Fatalf("含零距离段应降级")
	}
	anoms := l["gradient"].(map[string]any)["data_anomalies"].([]any)
	if len(anoms) != 1 {
		t.Fatalf("应报 1 个零距离数据异常，实际 %d", len(anoms))
	}
	za := anoms[0].(map[string]any)
	if za["from_point_id"] != "a" || za["to_point_id"] != "dup" {
		t.Fatalf("零距离点对错误：%v → %v", za["from_point_id"], za["to_point_id"])
	}
}

// TestExistingEndpointsUnaffected 新增能力不得改变原有单点接口的可用性。
func TestExistingEndpointsUnaffected(t *testing.T) {
	for _, p := range []string{"/api/v1/reduce", "/api/v1/scan"} {
		var body map[string]any
		if p == "/api/v1/reduce" {
			body = map[string]any{"gobs": 9.8060, "h": 200, "phi": 45.0, "rho": 2.67}
		} else {
			body = map[string]any{"gobs": 9.8060, "phi": 45.0, "rho": 2.67, "heights": []float64{0, 100}}
		}
		code, out := doJSON(t, setupRouter(), http.MethodPost, p, body)
		if code != http.StatusOK {
			t.Fatalf("原接口 %s 应仍为 200，实际 %d: %v", p, code, out)
		}
	}
	code, out := doJSON(t, setupRouter(), http.MethodGet, "/api/v1/sample", nil)
	if code != http.StatusOK || out["reduction"] == nil {
		t.Fatalf("内置示例接口应保持可用：%d", code)
	}
}
