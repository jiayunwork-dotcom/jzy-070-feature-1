package gravity

// 两项高程相关改正采用题目钉死的常用线性形式（结果单位均为 mGal）：
//
//	自由空气改正：Δg_FA = 0.3086 · h   （h 以米计，恒为正）
//	布格板改正：  Δg_B  = 0.04193 · ρ · h（ρ 以 g/cm³ 计，恒为正）
//
// 这两个函数返回的是“改正量的大小”，恒为非负；
// 它们在合成布格异常时的正负号在 reduction.go 中钉死，不得在别处翻号。
const (
	// FreeAirGradient 为自由空气梯度，mGal/米。
	FreeAirGradient = 0.3086
	// BouguerSlabCoefficient 为无限布格板公式系数（ρ 取 g/cm³、h 取米时），mGal/米/(g/cm³)。
	BouguerSlabCoefficient = 0.04193
)

// FreeAirCorrection 计算自由空气改正量的大小：Δg_FA = 0.3086·h。
// h 以米计，结果为 mGal。h=0 时结果必为 0；随 h 线性增大。
// 调用前应保证 h 为有限数（由 validate 包负责）。
func FreeAirCorrection(h float64) float64 {
	return FreeAirGradient * h
}

// BouguerSlabCorrection 计算无限布格板改正量的大小：Δg_B = 0.04193·ρ·h。
// rho 以 g/cm³ 计、h 以米计，结果为 mGal。
// h=0 或在合法范围内变化时满足线性关系；密度 rho 只影响这一项。
// 调用前应保证 rho > 0 且 h、rho 均为有限数（由 validate 包负责）。
func BouguerSlabCorrection(rho, h float64) float64 {
	return BouguerSlabCoefficient * rho * h
}

// TerrainCorrection 地形改正。本服务从简处理：一律取零（单位 mGal）。
func TerrainCorrection() float64 {
	return 0
}
