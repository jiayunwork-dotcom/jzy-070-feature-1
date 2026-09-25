package api

// linesRequest 面向测线/测网的批量归算与诊断请求。
// 标量物理字段沿用单点接口的指针用法，以区分“缺失”与“零值”
// （如 easting_m=0、station_m=0 都是合法输入，必须能与未提供区分）。
type linesRequest struct {
	GradientThresholdMGalPerKm *float64     `json:"gradient_threshold_mgal_per_km"`
	IntersectionToleranceMGal  *float64     `json:"intersection_tolerance_mgal"`
	Lines                      []lineReqDTO `json:"lines"`
}

type lineReqDTO struct {
	ID     string            `json:"id"`
	Points []linePointReqDTO `json:"points"`
}

type linePointReqDTO struct {
	ID   string   `json:"id"`
	Gobs *float64 `json:"gobs"`
	H    *float64 `json:"h"`
	Phi  *float64 `json:"phi"`
	Rho  *float64 `json:"rho"`
	// EastingM/NorthingM 平面直角坐标（东向、北向，米），必填。
	EastingM *float64 `json:"easting_m"`
	// NorthingM 北向坐标（米），必填。
	NorthingM *float64 `json:"northing_m"`
	// StationM 沿线里程/桩号（米），选填；整条线要么都给、要么都不给，
	// 缺失会被识别为“里程无法定序”的降级，而不是悄悄按输入顺序计算。
	StationM *float64 `json:"station_m"`
}

// ---- 响应视图 ----

type reasonView struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type linePointView struct {
	ID             string      `json:"id"`
	StationM       *float64    `json:"station_m,omitempty"`
	EastingM       float64     `json:"easting_m"`
	NorthingM      float64     `json:"northing_m"`
	HM             float64     `json:"h_m"`
	BouguerAnomaly gravityView `json:"bouguer_anomaly"`
}

type segmentView struct {
	FromID              string  `json:"from_point_id"`
	ToID                string  `json:"to_point_id"`
	FromStationM        float64 `json:"from_station_m"`
	ToStationM          float64 `json:"to_station_m"`
	HorizontalDistanceM float64 `json:"horizontal_distance_m"`
	AnomalyDeltaMGal    float64 `json:"anomaly_delta_mgal"`
	GradientMGalPerKm   float64 `json:"gradient_mgal_per_km"`
	ThresholdMGalPerKm  float64 `json:"threshold_mgal_per_km"`
	ExcessMGalPerKm     float64 `json:"excess_mgal_per_km"`
	Suspicious          bool    `json:"suspicious"`
	Kind                string  `json:"kind"`
}

type gradientView struct {
	Status          string        `json:"status"`
	SkipReason      *reasonView   `json:"skip_reason,omitempty"`
	Segments        []segmentView `json:"segments"`
	SuspiciousCount int           `json:"suspicious_count"`
	CoincidentCount int           `json:"coincident_count"`
}

type extremumView struct {
	PointID string  `json:"point_id"`
	MGal    float64 `json:"mgal"`
}

type statisticsView struct {
	PointCount int          `json:"point_count"`
	MeanMGal   float64      `json:"mean_mgal"`
	StdMGal    float64      `json:"std_mgal"`
	Min        extremumView `json:"min"`
	Max        extremumView `json:"max"`
}

type lineView struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"`
	DegradeReasons []reasonView    `json:"degrade_reasons,omitempty"`
	Points         []linePointView `json:"points"`
	Gradient       gradientView    `json:"gradient"`
	Statistics     statisticsView  `json:"statistics"`
}

type interpolatedSideView struct {
	LineID        string  `json:"line_id"`
	SegmentFromID string  `json:"segment_from_point_id"`
	SegmentToID   string  `json:"segment_to_point_id"`
	ParameterT    float64 `json:"parameter_t"`
	AnomalyMGal   float64 `json:"bouguer_anomaly_mgal"`
}

type pairView struct {
	LineAID string      `json:"line_a_id"`
	LineBID string      `json:"line_b_id"`
	Status  string      `json:"status"`
	Reason  *reasonView `json:"reason,omitempty"`
	// 以下字段仅在 status=intersected 时出现。用指针而不是 omitempty 标量：
	// 交点坐标或交点差本身可能恰为 0，0 是有效结果，绝不能被 omitempty 吞掉；
	// 只有“无有效交点”时这些字段才整体缺席。
	EastingM         *float64              `json:"easting_m,omitempty"`
	NorthingM        *float64              `json:"northing_m,omitempty"`
	A                *interpolatedSideView `json:"a,omitempty"`
	B                *interpolatedSideView `json:"b,omitempty"`
	DifferenceMGal   *float64              `json:"difference_mgal,omitempty"`
	ToleranceMGal    *float64              `json:"tolerance_mgal,omitempty"`
	ExceedsTolerance *bool                 `json:"exceeds_tolerance,omitempty"`
}

type linesResponse struct {
	Lines []lineView `json:"lines"`
	Pairs []pairView `json:"pairs"`
}
