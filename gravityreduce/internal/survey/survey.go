// Package survey 在不改动单点归算物理链条的前提下，提供面向整条测线、整张测网的
// 批量归算与质量诊断：
//
//	测点组织与按里程排序  →  逐点复用 gravity.Reduce 得布格异常
//	→  沿测线水平梯度诊断（含同位置重复布点的零距离退化）
//	→  测线两两平面交点求解与交点处线性插值、交点差
//	→  每条测线的布格异常统计画像
//
// 本包只做领域计算与质量诊断，不含 HTTP 编解码（在 api 包）；
// 单点物理公式仍全部在 gravity 包，本包只调用、不重写。
package survey

// PointInput 是一个测点的全部输入（内部表示，由 HTTP 层 DTO 转换而来）。
// 平面位置 EastingM/NorthingM 为测区平面直角坐标（东向、北向，米），
// 只用于测线几何（排序后相邻距离、求交、插值），不参与任何物理归算。
type PointInput struct {
	ID      string
	GobsMS2 float64
	H       float64
	Phi     float64
	Rho     float64
	// EastingM/NorthingM 为测点平面坐标（米），必填。
	EastingM  float64
	NorthingM float64
	// StationM 为沿线里程/桩号（米），HasStation=false 表示该点未标里程。
	StationM   float64
	HasStation bool
}

// LineInput 是一条测线：一串带平面位置（最好还带里程）的测点。
type LineInput struct {
	ID     string
	Points []PointInput
}

// Request 是一次面向测线/测网的批量归算与诊断请求。
type Request struct {
	Lines []LineInput
	// GradientThresholdMGalPerKm 为水平梯度可疑段阈值，单位 mGal/km，必须为正。
	GradientThresholdMGalPerKm float64
	// IntersectionToleranceMGal 为交点差容差，单位 mGal，必须非负。
	IntersectionToleranceMGal float64
}

// Reason 是一处降级/跳过/无交点的结构化原因（机器可读代码 + 中文说明）。
type Reason struct {
	Code    string
	Message string
}

// 降级原因代码。
const (
	// ReasonSinglePoint 单点测线：可以归算与统计，但撑不起梯度与交点诊断。
	ReasonSinglePoint = "single_point"
	// ReasonStationMissing 存在未标里程的测点，整条线无法可靠定先后次序。
	ReasonStationMissing = "station_missing"
	// ReasonStationDuplicate 存在重复里程，测点先后次序不唯一。
	ReasonStationDuplicate = "station_duplicate"
)

// 交点配对的无交点原因代码。
const (
	// ReasonLineDegraded 至少一条测线处于降级状态，不满足交点插值条件。
	ReasonLineDegraded = "line_degraded"
	// ReasonZeroLengthLine 测线各测点平面重合、无有效走向。
	ReasonZeroLengthLine = "zero_length_line"
	// ReasonParallel 两线走向平行（不共线），几何上无交点。
	ReasonParallel = "parallel"
	// ReasonCollinear 两线共线，交点不唯一，不能硬凑唯一交点差。
	ReasonCollinear = "collinear"
	// ReasonOutOfRange 无限延长线虽相交，但交点落在至少一条线的实测段之外。
	ReasonOutOfRange = "intersection_out_of_range"
)

// 测线/配对的状态取值。
const (
	StatusOK             = "ok"
	StatusDegraded       = "degraded"
	StatusIntersected    = "intersected"
	StatusNoIntersection = "no_intersection"
)

func reasonFor(code string) Reason {
	switch code {
	case ReasonSinglePoint:
		return Reason{Code: code, Message: "单点测线：已完成逐点归算与统计，但不足两点，无法做梯度与交点诊断"}
	case ReasonStationMissing:
		return Reason{Code: code, Message: "存在未标里程（station_m）的测点，无法可靠确定沿线先后次序，梯度与交点诊断已降级跳过"}
	case ReasonStationDuplicate:
		return Reason{Code: code, Message: "存在重复里程（station_m），测点先后次序不唯一，梯度与交点诊断已降级跳过"}
	case ReasonLineDegraded:
		return Reason{Code: code, Message: "参与配对的测线处于降级状态（单点或里程无法定序），不具备交点插值条件"}
	case ReasonZeroLengthLine:
		return Reason{Code: code, Message: "测线各测点平面位置重合，没有有效走向，无法求交"}
	case ReasonParallel:
		return Reason{Code: code, Message: "两条测线走向平行，几何上不相交，无有效交点"}
	case ReasonCollinear:
		return Reason{Code: code, Message: "两条测线共线，交点不唯一，不产生唯一交点差"}
	case ReasonOutOfRange:
		return Reason{Code: code, Message: "两条线的延长线交点落在至少一条线的实测段范围之外，无有效交点"}
	default:
		return Reason{Code: code, Message: code}
	}
}

