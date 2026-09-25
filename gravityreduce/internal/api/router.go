package api

import (
	"github.com/gin-gonic/gin"
)

// NewRouter 装配全部 HTTP 路由。本服务只做重力测量数据归算，
// 不含导航定位、卫星几何精度因子（DOP）、测绘派工或轨迹记录等功能。
func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", Health)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/reduce", Reduce) // 单点归算
		v1.POST("/scan", Scan)     // 高程扫描
		v1.GET("/sample", Sample)  // 内置手工验算示例
	}

	return r
}
