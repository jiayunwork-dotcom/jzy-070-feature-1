package gravity

import (
	"math"
	"testing"

	"gravityreduce/internal/units"
)

const eps = 1e-9

func approxEq(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// 国际正常重力公式：关键纬度锚点（m/s²）。
func TestNormalGravityAnchors(t *testing.T) {
	cases := []struct {
		phi  float64
		want float64
	}{
		{0, 9.7803267715},   // 赤道
		{90, 9.8321863685},  // 北极（GRS80）
		{-90, 9.8321863685}, // 南极：对称
		{45, 9.8061991773},  // 中纬度参考点
	}
	for _, tc := range cases {
		got := NormalGravity(tc.phi)
		if !approxEq(got, tc.want, 1e-7) {
			t.Errorf("NormalGravity(%v) = %.10f，期望 %.10f", tc.phi, got, tc.want)
		}
	}
	// 关于赤道对称
	if !approxEq(NormalGravity(37.5), NormalGravity(-37.5), 1e-12) {
		t.Fatal("正常重力应关于纬度正负对称")
	}
	// 从赤道向两极随纬度绝对值单调增大（南北半球各查一遍）。
	for _, sign := range []float64{1, -1} {
		prev := NormalGravity(0)
		for p := 10.0; p <= 90.0; p += 10 {
			v := NormalGravity(sign * p)
			if v < prev-1e-12 {
				t.Fatalf("正常重力在纬度 %v 处出现非单调", sign*p)
			}
			prev = v
		}
	}
}

// 自由空气改正：Δg_FA = 0.3086·h。
func TestFreeAirCorrection(t *testing.T) {
	if got := FreeAirCorrection(0); got != 0 {
		t.Fatalf("h=0 时自由空气改正必须为 0，实际 %v", got)
	}
	if got := FreeAirCorrection(200); !approxEq(got, 61.72, eps) {
		t.Fatalf("h=200 时 Δg_FA 应为 61.72 mGal，实际 %v", got)
	}
	// 关键：符号方向——高地的自由空气改正量必须为正且随高程增大。
	h1, h2 := 100.0, 300.0
	fa1, fa2 := FreeAirCorrection(h1), FreeAirCorrection(h2)
	if !(fa1 > 0 && fa2 > fa1) {
		t.Fatalf("自由空气改正符号/方向错误：Δg_FA(%v)=%v, Δg_FA(%v)=%v", h1, fa1, h2, fa2)
	}
	// 高程翻倍，自由空气改正翻倍。
	if got := FreeAirCorrection(2 * 123.5); !approxEq(got, 2*FreeAirCorrection(123.5), eps) {
		t.Fatalf("高程翻倍后自由空气改正未翻倍，实际 %v", got)
	}
	// 自由空气改正与密度、纬度完全无关（签名上就只吃 h）。
}

// 布格板改正：Δg_B = 0.04193·ρ·h。
func TestBouguerSlabCorrection(t *testing.T) {
	if got := BouguerSlabCorrection(2.67, 0); got != 0 {
		t.Fatalf("h=0 时布格板改正必须为 0，实际 %v", got)
	}
	want := 0.04193 * 2.67 * 200.0
	if got := BouguerSlabCorrection(2.67, 200); !approxEq(got, want, eps) {
		t.Fatalf("布格板改正应为 %v，实际 %v", want, got)
	}
	// 高程翻倍，板项翻倍。
	b := BouguerSlabCorrection(2.67, 150)
	b2 := BouguerSlabCorrection(2.67, 300)
	if !approxEq(b2, 2*b, eps) {
		t.Fatalf("高程翻倍后布格板改正未翻倍：%v vs 2×%v", b2, b)
	}
	// 密度翻倍，只有板项翻倍。
	br := BouguerSlabCorrection(2.20, 150)
	br2 := BouguerSlabCorrection(4.40, 150)
	if !approxEq(br2, 2*br, eps) {
		t.Fatalf("密度翻倍后布格板改正未翻倍：%v vs 2×%v", br2, br)
	}
	// 板项恒非负。
	if BouguerSlabCorrection(2.67, 200) <= 0 {
		t.Fatal("布格板改正量必须为正")
	}
}

// 关系 1：h=0 时两项改正同时归零，布格异常恰等于 gobs − γ。
func TestReduceZeroHeight(t *testing.T) {
	gobs := 9.8062
	in := Observation{GobsMS2: gobs, H: 0, Phi: 45, Rho: 2.67}
	r := Reduce(in)

	if r.FreeAirMGal != 0 || r.BouguerSlabMGal != 0 {
		t.Fatalf("h=0 时两项改正必须同时为 0，实际 FA=%v B=%v", r.FreeAirMGal, r.BouguerSlabMGal)
	}
	want := units.MS2ToMGal(gobs) - r.NormalGravityMGal
	if !approxEq(r.BouguerAnomalyMGal, want, 1e-7) {
		t.Fatalf("h=0 时布格异常应等于 gobs−γ：%v，实际 %v", want, r.BouguerAnomalyMGal)
	}
}

// 关系 2：其余参数不变，高程翻倍 → 自由空气与布格板两项都翻倍。
func TestReduceHeightDoubling(t *testing.T) {
	base := Observation{GobsMS2: 9.8060, H: 100, Phi: 45, Rho: 2.67}
	r1 := Reduce(base)
	base.H = 200
	r2 := Reduce(base)

	if !approxEq(r2.FreeAirMGal, 2*r1.FreeAirMGal, 1e-7) {
		t.Fatalf("高程翻倍自由空气未翻倍：%v vs 2×%v", r2.FreeAirMGal, r1.FreeAirMGal)
	}
	if !approxEq(r2.BouguerSlabMGal, 2*r1.BouguerSlabMGal, 1e-7) {
		t.Fatalf("高程翻倍布格板未翻倍：%v vs 2×%v", r2.BouguerSlabMGal, r1.BouguerSlabMGal)
	}
	// 正常重力不随高程变化（纬度没变）。
	if r2.NormalGravityMGal != r1.NormalGravityMGal {
		t.Fatal("仅改变高程，正常重力不应变化")
	}
}

// 关系 3：密度翻倍 → 只有布格板改正翻倍，自由空气毫不受影响。
func TestReduceDensityDoubling(t *testing.T) {
	base := Observation{GobsMS2: 9.8060, H: 200, Phi: 45, Rho: 2.67}
	r1 := Reduce(base)
	base.Rho = 5.34
	r2 := Reduce(base)

	if !approxEq(r2.BouguerSlabMGal, 2*r1.BouguerSlabMGal, 1e-7) {
		t.Fatalf("密度翻倍布格板应翻倍：%v vs 2×%v", r2.BouguerSlabMGal, r1.BouguerSlabMGal)
	}
	if r2.FreeAirMGal != r1.FreeAirMGal {
		t.Fatalf("密度翻倍自由空气不应变化：%v vs %v", r2.FreeAirMGal, r1.FreeAirMGal)
	}
	if r2.NormalGravityMGal != r1.NormalGravityMGal {
		t.Fatal("仅改变密度，正常重力不应变化")
	}
}

// 关系 4：仅改纬度 → 正常重力变化，两项高程相关改正保持不变。
func TestReduceLatitudeOnly(t *testing.T) {
	base := Observation{GobsMS2: 9.8060, H: 200, Phi: 20, Rho: 2.67}
	r1 := Reduce(base)
	base.Phi = 60
	r2 := Reduce(base)

	if r2.NormalGravityMGal == r1.NormalGravityMGal {
		t.Fatal("纬度改变后正常重力必须变化")
	}
	if r2.FreeAirMGal != r1.FreeAirMGal {
		t.Fatalf("仅改纬度，自由空气改正应不变：%v vs %v", r2.FreeAirMGal, r1.FreeAirMGal)
	}
	if r2.BouguerSlabMGal != r1.BouguerSlabMGal {
		t.Fatalf("仅改纬度，布格板改正应不变：%v vs %v", r2.BouguerSlabMGal, r1.BouguerSlabMGal)
	}
}

// 关系 5（重点）：自由空气符号方向。
// 一旦符号被取反（变成 -Δg_FA），所有高地测点的布格异常都会系统性下漂 2·Δg_FA。
func TestReduceFreeAirSignDirection(t *testing.T) {
	r := Reduce(Observation{GobsMS2: 9.8060, H: 300, Phi: 45, Rho: 2.67})

	// 带符号项：自由空气必须为正、布格板必须为负。
	if !(r.FreeAirSignedMGal > 0) {
		t.Fatalf("自由空气带符号项必须为正，实际 %v", r.FreeAirSignedMGal)
	}
	if !(r.BouguerSlabSignedMGal < 0) {
		t.Fatalf("布格板带符号项必须为负，实际 %v", r.BouguerSlabSignedMGal)
	}

	// 用独立重算的合成式交叉验证，抓住任何翻号实现：
	// Δg_Bouguer = gobs − γ + Δg_FA − Δg_B
	gobsMGal := units.MS2ToMGal(9.8060)
	want := gobsMGal - r.NormalGravityMGal + FreeAirCorrection(300) - BouguerSlabCorrection(2.67, 300)
	if !approxEq(r.BouguerAnomalyMGal, want, 1e-6) {
		t.Fatalf("布格异常合成符号错误：got %v want %v", r.BouguerAnomalyMGal, want)
	}

	// 回归哨兵：若自由空气被翻号，结果会比正确值低 2·Δg_FA。
	if wrong := gobsMGal - r.NormalGravityMGal - FreeAirCorrection(300) - BouguerSlabCorrection(2.67, 300); approxEq(r.BouguerAnomalyMGal, wrong, 1e-6) {
		t.Fatal("结果与“自由空气取反”的错误实现一致，符号被取反了")
	}
}

// 高程扫描必须逐点由真实公式算出，不能退化成写死的直线：
// 固定不同密度时，点列斜率应随之改变（布格板梯度不同）。
func TestReduceScanRealPerPoint(t *testing.T) {
	heights := []float64{0, 50, 100, 200, 400}

	base := Observation{GobsMS2: 9.8060, Phi: 45, Rho: 2.67}
	res := ReduceScan(base, heights)
	if len(res) != len(heights) {
		t.Fatalf("扫描点数不匹配：%d vs %d", len(res), len(heights))
	}

	// 每个点都必须等于对该高程单独做一次 Reduce 的结果。
	for i, h := range heights {
		p := base
		p.H = h
		one := Reduce(p)
		if !approxEq(res[i].BouguerAnomalyMGal, one.BouguerAnomalyMGal, 1e-9) {
			t.Fatalf("扫描第 %d 点 h=%v 与单点公式结果不一致", i, h)
		}
		if !approxEq(res[i].FreeAirMGal, FreeAirCorrection(h), eps) {
			t.Fatalf("扫描第 %d 点自由空气改正不正确", i)
		}
	}

	// 改变密度，扫描点列必须整体改变（写死直线无法通过）。
	other := base
	other.Rho = 3.00
	resOther := ReduceScan(other, heights)
	changed := false
	for i := range heights {
		if heights[i] > 0 && !approxEq(res[i].BouguerAnomalyMGal, resOther[i].BouguerAnomalyMGal, 1e-9) {
			changed = true
		}
	}
	if !changed {
		t.Fatal("改变密度后扫描点列未变化，疑似使用了与密度无关的写死直线")
	}
}
