package validate

import (
	"errors"
	"math"
	"testing"
)

func validInputs() Inputs {
	return Inputs{GobsMS2: 9.8060, H: 200, Phi: 45, Rho: 2.67}
}

func TestPointValid(t *testing.T) {
	if err := Point(validInputs()); err != nil {
		t.Fatalf("合法输入不应报错：%v", err)
	}
	// h=0 合法（测点就在参考面上）。
	in := validInputs()
	in.H = 0
	if err := Point(in); err != nil {
		t.Fatalf("h=0 应合法：%v", err)
	}
	// h 允许为负（低于参考面）。
	in.H = -50
	if err := Point(in); err != nil {
		t.Fatalf("h=-50 应合法：%v", err)
	}
	// 纬度边界 -90 / 90 合法。
	in = validInputs()
	in.Phi = -90
	if err := Point(in); err != nil {
		t.Fatalf("φ=-90 应合法：%v", err)
	}
	in.Phi = 90
	if err := Point(in); err != nil {
		t.Fatalf("φ=90 应合法：%v", err)
	}
}

func TestPointInvalid(t *testing.T) {
	cases := []struct {
		name  string
		mut   func(*Inputs)
		field string
	}{
		{"纬度超上限", func(i *Inputs) { i.Phi = 90.01 }, "phi"},
		{"纬度超下限", func(i *Inputs) { i.Phi = -91 }, "phi"},
		{"密度为零", func(i *Inputs) { i.Rho = 0 }, "rho"},
		{"密度为负", func(i *Inputs) { i.Rho = -1.2 }, "rho"},
		{"重力为零", func(i *Inputs) { i.GobsMS2 = 0 }, "gobs"},
		{"重力为负", func(i *Inputs) { i.GobsMS2 = -9.8 }, "gobs"},
		{"高程 NaN", func(i *Inputs) { i.H = math.NaN() }, "h"},
		{"纬度 Inf", func(i *Inputs) { i.Phi = math.Inf(1) }, "phi"},
		{"密度 NaN", func(i *Inputs) { i.Rho = math.NaN() }, "rho"},
		{"重力 Inf", func(i *Inputs) { i.GobsMS2 = math.Inf(-1) }, "gobs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInputs()
			tc.mut(&in)
			err := Point(in)
			if err == nil {
				t.Fatalf("非法输入未被拒绝（%s）", tc.name)
			}
			var fe FieldError
			if !errors.As(err, &fe) {
				t.Fatalf("应返回 FieldError，实际 %T", err)
			}
			if fe.Field != tc.field {
				t.Fatalf("错误字段应为 %s，实际 %s", tc.field, fe.Field)
			}
			if fe.Reason == "" {
				t.Fatal("错误必须带原因")
			}
		})
	}
}

func TestScanValidation(t *testing.T) {
	in := validInputs()
	if err := Scan(in, []float64{0, 100, 200}, true); err != nil {
		t.Fatalf("合法扫描不应报错：%v", err)
	}
	if err := Scan(in, nil, false); err == nil {
		t.Fatal("缺失 heights 必须被拒绝")
	}
	if err := Scan(in, []float64{}, true); err == nil {
		t.Fatal("空序列必须被拒绝")
	}
	if err := Scan(in, []float64{1, math.NaN()}, true); err == nil {
		t.Fatal("含 NaN 的序列必须被拒绝")
	}
	// 固定参数非法也要被拒（例如密度为负）。
	bad := in
	bad.Rho = -2
	if err := Scan(bad, []float64{1, 2}, true); err == nil {
		t.Fatal("固定参数非法时扫描必须被拒绝")
	}
}
