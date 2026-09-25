package units

import (
	"math"
	"testing"
)

func TestConversions(t *testing.T) {
	if got := MS2ToMGal(1.0); math.Abs(got-100000.0) > 1e-9 {
		t.Fatalf("1 m/s² 应为 100000 mGal，实际 %v", got)
	}
	if got := MGalToMS2(1.0); math.Abs(got-1e-5) > 1e-12 {
		t.Fatalf("1 mGal 应为 1e-5 m/s²，实际 %v", got)
	}
	// 往返一致
	for _, a := range []float64{0, 9.78, 9.832, 12.3456} {
		if got := MGalToMS2(MS2ToMGal(a)); math.Abs(got-a) > 1e-12 {
			t.Fatalf("往返换算不一致：%v -> %v", a, got)
		}
	}
	// 常量本身要钉死，防止量纲悄悄改坏
	if MGalPerMS2 != 1.0/MS2PerMGal {
		t.Fatalf("mGal/m/s² 换算常量不互为倒数")
	}
}
