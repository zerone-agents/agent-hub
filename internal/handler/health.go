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

// HealthCheck reports the legacy Casdoor-aware health view. New server wiring
// should use HealthCheckForAuthMode so optional auth backends do not make an
// otherwise healthy deployment fail readiness checks.
func HealthCheck(c *gin.Context) {
	healthCheck(c, true)
}

// HealthCheckForAuthMode builds a health handler for the configured auth mode.
// Casdoor is a required dependency only when Casdoor authentication is active;
// builtin deployments report it as disabled and remain healthy.
func HealthCheckForAuthMode(casdoorRequired bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		healthCheck(c, casdoorRequired)
	}
}

func healthCheck(c *gin.Context, casdoorRequired bool) {
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
	if !casdoorRequired {
		services["casdoor"] = ServiceStatus{Status: "disabled"}
	} else if auth.GetClient() != nil {
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

// ServiceHealthCheck retains the legacy Casdoor-required behaviour.
func ServiceHealthCheck(c *gin.Context) {
	serviceHealthCheck(c, true)
}

// ServiceHealthCheckForAuthMode reports optional Casdoor as disabled (and
// available) in builtin mode instead of returning a false-positive outage.
func ServiceHealthCheckForAuthMode(casdoorRequired bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceHealthCheck(c, casdoorRequired)
	}
}

func serviceHealthCheck(c *gin.Context, casdoorRequired bool) {
	serviceName := c.Param("service")

	services := map[string]ServiceStatus{
		"backend": {Status: "healthy"},
		"mysql":   {Status: "healthy"},
	}
	if casdoorRequired {
		if auth.GetClient() == nil {
			services["casdoor"] = ServiceStatus{Status: "unhealthy", Error: "not initialized"}
		} else {
			services["casdoor"] = ServiceStatus{Status: "healthy"}
		}
	} else {
		services["casdoor"] = ServiceStatus{Status: "disabled"}
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
	if status.Status == "unhealthy" {
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
