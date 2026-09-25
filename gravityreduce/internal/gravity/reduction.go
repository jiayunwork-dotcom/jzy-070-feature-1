package gravity

import "gravityreduce/internal/units"

// Observation 是一次单点归算所需的全部原始观测输入。
type Observation struct {
	// GobsMS2 为测点绝对重力观测值，单位 m/s²（SI）。
	GobsMS2 float64
	// H 为测点高程，单位米（相对参考面/椭球面）。
	H float64
	// Phi 为测点地理（大地）纬度，单位度。
	Phi float64
	// Rho 为测点下方中间层的平均密度，单位 g/cm³。
	Rho float64
}

// Result 是一次归算的完整结果，所有带 MGal 后缀的量单位均为 mGal，
// 带 MS2 后缀的量单位均为 m/s²。两类单位分别成组，绝不混加。
type Result struct {
	GobsMGal float64
	GobsMS2  float64

	NormalGravityMGal float64
	NormalGravityMS2  float64

	// FreeAirMGal 自由空气改正“量值”，恒 ≥ 0；合成时取正号（见 +FreeAirSignedMGal）。
	FreeAirMGal float64
	// BouguerSlabMGal 布格板改正“量值”，恒 ≥ 0；合成时取负号（见 +BouguerSlabSignedMGal）。
	BouguerSlabMGal float64
	// TerrainMGal 地形改正，本服务固定为 0。
	TerrainMGal float64

	// FreeAirSignedMGal 与 BouguerSlabSignedMGal 是代入合成公式时的带符号项：
	// 自由空气项恒为正、布格板项恒为负，二者符号相反、缺一不可。
	FreeAirSignedMGal     float64
	BouguerSlabSignedMGal float64
	TerrainSignedMGal     float64

	BouguerAnomalyMGal float64
	BouguerAnomalyMS2  float64
}

// Reduce 按固定物理链条完成一次单点归算。
//
//	Δg_Bouguer = gobs − γ + Δg_FA − Δg_B (+ Δg_T)
//
// 其中 gobs、γ 先统一换算为 mGal；Δg_FA、Δg_B 本身就是 mGal；
// 地形改正 Δg_T 在本服务缺省取 0。
//
// 本函数假定输入已通过 validate 包校验；物理正确性所需的符号关系在此钉死。
func Reduce(in Observation) Result {
	gobsMGal := units.MS2ToMGal(in.GobsMS2)

	gammaMS2 := NormalGravity(in.Phi)
	gammaMGal := units.MS2ToMGal(gammaMS2)

	faMGal := FreeAirCorrection(in.H)
	bouguerMGal := BouguerSlabCorrection(in.Rho, in.H)
	terrainMGal := TerrainCorrection()

	// 钉死符号：自由空气取正、布格板取负、二者相反，绝不可省任一项。
	bouguerAnomalyMGal := gobsMGal - gammaMGal + faMGal - bouguerMGal + terrainMGal

	return Result{
		GobsMGal:              gobsMGal,
		GobsMS2:               in.GobsMS2,
		NormalGravityMGal:     gammaMGal,
		NormalGravityMS2:      gammaMS2,
		FreeAirMGal:           faMGal,
		BouguerSlabMGal:       bouguerMGal,
		TerrainMGal:           terrainMGal,
		FreeAirSignedMGal:     faMGal,
		BouguerSlabSignedMGal: -bouguerMGal,
		TerrainSignedMGal:     terrainMGal,
		BouguerAnomalyMGal:    bouguerAnomalyMGal,
		BouguerAnomalyMS2:     units.MGalToMS2(bouguerAnomalyMGal),
	}
}

// ReduceScan 固定其余参数，对给定高程序列逐点用真实公式计算布格异常。
// heights 单位米；返回切片与 heights 等长且一一对应，绝不用写死的直线代替。
// 本函数假定 heights 非空且各元素有限、其余输入已通过 validate 包校验。
func ReduceScan(in Observation, heights []float64) []Result {
	out := make([]Result, len(heights))
	for i, h := range heights {
		point := in
		point.H = h
		out[i] = Reduce(point)
	}
	return out
}
