// Package units 是重力单位换算的唯一出入口。
//
// 单位链（务必在整个归算链条中保持一致）：
//
//	1 Gal    = 1 cm/s^2 = 1e-2 m/s^2
//	1 mGal   = 1e-3 Gal = 1e-5 m/s^2
//	1 m/s^2  = 1e5 mGal
//
// 因此本服务内部一律先把绝对重力观测值（SI，m/s^2）换算成 mGal，
// 再与正常重力、自由空气改正、布格板改正（均为 mGal）相加；
// 绝不允许把 m/s^2 与 mGal 两种量纲的数直接相加。
package units

// MGalPerMS2 是 1 m/s^2 折合的 mGal 数。
const MGalPerMS2 = 100000.0

// MS2PerMGal 是 1 mGal 折合的 m/s^2 数。
const MS2PerMGal = 1e-5

// MS2ToMGal 把以 m/s^2 给出的重力加速度换算为 mGal。
func MS2ToMGal(a float64) float64 {
	return a * MGalPerMS2
}

// MGalToMS2 把以 mGal 给出的重力加速度换算为 m/s^2。
func MGalToMS2(a float64) float64 {
	return a * MS2PerMGal
}
