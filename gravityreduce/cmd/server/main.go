// Command server 启动重力归算后端服务（仅 HTTP 接口，无界面、无用户系统）。
package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"gravityreduce/internal/api"
)

func main() {
	// 容器内默认安静输出；需要调试可设 GIN_MODE=debug。
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r := api.NewRouter()
	log.Printf("gravity-reduction service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
