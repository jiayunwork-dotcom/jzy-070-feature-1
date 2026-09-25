// Package gravity 实现重力勘探内业归算的物理链条：
//
//	理论正常重力 γ  →  自由空气改正 Δg_FA  →  布格板改正 Δg_B  →  布格重力异常 Δg_Bouguer
//
// 本包只做纯物理计算，不负责输入校验与 HTTP 传输（分别在 validate、api 包）。
package gravity

import "math"

// 国际正常重力公式（IAG 1980 / GRS80，Somigliana 闭合形式）的常数：
//
//	γ(φ) = γ_e · (1 + k·sin²φ) / sqrt(1 − e²·sin²φ)
const (
	// gammaEquator 为赤道（φ=0）处参考椭球面上的正常重力，单位 m/s²。
	gammaEquator = 9.7803267715
	// gammaK 为 Somigliana 公式的无量纲常数 k。
	gammaK = 0.00193185138639
	// gammaE2 为参考椭球第一偏心率平方 e²。
	gammaE2 = 0.00669437999013

	deg2rad = math.Pi / 180.0
)

// NormalGravity 由大地纬度 phiDeg（单位：度）计算参考椭球面上的理论正常重力 γ。
//
// 返回值单位为 m/s²。纬度 φ 只在这一步进入整条归算链条；
// 自由空气改正与布格板改正均与纬度无关。
// 调用前应保证 phiDeg ∈ [-90, 90]（由 validate 包负责）。
func NormalGravity(phiDeg float64) float64 {
	sinPhi := math.Sin(phiDeg * deg2rad)
	sin2 := sinPhi * sinPhi
	return gammaEquator * (1.0 + gammaK*sin2) / math.Sqrt(1.0-gammaE2*sin2)
}
