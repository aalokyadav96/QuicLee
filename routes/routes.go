package routes

import (
	"naevis/handlers"
	"naevis/ratelim"
	_ "net/http/pprof"

	"github.com/julienschmidt/httprouter"
)

func AddProxyRoutes(router *httprouter.Router, rateLimiter *ratelim.RateLimiter) {
	// route /api and all subpaths
	router.GET("/api", handlers.ApiHandler)
	router.POST("/api", handlers.ApiHandler)
	router.PUT("/api", handlers.ApiHandler)
	router.DELETE("/api", handlers.ApiHandler)
	router.GET("/api/*path", handlers.ApiHandler)
	router.POST("/api/*path", handlers.ApiHandler)
	router.PUT("/api/*path", handlers.ApiHandler)
	router.DELETE("/api/*path", handlers.ApiHandler)
}
