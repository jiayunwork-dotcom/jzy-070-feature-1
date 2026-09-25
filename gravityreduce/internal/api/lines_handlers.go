package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gravityreduce/internal/survey"
)

// ReduceLines 处理 POST /api/v1/lines/reduce：
// 一条或多条测线的批量归算 + 沿测线梯度诊断 + 测线两两交点差 + 线内统计。
//
// 单点物理链条完全复用既有 /reduce 同一套 gravity 包；本处理器只负责
// 请求解析、字段完备性检查、调用 survey 领域层与响应组装。
func ReduceLines(c *gin.Context) {
	var req linesRequest
	if err := decodeStrict(c, &req); err != nil {
		abortError(c, http.StatusBadRequest, "", "请求体不是合法 JSON："+err.Error())
		return
	}

	in, err := parseLinesRequest(req)
	if err != nil {
		abortFieldError(c, err)
		return
	}
	if err := survey.ValidateRequest(in); err != nil {
		abortFieldError(c, err)
		return
	}

	rep := survey.Process(in)
	c.JSON(http.StatusOK, buildLinesResponse(rep))
}

// parseLinesRequest 把 HTTP DTO 转成领域请求，负责区分“缺失字段”与“零值”，
// 字段路径定位到具体线/点，与既有错误返回风格一致。
func parseLinesRequest(req linesRequest) (survey.Request, error) {
	var in survey.Request

	if req.GradientThresholdMGalPerKm == nil {
		return in, fieldErrAPI("gradient_threshold_mgal_per_km", "缺失水平梯度阈值（mGal/km）")
	}
	in.GradientThresholdMGalPerKm = *req.GradientThresholdMGalPerKm
	if req.IntersectionToleranceMGal == nil {
		return in, fieldErrAPI("intersection_tolerance_mgal", "缺失交点差容差（mGal）")
	}
	in.IntersectionToleranceMGal = *req.IntersectionToleranceMGal

	if len(req.Lines) == 0 {
		return in, fieldErrAPI("lines", "至少需要一条测线")
	}

	in.Lines = make([]survey.LineInput, len(req.Lines))
	for li, lr := range req.Lines {
		lf := lineField(li)
		if lr.ID == "" {
			return in, fieldErrAPI(lf+".id", "测线 ID 缺失，必须给出唯一非空标识")
		}
		if len(lr.Points) == 0 {
			return in, fieldErrAPI(lf+".points", "测线没有任何测点")
		}

		line := survey.LineInput{ID: lr.ID, Points: make([]survey.PointInput, 0, len(lr.Points))}
		for pi, pr := range lr.Points {
			pf := pointField(li, pi)
			if pr.ID == "" {
				return in, fieldErrAPI(pf+".id", "测点 ID 缺失，诊断结果必须能指认到具体测点")
			}
			p, err := parseLinePoint(pf, pr)
			if err != nil {
				return in, err
			}
			line.Points = append(line.Points, p)
		}
		in.Lines[li] = line
	}
	return in, nil
}

// parseLinePoint 解析单个测点：四个物理量 + 两个平面坐标必填；里程选填。
func parseLinePoint(pf string, pr linePointReqDTO) (survey.PointInput, error) {
	var p survey.PointInput
	p.ID = pr.ID

	var err error
	if p.GobsMS2, err = requireField(pr.Gobs, pf+".gobs"); err != nil {
		return p, err
	}
	if p.H, err = requireField(pr.H, pf+".h"); err != nil {
		return p, err
	}
	if p.Phi, err = requireField(pr.Phi, pf+".phi"); err != nil {
		return p, err
	}
	if p.Rho, err = requireField(pr.Rho, pf+".rho"); err != nil {
		return p, err
	}
	if p.EastingM, err = requireField(pr.EastingM, pf+".easting_m"); err != nil {
		return p, err
	}
	if p.NorthingM, err = requireField(pr.NorthingM, pf+".northing_m"); err != nil {
		return p, err
	}
	if pr.StationM != nil {
		p.StationM = *pr.StationM
		p.HasStation = true
	}
	return p, nil
}

func lineField(li int) string { return "lines[" + itoa(li) + "]" }
func pointField(li, pi int) string {
	return lineField(li) + ".points[" + itoa(pi) + "]"
}
