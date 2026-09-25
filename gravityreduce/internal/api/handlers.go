package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"gravityreduce/internal/gravity"
	"gravityreduce/internal/sample"
	"gravityreduce/internal/validate"
)

const bouguerFormula = "Delta_g_Bouguer = gobs - gamma + Delta_g_FA - Delta_g_B (+ Delta_g_T=0)"

func signsMap() map[string]string {
	return map[string]string{
		"free_air":     "正（+Δg_FA，向参考面归算时重力随高程增大）",
		"bouguer_slab": "负（-Δg_B，扣除测点与参考面之间中间层物质的引力）",
		"terrain":      "零（本服务地形改正缺省为 0）",
	}
}

// errorBody 是统一错误响应。
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Field   string `json:"field,omitempty"`
	} `json:"error"`
}

func abortError(c *gin.Context, status int, field, msg string) {
	var b errorBody
	b.Error.Field = field
	b.Error.Message = msg
	c.AbortWithStatusJSON(status, b)
}

// abortFieldError 把校验错误翻译成 422。
func abortFieldError(c *gin.Context, err error) {
	var fe validate.FieldError
	if errors.As(err, &fe) {
		abortError(c, http.StatusUnprocessableEntity, fe.Field, fe.Reason)
		return
	}
	abortError(c, http.StatusBadRequest, "", err.Error())
}

// decodeStrict 解析 JSON：拒绝空体、非法 JSON 与未知字段，避免错误输入被静默吞掉。
func decodeStrict(c *gin.Context, dst any) error {
	if c.Request.Body == nil {
		return errors.New("请求体为空，需要 application/json")
	}
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("请求体中只能包含一个 JSON 对象")
	}
	return nil
}

func requireField(p *float64, name string) (float64, error) {
	if p == nil {
		return 0, validate.FieldError{Field: name, Reason: "缺失该必填字段"}
	}
	return *p, nil
}

// parsePointFields 抽取 gobs/h/phi/rho 四个必填字段（缺失即报错）。
func parsePointFields(req reduceRequest) (validate.Inputs, error) {
	var in validate.Inputs
	var err error
	if in.GobsMS2, err = requireField(req.Gobs, "gobs"); err != nil {
		return in, err
	}
	if in.H, err = requireField(req.H, "h"); err != nil {
		return in, err
	}
	if in.Phi, err = requireField(req.Phi, "phi"); err != nil {
		return in, err
	}
	if in.Rho, err = requireField(req.Rho, "rho"); err != nil {
		return in, err
	}
	return in, nil
}

func toGravityView(mgal, ms2 float64) gravityView {
	return gravityView{MGal: roundMGal(mgal), MS2: roundMS2(ms2)}
}

// Reduce 处理 POST /api/v1/reduce：单点归算。
func Reduce(c *gin.Context) {
	var req reduceRequest
	if err := decodeStrict(c, &req); err != nil {
		abortError(c, http.StatusBadRequest, "", "请求体不是合法 JSON："+err.Error())
		return
	}
	if req.Heights != nil {
		abortError(c, http.StatusBadRequest, "heights", "单点归算不接受 heights；高程扫描请调用 /api/v1/scan")
		return
	}

	in, err := parsePointFields(req)
	if err != nil {
		abortFieldError(c, err)
		return
	}
	if err := validate.Point(in); err != nil {
		abortFieldError(c, err)
		return
	}

	res := gravity.Reduce(gravity.Observation{
		GobsMS2: in.GobsMS2,
		H:       in.H,
		Phi:     in.Phi,
		Rho:     in.Rho,
	})

	var resp reduceResponse
	resp.Inputs.GobsMS2 = in.GobsMS2
	resp.Inputs.HM = in.H
	resp.Inputs.PhiDeg = in.Phi
	resp.Inputs.Rho = in.Rho
	resp.NormalGravity = toGravityView(res.NormalGravityMGal, res.NormalGravityMS2)
	resp.FreeAirCorrection = correctionView{
		MagnitudeMGal: roundMGal(res.FreeAirMGal),
		SignedMGal:    roundMGal(res.FreeAirSignedMGal),
	}
	resp.BouguerSlabCorrection = correctionView{
		MagnitudeMGal: roundMGal(res.BouguerSlabMGal),
		SignedMGal:    roundMGal(res.BouguerSlabSignedMGal),
	}
	resp.TerrainCorrectionMGal = roundMGal(res.TerrainMGal)
	resp.BouguerAnomaly = toGravityView(res.BouguerAnomalyMGal, res.BouguerAnomalyMS2)
	resp.Formula = bouguerFormula
	resp.Signs = signsMap()

	c.JSON(http.StatusOK, resp)
}