// PointResult 是一个测点的归算结果在测线语境下的表示。
type PointResult struct {
	ID                 string
	StationM           float64
	HasStation         bool
	EastingM           float64
	NorthingM          float64
	HM                 float64
	BouguerAnomalyMGal float64
	BouguerAnomalyMS2  float64
}

// 梯度段类型。
const (
	// SegmentNormal 正常段：水平距离非零，已算出水平梯度。
	SegmentNormal = "normal"
	// SegmentCoincident 退化段：两端点平面位置重合（零距离），
	// 梯度无定义，作为明确的数据异常报出，绝不产生 Inf。
	SegmentCoincident = "coincident"
)

// SegmentResult 是排序后相邻两测点之间一段的梯度诊断结果。
type SegmentResult struct {
	FromID              string
	ToID                string
	FromStationM        float64
	ToStationM          float64
	HorizontalDistanceM float64
	// AnomalyDeltaMGal 为 to−from 的布格异常差（mGal，带符号）。
	AnomalyDeltaMGal float64
	// GradientMGalPerKm 为水平梯度（mGal/km，带符号）；零距离段为 0。
	GradientMGalPerKm  float64
	ThresholdMGalPerKm float64
	// ExcessMGalPerKm 为梯度绝对值超阈值的量（mGal/km），未超为 0。
	ExcessMGalPerKm float64
	Suspicious      bool
	Kind            string
}

// GradientReport 是一条线的沿测线梯度诊断。
type GradientReport struct {
	// Status 为 "ok" 或 "skipped"。
	Status          string
	SkipReason      Reason
	Segments        []SegmentResult
	SuspiciousCount int
	// CoincidentCount 为同位置重复布点（零距离）退化段的数量。
	CoincidentCount int
}

// Extremum 指明最大/最小异常值及其测点。
type Extremum struct {
	PointID   string
	ValueMGal float64
}

// Statistics 是一条线布格异常的整体统计画像，全部由该线各点
// 真实归算结果算出，绝不使用固定值或近似占位。
type Statistics struct {
	PointCount int
	MeanMGal   float64
	// StdMGal 为样本标准差（n−1，n≥2）；单点时为 0。
	StdMGal float64
	Min     Extremum
	Max     Extremum
}

// LineReport 是一条测线的完整结果：归算点列、梯度诊断、统计。
type LineReport struct {
	ID             string
	Status         string // "ok" | "degraded"
	DegradeReasons []Reason
	// Points 在可定序时按里程升序排列；降级时保留输入顺序。
	Points     []PointResult
	Gradient   GradientReport
	Statistics Statistics
}

// InterpolatedSide 是一条线在交点处的插值结果及所用测点段。
type InterpolatedSide struct {
	LineID        string
	SegmentFromID string
	SegmentToID   string
	// ParameterT 为交点在该测点段上的线性参数（0=起点，1=终点）。
	ParameterT  float64
	AnomalyMGal float64
}

// PairReport 是两条测线一对的交点差结果。
type PairReport struct {
	LineAID string
	LineBID string
	// Status 为 "intersected" 或 "no_intersection"。
	Status string
	// Reason 仅在 no_intersection 时有值，如实说明为何不产生交点差。
	Reason Reason
	// 以下字段仅在 intersected 时有意义。
	EastingM  float64
	NorthingM float64
	A         *InterpolatedSide
	B         *InterpolatedSide
	// DifferenceMGal 为 A−B 的交点处布格异常差（mGal，带符号）。
	DifferenceMGal   float64
	ToleranceMGal    float64
	ExceedsTolerance bool
}

// Report 是一次批量归算与诊断的完整结果。
type Report struct {
	Lines []LineReport
	Pairs []PairReport
}
