// Package validate 集中负责所有物理合理性与输入完备性校验，
// 并负责把 HTTP 层的请求 DTO 转换为 gravity.Observation。
// 任何不合物理的输入都在这里被明确拒绝，并带上原因。
package validate

import (
	"fmt"
	"math"
)

// FieldError 指明出错字段与原因，便于 HTTP 层返回带原因的错误信息。
type FieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Error 实现 error，消息为中文、面向调用方。
func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

// Limits 是物理合理性范围。
const (
	MinLatitudeDeg = -90.0
	MaxLatitudeDeg = 90.0
	// MinDensity 严格大于 0（中间层平均密度必须为正）。
	MinDensity = 0.0
)

// Inputs 是经过字段名规整、单位明确的内部输入。
type Inputs struct {
	GobsMS2 float64
	H       float64
	Phi     float64
	Rho     float64
}

// finite 判断是否为有限实数（拒绝 NaN 与 ±Inf）。
func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// Point 校验单点归算输入，返回 FieldError（首个发现的问题）。
func Point(in Inputs) error {
	if !finite(in.GobsMS2) {
		return FieldError{"gobs", "绝对重力观测值必须是有限实数，不能为 NaN 或无穷"}
	}
	if in.GobsMS2 <= 0 {
		return FieldError{"gobs", "绝对重力观测值必须为正数（m/s²）"}
	}
	if !finite(in.H) {
		return FieldError{"h", "测点高程必须是有限实数，不能为 NaN 或无穷"}
	}
	if !finite(in.Phi) {
		return FieldError{"phi", "地理纬度必须是有限实数，不能为 NaN 或无穷"}
	}
	if in.Phi < MinLatitudeDeg || in.Phi > MaxLatitudeDeg {
		return FieldError{"phi", fmt.Sprintf("地理纬度超出合理范围，必须在 [%g, %g] 度之间", MinLatitudeDeg, MaxLatitudeDeg)}
	}
	if !finite(in.Rho) {
		return FieldError{"rho", "中间层平均密度必须是有限实数，不能为 NaN 或无穷"}
	}
	if in.Rho <= MinDensity {
		return FieldError{"rho", "中间层平均密度必须为正数（g/cm³），不能为零或负"}
	}
	return nil
}

// Scan 校验高程扫描输入：固定参数同单点，且高程序列必须非空、元素有限。
func Scan(in Inputs, heights []float64, heightsPresent bool) error {
	if err := Point(in); err != nil {
		return err
	}
	if !heightsPresent {
		return FieldError{"heights", "缺失高程采样序列 heights"}
	}
	if len(heights) == 0 {
		return FieldError{"heights", "高程采样序列不能为空"}
	}
	for i, h := range heights {
		if !finite(h) {
			return FieldError{"heights", fmt.Sprintf("第 %d 个高程采样不是有限实数（不能为 NaN 或无穷）", i)}
		}
	}
	return nil
}