// Scan 处理 POST /api/v1/scan：高程扫描，逐点用真实公式计算布格异常。
func Scan(c *gin.Context) {
	var req reduceRequest
	if err := decodeStrict(c, &req); err != nil {
		abortError(c, http.StatusBadRequest, "", "请求体不是合法 JSON："+err.Error())
		return
	}
	if req.H != nil {
		abortError(c, http.StatusBadRequest, "h", "高程扫描不接受单点字段 h；请在 heights 数组中给出高程采样序列")
		return
	}

	in, err := parsePointFieldsIgnoreH(req)
	if err != nil {
		abortFieldError(c, err)
		return
	}

	var heights []float64
	present := req.Heights != nil
	if present {
		heights = *req.Heights
	}
	// 固定参数 + 序列非空、元素有限，一次性校验。
	if err := validate.Scan(in, heights, present); err != nil {
		abortFieldError(c, err)
		return
	}

	results := gravity.ReduceScan(gravity.Observation{
		GobsMS2: in.GobsMS2,
		Phi:     in.Phi,
		Rho:     in.Rho,
	}, heights)

	var resp scanResponse
	resp.Fixed.GobsMS2 = in.GobsMS2
	resp.Fixed.PhiDeg = in.Phi
	resp.Fixed.Rho = in.Rho
	resp.Formula = bouguerFormula
	resp.Points = make([]scanPoint, 0, len(results))
	for i, r := range results {
		resp.Points = append(resp.Points, scanPoint{
			HM:                    heights[i],
			FreeAirCorrectionMGal: roundMGal(r.FreeAirMGal),
			BouguerSlabMGal:       roundMGal(r.BouguerSlabMGal),
			BouguerAnomaly:        toGravityView(r.BouguerAnomalyMGal, r.BouguerAnomalyMS2),
		})
	}

	c.JSON(http.StatusOK, resp)
}

// parsePointFieldsIgnoreH 扫描接口只需要 gobs/phi/rho（h 来自 heights）。
func parsePointFieldsIgnoreH(req reduceRequest) (validate.Inputs, error) {
	var in validate.Inputs
	var err error
	if in.GobsMS2, err = requireField(req.Gobs, "gobs"); err != nil {
		return in, err
	}
	if in.Phi, err = requireField(req.Phi, "phi"); err != nil {
		return in, err
	}
	if in.Rho, err = requireField(req.Rho, "rho"); err != nil {
		return in, err
	}
	return in, nil
}

// Sample 处理 GET /api/v1/sample：返回内置示例，供手工验算。
func Sample(c *gin.Context) {
	res := gravity.Reduce(gravity.Observation(sample.Observation))

	var resp reduceResponse
	resp.Inputs.GobsMS2 = sample.Observation.GobsMS2
	resp.Inputs.HM = sample.Observation.H
	resp.Inputs.PhiDeg = sample.Observation.Phi
	resp.Inputs.Rho = sample.Observation.Rho
	resp.NormalGravity = toGravityView(res.NormalGravityMGal, res.NormalGravityMS2)
	resp.FreeAirCorrection = correctionView{
		MagnitudeMGal: roundMGal(res.FreeAirMGal),
		SignedMGal:    roundMGal(res.FreeAirSignedMGal),
	}
	resp.BouguerSlabCorrection = correctionView{
		MagnitudeMGal: roundMGal(res.BouguerSlabMGal),
		SignedMGal:    roundMGal(res.BouguerSlabSignedMGal),
	}
	resp.TerrainCorrectionMGal = roundMGal(res.TerrainMGal)
	resp.BouguerAnomaly = toGravityView(res.BouguerAnomalyMGal, res.BouguerAnomalyMS2)
	resp.Formula = bouguerFormula
	resp.Signs = signsMap()

	c.JSON(http.StatusOK, gin.H{
		"description": sample.Description,
		"reduction":   resp,
	})
}

// Health 处理 GET /healthz。
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gravity-reduction"})
}
