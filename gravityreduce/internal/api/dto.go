// Package api 负责 HTTP 路由、JSON 编解码与错误返回，
// 不含物理公式（在 gravity 包）与校验规则（在 validate 包）。
package api

import "math"

// reduceRequest 单点归算请求。字段用指针以区分“缺失”与“零值”，
// 例如 h=0 是合法输入，必须和未提供 h 区分开。
type reduceRequest struct {
	Gobs    *float64   `json:"gobs"`
	H       *float64   `json:"h"`
	Phi     *float64   `json:"phi"`
	Rho     *float64   `json:"rho"`
	Heights *[]float64 `json:"heights"`
}

// correctionView 同时给出改正量“大小”（恒非负）和代入公式时的“带符号项”。
type correctionView struct {
	MagnitudeMGal float64 `json:"magnitude_mgal"`
	SignedMGal    float64 `json:"signed_mgal"`
}

type gravityView struct {
	MGal float64 `json:"mgal"`
	MS2  float64 `json:"m_s2"`
}

// reduceResponse 单点归算响应。四个核心量齐全：
// 正常重力、自由空气改正、布格板改正、布格异常。
type reduceResponse struct {
	Inputs struct {
		GobsMS2 float64 `json:"gobs_m_s2"`
		HM      float64 `json:"h_m"`
		PhiDeg  float64 `json:"phi_deg"`
		Rho     float64 `json:"rho_g_cm3"`
	} `json:"inputs"`
	NormalGravity         gravityView       `json:"normal_gravity"`
	FreeAirCorrection     correctionView    `json:"free_air_correction"`
	BouguerSlabCorrection correctionView    `json:"bouguer_slab_correction"`
	TerrainCorrectionMGal float64           `json:"terrain_correction_mgal"`
	BouguerAnomaly        gravityView       `json:"bouguer_anomaly"`
	Formula               string            `json:"formula"`
	Signs                 map[string]string `json:"signs"`
}

// scanPoint 高程扫描中的单个点，全部由真实公式逐点算出。
type scanPoint struct {
	HM                    float64     `json:"h_m"`
	FreeAirCorrectionMGal float64     `json:"free_air_correction_mgal"`
	BouguerSlabMGal       float64     `json:"bouguer_slab_correction_mgal"`
	BouguerAnomaly        gravityView `json:"bouguer_anomaly"`
}

type scanResponse struct {
	Fixed struct {
		GobsMS2 float64 `json:"gobs_m_s2"`
		PhiDeg  float64 `json:"phi_deg"`
		Rho     float64 `json:"rho_g_cm3"`
	} `json:"fixed"`
	Formula string      `json:"formula"`
	Points  []scanPoint `json:"points"`
}

// roundMGal 把 mGal 量值规整到 6 位小数（亚 µGal 级），避免 JSON 出现二进制浮点尾差。
func roundMGal(x float64) float64 {
	return math.Round(x*1e6) / 1e6
}

// roundMS2 把 m/s² 量值规整到 11 位小数；因为 1 mGal = 1e-5 m/s²，
// m/s² 侧要保留与 mGal 相同的分辨率需要 11 位小数，不能也只留 6 位。
func roundMS2(x float64) float64 {
	return math.Round(x*1e11) / 1e11
}
