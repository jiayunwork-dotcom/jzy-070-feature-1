package api

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"gravityreduce/internal/gravity"
	"gravityreduce/internal/units"
)

func setupRouter() http.Handler { return NewRouter() }

func doJSON(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("编码请求失败: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("响应不是 JSON: %v\nbody=%s", err, rec.Body.String())
		}
	}
	return rec.Code, out
}

func TestHealth(t *testing.T) {
	code, out := doJSON(t, setupRouter(), http.MethodGet, "/healthz", nil)
	if code != http.StatusOK || out["status"] != "ok" {
		t.Fatalf("健康检查异常: code=%d body=%v", code, out)
	}
}

func TestReduceEndpointHappyPath(t *testing.T) {
	body := map[string]any{"gobs": 9.8060, "h": 200.0, "phi": 45.0, "rho": 2.67}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}

	fa := out["free_air_correction"].(map[string]any)
	bb := out["bouguer_slab_correction"].(map[string]any)
	ng := out["normal_gravity"].(map[string]any)
	an := out["bouguer_anomaly"].(map[string]any)

	if math.Abs(fa["magnitude_mgal"].(float64)-61.72) > 1e-6 {
		t.Fatalf("自由空气改正应为 61.72，实际 %v", fa["magnitude_mgal"])
	}
	wantB := 0.04193 * 2.67 * 200
	if math.Abs(bb["magnitude_mgal"].(float64)-wantB) > 1e-6 {
		t.Fatalf("布格板改正应为 %v，实际 %v", wantB, bb["magnitude_mgal"])
	}
	// 带符号项：FA 为正，B 为负。
	if fa["signed_mgal"].(float64) <= 0 {
		t.Fatalf("自由空气带符号项应为正，实际 %v", fa["signed_mgal"])
	}
	if bb["signed_mgal"].(float64) >= 0 {
		t.Fatalf("布格板带符号项应为负，实际 %v", bb["signed_mgal"])
	}

	// 合成式交叉验证（接口层再抓一次符号翻转）。
	gobsMGal := units.MS2ToMGal(9.8060)
	gammaMGal := ng["mgal"].(float64)
	wantAnomaly := gobsMGal - gammaMGal + 61.72 - wantB
	if math.Abs(an["mgal"].(float64)-wantAnomaly) > 1e-4 {
		t.Fatalf("布格异常合成错误：got %v want %v", an["mgal"], wantAnomaly)
	}
}

func TestReduceZeroHeightIdentity(t *testing.T) {
	body := map[string]any{"gobs": 9.8062, "h": 0.0, "phi": 45.0, "rho": 2.67}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	fa := out["free_air_correction"].(map[string]any)["magnitude_mgal"].(float64)
	bb := out["bouguer_slab_correction"].(map[string]any)["magnitude_mgal"].(float64)
	if fa != 0 || bb != 0 {
		t.Fatalf("h=0 两项改正必须为 0：FA=%v B=%v", fa, bb)
	}
	an := out["bouguer_anomaly"].(map[string]any)["mgal"].(float64)
	ng := out["normal_gravity"].(map[string]any)["mgal"].(float64)
	want := units.MS2ToMGal(9.8062) - ng
	if math.Abs(an-want) > 1e-4 {
		t.Fatalf("h=0 布格异常应等于 gobs−γ：got %v want %v", an, want)
	}
}

func TestReduceMissingFields(t *testing.T) {
	// 缺失 h（h=0 与缺失必须区分）。
	body := map[string]any{"gobs": 9.8060, "phi": 45.0, "rho": 2.67}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺失 h 应返回 422，实际 %d", code)
	}
	if e, _ := out["error"].(map[string]any); e["field"] != "h" {
		t.Fatalf("错误字段应为 h，实际 %v", out)
	}
}

func TestReduceInvalidInputs(t *testing.T) {
	cases := []map[string]any{
		{"gobs": 9.8060, "h": 200, "phi": 91.0, "rho": 2.67}, // 纬度越界
		{"gobs": 9.8060, "h": 200, "phi": 45.0, "rho": 0},    // 密度非正
		{"gobs": 9.8060, "h": 200, "phi": 45.0, "rho": -2},   // 密度负
	}
	for i, b := range cases {
		code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce", b)
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("case %d 应返回 422，实际 %d: %v", i, code, out)
		}
		msg, _ := out["error"].(map[string]any)["message"].(string)
		if msg == "" {
			t.Fatalf("case %d 错误必须带原因", i)
		}
	}
}

