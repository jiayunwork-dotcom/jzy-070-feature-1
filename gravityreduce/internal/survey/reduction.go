// 批量层对单点归算物理链条的唯一复用点：每个测点的正常重力、两项改正、
// 布格异常都经由此处调用既有 gravity.Reduce 得到，绝不重写物理公式。
package survey

import "gravityreduce/internal/gravity"

// reducePoint 复用单点归算链条，返回该测点完整归算结果。
func reducePoint(p Point) gravity.Result {
	return gravity.Reduce(p.Obs)
}
