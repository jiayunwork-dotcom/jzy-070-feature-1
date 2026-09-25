package survey

import (
	"errors"
	"fmt"
	"math"

	"gravityreduce/internal/validate"
)

// fieldErr 复用既有 validate.FieldError 的错误返回风格（带字段名与中文原因）。
func fieldErr(field, reason string) error {
	return validate.FieldError{Field: field, Reason: reason}
}

func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// ValidateRequest 做批量请求的结构与物理合理性校验。
// 物理量（gobs/h/phi/rho）逐点复用既有的 validate.Point 规则，
// 这里只额外负责测线组织所需的完备性：至少一条线、线/点 ID、平面坐标、里程、阈值容差。
//
// 注意：单点测线、里程缺失属于“能归算但诊断需降级”的数据情形，
// 不在这里拒绝，而在处理结果中带原因降级；这里拒绝的是根本无法处理或
// 会让结果无意义的输入（无点、无坐标、非有限数、重复 ID、重复里程等）。
func ValidateRequest(req Request) error {
	if len(req.Lines) == 0 {
		return fieldErr("lines", "至少需要一条测线")
	}
	if !isFinite(req.GradientThresholdMGalPerKm) || req.GradientThresholdMGalPerKm <= 0 {
		return fieldErr("gradient_threshold_mgal_per_km", "水平梯度阈值必须为正的有限实数（mGal/km）")
	}
	if !isFinite(req.IntersectionToleranceMGal) || req.IntersectionToleranceMGal < 0 {
		return fieldErr("intersection_tolerance_mgal", "交点差容差必须为非负的有限实数（mGal）")
	}

	seenLineIDs := make(map[string]bool, len(req.Lines))
	for li := range req.Lines {
		line := &req.Lines[li]
		lf := fmt.Sprintf("lines[%d]", li)
		if line.ID == "" {
			return fieldErr(lf+".id", "测线 ID 缺失，必须给出唯一非空标识")
		}
		if seenLineIDs[line.ID] {
			return fieldErr(lf+".id", fmt.Sprintf("测线 ID %q 重复，无法区分测线", line.ID))
		}
		seenLineIDs[line.ID] = true

		if len(line.Points) == 0 {
			return fieldErr(lf+".points", fmt.Sprintf("测线 %q 没有任何测点", line.ID))
		}

		seenPointIDs := make(map[string]bool, len(line.Points))
		for pi := range line.Points {
			p := &line.Points[pi]
			pf := fmt.Sprintf("%s.points[%d]", lf, pi)
			if p.ID == "" {
				return fieldErr(pf+".id", fmt.Sprintf("测线 %q 中存在无 ID 的测点，诊断结果必须能指认到具体测点", line.ID))
			}
			if seenPointIDs[p.ID] {
				return fieldErr(pf+".id", fmt.Sprintf("测线 %q 中测点 ID %q 重复", line.ID, p.ID))
			}
			seenPointIDs[p.ID] = true

			// 物理量逐点复用既有单点校验（同一套物理合理性规则，不在此重写）。
			if err := validate.Point(validate.Inputs{
				GobsMS2: p.GobsMS2,
				H:       p.H,
				Phi:     p.Phi,
				Rho:     p.Rho,
			}); err != nil {
				var fe validate.FieldError
				if errors.As(err, &fe) {
					return fieldErr(pf+"."+fe.Field, fe.Reason)
				}
				return fieldErr(pf, err.Error())
			}

			if !isFinite(p.EastingM) {
				return fieldErr(pf+".easting_m", "测点东向坐标必须为有限实数（不能为 NaN/Inf），平面位置是测线组织与求交的必需输入")
			}
			if !isFinite(p.NorthingM) {
				return fieldErr(pf+".northing_m", "测点北向坐标必须为有限实数（不能为 NaN/Inf），平面位置是测线组织与求交的必需输入")
			}
			if p.HasStation && !isFinite(p.StationM) {
				return fieldErr(pf+".station_m", "测点里程必须为有限实数（不能为 NaN/Inf）")
			}
		}
	}
	return nil
}
