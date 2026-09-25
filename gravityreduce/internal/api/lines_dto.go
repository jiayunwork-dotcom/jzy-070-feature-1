package api

// linesRequest 是批量测线归算与质控请求。
// 数值字段沿用 reduceRequest 的指针约定以区分“缺失”与“零值”：
// 平面坐标与单点观测量必填，里程/桩号可选（但一条线必须全给或全不给）。
type linesRequest struct {
	GradientThresholdMGalPerM *float64           `json:"gradient_threshold_mgal_per_m"`
	IntersectionTolMGal       *float64           `json:"intersection_tolerance_mgal"`
	Lines                     []linesRequestLine `json:"lines"`
}

type linesRequestLine struct {
	ID     string              `json:"id"`
	Points []linesRequestPoint `json:"points"`
}

type linesRequestPoint struct {
	ID       string   `json:"id"`
	Easting  *float64 `json:"easting_m"`
	Northing *float64 `json:"northing_m"`
	Station  *float64 `json:"station_m"`
	Gobs     *float64 `json:"gobs"`
	H        *float64 `json:"h"`
	Phi      *float64 `json:"phi"`
	Rho      *float64 `json:"rho"`
}
