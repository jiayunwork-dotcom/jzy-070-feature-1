// Package sample 提供一份可供手工验算的内置示例数据。
package sample

import "gravityreduce/internal/gravity"

// Description 说明示例测点与人工核对要点。
const Description = `中纬度示例测点：纬度 45°N、高程 200 m、中间层平均密度 2.67 g/cm³。
人工核对（mGal）：
  Δg_FA = 0.3086 × 200        = 61.72
  Δg_B  = 0.04193 × 2.67 × 200 ≈ 22.39
  gobs  = 9.80600000 m/s²      = 980600.00 mGal
  γ(45°) 见 GRS80 正常重力公式输出（约 980619.0 mGal）
  Δg_Bouguer = 980600.00 − γ + 61.72 − 22.39`

// Observation 是示例测点。取中纬度、几百米高程，使两项改正落在明显的 mGal 量级。
var Observation = gravity.Observation{
	GobsMS2: 9.80600000, // 980600.00 mGal
	H:       200.0,      // 米
	Phi:     45.0,       // 度（北纬）
	Rho:     2.67,       // g/cm³
}
