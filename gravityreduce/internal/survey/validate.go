// 输入完备性与合理性校验：测线级入口必须把撑不起诊断的输入在此明确拒绝，
// 不让缺坐标、非正观测值、重复编号等问题流入后面的几何与统计计算。
// 单点物理合理性直接复用既有 validate.Point，不重写规则。
package survey

import (
	"fmt"
	"math"

	"gravityreduce/internal/validate"
)

// FieldError 指明出错字段（含所属测线/测点定位）与原因，供 HTTP 层翻译成 422。
// 与 validate.FieldError 同构；此处单独定义以避免 survey 包对外暴露 api 层校验细节。
type FieldError struct {
	Field  string
	Reason string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func finite(x float64) bool {
	return !math.IsNaN(x) && !math.IsInf(x, 0)
}

// Validate 校验整批测线输入。返回的错误为 FieldError，Field 形如
// "lines[2].points[1].easting"，便于调用方定位具体测点。
//
// 注意：缺里程、重复里程、单点测线属于“可降级诊断”的数据情形，不在这里拒绝
// （由 Process 在线级结果中带原因降级）；这里只拒绝无法安全进入处理的硬错误。
func Validate(lines []Line, th Thresholds) error {
	if !finite(th.GradientMGalPerM) {
		return FieldError{"gradient_threshold_mgal_per_m", "梯度阈值必须是有限实数，不能为 NaN 或无穷"}
	}
	if th.GradientMGalPerM <= 0 {
		return FieldError{"gradient_threshold_mgal_per_m", "梯度阈值必须为正数（mGal/m）"}
	}
	if !finite(th.IntersectionTolMGal) {
		return FieldError{"intersection_tolerance_mgal", "交点差容许限必须是有限实数，不能为 NaN 或无穷"}
	}
	if th.IntersectionTolMGal < 0 {
		return FieldError{"intersection_tolerance_mgal", "交点差容许限不能为负数（mGal）"}
	}
	if len(lines) == 0 {
		return FieldError{"lines", "至少需要一条测线"}
	}

	lineIDs := make(map[string]int, len(lines))
	for li, line := range lines {
		lf := fmt.Sprintf("lines[%d]", li)
		if line.ID == "" {
			return FieldError{lf + ".id", fmt.Sprintf("第 %d 条测线缺少必填字段 id", li)}
		}
		if first, dup := lineIDs[line.ID]; dup {
			return FieldError{lf + ".id", fmt.Sprintf("测线编号 %q 与第 %d 条测线重复，测线编号必须唯一", line.ID, first)}
		}
		lineIDs[line.ID] = li

		if len(line.Points) == 0 {
			return FieldError{lf + ".points", fmt.Sprintf("测线 %q 没有任何测点", line.ID)}
		}

		pointIDs := make(map[string]int, len(line.Points))
		stationCount := 0
		for pi, p := range line.Points {
			pf := fmt.Sprintf("%s.points[%d]", lf, pi)
			if p.ID == "" {
				return FieldError{pf + ".id", fmt.Sprintf("测线 %q 第 %d 个测点缺少必填字段 id", line.ID, pi)}
			}
			if first, dup := pointIDs[p.ID]; dup {
				return FieldError{pf + ".id", fmt.Sprintf("测线 %q 内测点编号 %q 与第 %d 个测点重复，测点编号在线内必须唯一", line.ID, p.ID, first)}
			}
			pointIDs[p.ID] = pi

			if !finite(p.Easting) {
				return FieldError{pf + ".easting", fmt.Sprintf("测点 %q 缺失东向坐标或其值不是有限实数（平面位置必填，单位米）", p.ID)}
			}
			if !finite(p.Northing) {
				return FieldError{pf + ".northing", fmt.Sprintf("测点 %q 缺失北向坐标或其值不是有限实数（平面位置必填，单位米）", p.ID)}
			}
			if p.Station != nil {
				if !finite(*p.Station) {
					return FieldError{pf + ".station_m", fmt.Sprintf("测点 %q 的里程/桩号不是有限实数（不能为 NaN 或无穷）", p.ID)}
				}
				stationCount++
			}

			// 复用单点物理校验规则；字段路径落到具体测点上。
			if err := validate.Point(validate.Inputs{
				GobsMS2: p.Obs.GobsMS2,
				H:       p.Obs.H,
				Phi:     p.Obs.Phi,
				Rho:     p.Obs.Rho,
			}); err != nil {
				return mapPointFieldError(pf, p.ID, err)
			}
		}

		// 里程要么全给、要么全不给：只给一部分无法可靠定序，属于硬错误而非降级。
		if stationCount != 0 && stationCount != len(line.Points) {
			return FieldError{
				Field:  lf + ".points.station_m",
				Reason: fmt.Sprintf("测线 %q 的里程/桩号只标注了 %d/%d 个测点；定序要求全部测点都给里程，或全部不给", line.ID, stationCount, len(line.Points)),
			}
		}
	}
	return nil
}

// mapPointFieldError 把单点校验错误重新挂到所属测点字段路径上，
// 例如 gobs → lines[0].points[2].gobs。
func mapPointFieldError(pf, pointID string, err error) error {
	fe, ok := err.(validate.FieldError)
	if !ok {
		return FieldError{pf, fmt.Sprintf("测点 %q：%v", pointID, err)}
	}
	return FieldError{Field: pf + "." + fe.Field, Reason: fe.Reason}
}
