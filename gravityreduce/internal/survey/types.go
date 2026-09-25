// Package survey 在不改动单点归算物理链条的前提下，提供面向整条测线、
// 整张测网的批量归算与质量诊断：
//
//	逐点复用 gravity.Reduce → 按里程定序 → 沿测线水平梯度检查
//	                                      → 测线对交点差（几何求交 + 段内线性插值）
//	                                      → 线内布格异常统计画像
//
// 本包只做组织与诊断计算；单点物理公式仍在 gravity 包、单位换算仍在 units 包、
// HTTP 编解码仍在 api 包，各自职责不混。
package survey

import "gravityreduce/internal/gravity"

// Point 是测线上的一个测点：既携带单点归算所需的全部观测量，
// 又携带它在测区平面直角坐标系中的位置（东向、北向，单位米），
// 以及可选的沿线里程/桩号（单位米；nil 表示该线未提供里程）。
type Point struct {
	ID       string
	Easting  float64  // 东向坐标 E，米
	Northing float64  // 北向坐标 N，米
	Station  *float64 // 沿线里程/桩号，米；nil 表示未提供
	Obs      gravity.Observation
}

// Line 是一条测线：一串测点，顺序可以任意送入，诊断前由本包按里程理顺。
type Line struct {
	ID     string
	Points []Point
}

// Thresholds 是两类质控的判定阈值。
type Thresholds struct {
	// GradientMGalPerM 沿测线水平梯度可疑阈值，单位 mGal/m；|Δ异常/水平距离| 超过即标记。
	GradientMGalPerM float64
	// IntersectionTolMGal 交点差容许限，单位 mGal；|两线交点处异常插值之差| 超过即判超限。
	IntersectionTolMGal float64
}

// orderedPoint 是按里程排好序的测点，连同该点的真实归算结果。
type orderedPoint struct {
	Point  Point
	Result gravity.Result
}

// 线级诊断状态：
//
//	ok       —— 定序、梯度、交点诊断全部可用；
//	degraded —— 部分诊断撑不起来（缺里程、重复里程、单点、零距离退化等），
//	            已在 Notes 中逐条说明原因；逐点归算与统计画像始终照给。
const (
	StatusOK       = "ok"
	StatusDegraded = "degraded"
)

// GradientSegment 是排序后相邻两测点构成的一个诊断段。
type GradientSegment struct {
	FromPointID string
	ToPointID   string
	FromStation float64
	ToStation   float64
	// DistanceM 相邻测点水平距离，米。
	DistanceM float64
	// GradientMGalPerM 水平梯度 = (异常_To − 异常_From)/水平距离，mGal/m，带符号。
	GradientMGalPerM float64
	// ThresholdMGalPerM 判定所用阈值，mGal/m。
	ThresholdMGalPerM float64
	// Suspicious 是否超过阈值。
	Suspicious bool
	// ExceedsByMGalPerM 超阈量 = |梯度| − 阈值（未超阈时为 0），mGal/m。
	ExceedsByMGalPerM float64
}

// ZeroDistanceAnomaly 是同位置重复布点导致的退化数据异常：
// 两测点水平距离为零，梯度无定义，绝不能算成无穷大。
type ZeroDistanceAnomaly struct {
	LineID      string
	FromPointID string
	ToPointID   string
	FromStation float64
	ToStation   float64
	Reason      string
}

// GradientReport 汇总一条线的逐段梯度诊断。
type GradientReport struct {
	Available bool
	// ThresholdMGalPerM 回显判定阈值。
	ThresholdMGalPerM float64
	Segments          []GradientSegment
	// DataAnomalies 零距离等明确数据异常（与“梯度过大”性质不同，单列）。
	DataAnomalies []ZeroDistanceAnomaly
}

// Statistics 是一条线布格异常的整体统计画像，
// 全部由该线各点真实归算结果算出，不使用任何固定占位值。
type Statistics struct {
	Count    int
	MeanMGal float64
	// StdMGal 总体标准差 σ = sqrt(Σ(x−x̄)²/n)，mGal；单点时为 0。
	StdMGal    float64
	MinMGal    float64
	MinPointID string
	MaxMGal    float64
	MaxPointID string
}

// PointView 是一个测点的排序位次与归算结果。
type PointView struct {
	Point  Point
	Order  int
	Result gravity.Result
}

// LineResult 是一条测线的完整批量归算与诊断结果。
type LineResult struct {
	LineID string
	Status string
	// Notes 说明降级/异常原因（中文，面向内业人员）；状态 ok 时为空。
	Notes []string
	// Ordered 为按里程排好序的测点及归算结果；无法定序时按送入顺序给出并在 Notes 说明。
	Ordered []PointView
	// OrderingBasis 定序依据；目前只支持 "station_m"，无法定序时为 ""。
	OrderingBasis string
	Gradient      GradientReport
	Stats         Statistics
}

// Crossing 是两条测线的一个有效平面交点及其交点差。
type Crossing struct {
	Easting  float64
	Northing float64

	LineASegmentFromPointID string
	LineASegmentToPointID   string
	LineBSegmentFromPointID string
	LineBSegmentToPointID   string

	// InterpAMGal / InterpBMGal 分别为两条线在交点处对相邻测点布格异常
	// 做线性插值得到的异常估计，mGal。
	InterpAMGal float64
	InterpBMGal float64

	// DifferenceMGal 交点差 = A 线插值 − B 线插值，mGal，带符号。
	DifferenceMGal float64
	ToleranceMGal  float64
	// WithinTolerance 交点差是否在容许限内。
	WithinTolerance bool
}

// 线对诊断状态。
const (
	PairStatusIntersects      = "intersects"
	PairStatusNoValidCrossing = "no_valid_crossing"
)

// PairResult 是一对测线的交点差诊断结果。
// 几何上不相交、平行、共线、交点落在测量段之外等情形一律 Status=no_valid_crossing，
// 并在 Reason/ReasonDetail 中如实说明，绝不硬凑交点差。
type PairResult struct {
	LineAID      string
	LineBID      string
	Status       string
	Reason       string
	ReasonDetail string
	Crossings    []Crossing
}

// BatchResult 是一次批量归算（一条或多条测线）的完整结果。
type BatchResult struct {
	Thresholds Thresholds
	Lines      []LineResult
	Pairs      []PairResult
}