func TestReduceRejectsBadJSON(t *testing.T) {
	h := NewRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reduce", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应返回 400，实际 %d", rec.Code)
	}
}

func TestReduceRejectsUnknownField(t *testing.T) {
	body := map[string]any{"gobs": 9.8060, "h": 1, "phi": 45, "rho": 2.67, "dop": 3.1}
	code, _ := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/reduce", body)
	if code != http.StatusBadRequest {
		t.Fatalf("未知字段应被拒绝（400），实际 %d", code)
	}
}

func TestScanEndpointPerPointFormula(t *testing.T) {
	heights := []float64{0, 100, 200, 350}
	body := map[string]any{"gobs": 9.8060, "phi": 45.0, "rho": 2.67, "heights": heights}
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/scan", body)
	if code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d: %v", code, out)
	}
	points := out["points"].([]any)
	if len(points) != len(heights) {
		t.Fatalf("点数不匹配：%d", len(points))
	}

	// 用独立的物理公式逐点核对，并抓住自由空气翻号。
	gammaMGal := units.MS2ToMGal(gravity.NormalGravity(45))
	for i, hp := range points {
		p := hp.(map[string]any)
		h := heights[i]
		wantFA := 0.3086 * h
		wantB := 0.04193 * 2.67 * h
		wantAnomaly := units.MS2ToMGal(9.8060) - gammaMGal + wantFA - wantB
		if math.Abs(p["free_air_correction_mgal"].(float64)-wantFA) > 1e-6 {
			t.Fatalf("点 %d 自由空气改正错误：%v vs %v", i, p["free_air_correction_mgal"], wantFA)
		}
		if math.Abs(p["bouguer_slab_correction_mgal"].(float64)-wantB) > 1e-6 {
			t.Fatalf("点 %d 布格板改正错误：%v vs %v", i, p["bouguer_slab_correction_mgal"], wantB)
		}
		an := p["bouguer_anomaly"].(map[string]any)["mgal"].(float64)
		if math.Abs(an-wantAnomaly) > 1e-4 {
			t.Fatalf("点 %d 布格异常错误（可能自由空气翻号）：got %v want %v", i, an, wantAnomaly)
		}
	}
}

func TestScanEmptyAndMissingHeights(t *testing.T) {
	// 空序列
	code, out := doJSON(t, setupRouter(), http.MethodPost, "/api/v1/scan",
		map[string]any{"gobs": 9.8060, "phi": 45, "rho": 2.67, "heights": []float64{}})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("空序列应 422，实际 %d: %v", code, out)
	}
	// 缺失 heights
	code, out = doJSON(t, setupRouter(), http.MethodPost, "/api/v1/scan",
		map[string]any{"gobs": 9.8060, "phi": 45, "rho": 2.67})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("缺失 heights 应 422，实际 %d: %v", code, out)
	}
	// 扫描接口拒绝单点 h
	code, out = doJSON(t, setupRouter(), http.MethodPost, "/api/v1/scan",
		map[string]any{"gobs": 9.8060, "h": 1, "phi": 45, "rho": 2.67, "heights": []float64{1, 2}})
	if code != http.StatusBadRequest {
		t.Fatalf("scan 携带 h 应 400，实际 %d: %v", code, out)
	}
}

func TestSampleEndpoint(t *testing.T) {
	code, out := doJSON(t, setupRouter(), http.MethodGet, "/api/v1/sample", nil)
	if code != http.StatusOK {
		t.Fatalf("示例接口应 200，实际 %d", code)
	}
	r := out["reduction"].(map[string]any)
	fa := r["free_air_correction"].(map[string]any)["magnitude_mgal"].(float64)
	if math.Abs(fa-61.72) > 1e-6 {
		t.Fatalf("示例自由空气改正应为 61.72，实际 %v", fa)
	}
}
