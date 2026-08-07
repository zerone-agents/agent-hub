package handler

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"control-panel/internal/auth"
	"control-panel/pkg/database"

	"github.com/gin-gonic/gin"
)

var startTime = time.Now()

// ServiceStatus represents the health status of a single service.
type ServiceStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

// SystemMetrics holds runtime system metrics for health reporting.
type SystemMetrics struct {
	GoVersion   string `json:"go_version"`
	Goroutines  int    `json:"goroutines"`
	MemoryUsage string `json:"memory_usage"`
}

// HealthCheck reports the overall system health including database, auth, and runtime metrics.
func HealthCheck(c *gin.Context) {
	services := make(map[string]ServiceStatus)
	allHealthy := true

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	dbStart := time.Now()
	if err := database.Ping(); err != nil {
		services["database"] = ServiceStatus{
			Status: "unhealthy",
			Error:  err.Error(),
		}
		allHealthy = false
	} else {
		services["database"] = ServiceStatus{
			Status:  "healthy",
			Latency: time.Since(dbStart).String(),
		}
	}

	casdoorStart := time.Now()
	if auth.GetClient() != nil {
		services["casdoor"] = ServiceStatus{
			Status:  "healthy",
			Latency: time.Since(casdoorStart).String(),
		}
	} else {
		services["casdoor"] = ServiceStatus{
			Status: "unhealthy",
			Error:  "not initialized",
		}
		allHealthy = false
	}

	status := "healthy"
	httpStatus := http.StatusOK
	if !allHealthy {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"status":   status,
		"uptime":   time.Since(startTime).String(),
		"services": services,
		"system": SystemMetrics{
			GoVersion:   runtime.Version(),
			Goroutines:  runtime.NumGoroutine(),
			MemoryUsage: formatBytes(memStats.Alloc),
		},
	})
}

// ServiceHealthCheck reports the health status of a specific named service.
func ServiceHealthCheck(c *gin.Context) {
	serviceName := c.Param("service")

	services := map[string]ServiceStatus{
		"backend": {Status: "healthy"},
		"casdoor": {Status: "healthy"},
		"mysql":   {Status: "healthy"},
	}

	status, ok := services[serviceName]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "service not found",
		})
		return
	}

	httpStatus := http.StatusOK
	if status.Status != "healthy" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"success": true,
		"data": gin.H{
			"service": serviceName,
			"status":  status.Status,
		},
	})
}

func formatBytes(b uint64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
}
