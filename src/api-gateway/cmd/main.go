package main

import (
	"github.com/gin-gonic/gin"
	"gitlab.lrz.de/peslalz/errorhandling-microservices-thesis/logger"
)

func main() {
	logger := logger.InitLogger()

	// Force log's color
	gin.ForceConsoleColor()

	gin.SetMode(gin.DebugMode)
	// Creates a gin router with default middleware:
	// logger and recovery (crash-free) middleware
	router := gin.Default()

	router.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	// By default it serves on :8080 unless a
	// PORT environment variable was defined.
	err := router.Run()
	if err != nil {
		return
	}
	// router.Run(":3000") for a hard coded port
}
