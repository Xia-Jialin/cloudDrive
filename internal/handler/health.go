package handler

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"syscall"
	"time"

	"cloudDrive/internal/logger"
	"cloudDrive/internal/middleware"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// ComponentStatus 组件状态
type ComponentStatus string

const (
	StatusHealthy   ComponentStatus = "healthy"
	StatusDegraded  ComponentStatus = "degraded"
	StatusUnhealthy ComponentStatus = "unhealthy"
)

// Component 组件信息
type Component struct {
	Status       ComponentStatus `json:"status"`
	Message      string          `json:"message,omitempty"`
	ResponseTime string          `json:"response_time,omitempty"`
	Details      interface{}     `json:"details,omitempty"`
}

// SystemInfo 系统信息
type SystemInfo struct {
	MemoryUsage    MemoryInfo `json:"memory_usage"`
	DiskUsage      DiskInfo   `json:"disk_usage"`
	GoroutineCount int        `json:"goroutine_count"`
	CPUCount       int        `json:"cpu_count"`
}

// MemoryInfo 内存信息
type MemoryInfo struct {
	Alloc      uint64  `json:"alloc"`       // 当前分配的内存
	TotalAlloc uint64  `json:"total_alloc"` // 总分配的内存
	Sys        uint64  `json:"sys"`         // 系统内存
	NumGC      uint32  `json:"num_gc"`      // GC次数
	AllocMB    float64 `json:"alloc_mb"`    // 当前分配内存(MB)
	SysMB      float64 `json:"sys_mb"`      // 系统内存(MB)
}

// DiskInfo 磁盘信息
type DiskInfo struct {
	Total       uint64  `json:"total"`        // 总空间
	Free        uint64  `json:"free"`         // 可用空间
	Used        uint64  `json:"used"`         // 已用空间
	UsedPercent float64 `json:"used_percent"` // 使用百分比
	TotalGB     float64 `json:"total_gb"`     // 总空间(GB)
	FreeGB      float64 `json:"free_gb"`      // 可用空间(GB)
	UsedGB      float64 `json:"used_gb"`      // 已用空间(GB)
}

// EnhancedHealthCheckResponse 增强的健康检查响应
type EnhancedHealthCheckResponse struct {
	Status     ComponentStatus      `json:"status"`
	Timestamp  time.Time            `json:"timestamp"`
	Version    string               `json:"version"`
	Uptime     string               `json:"uptime"`
	RequestID  string               `json:"request_id"`
	Components map[string]Component `json:"components"`
	SystemInfo SystemInfo           `json:"system_info"`
}

var startTime = time.Now()

// HealthCheck 处理健康检查请求
func HealthCheck(c *gin.Context) {
	start := time.Now()
	requestID := middleware.GetRequestID(c)

	// 记录健康检查请求
	fields := &logger.LogFields{
		RequestID: requestID,
		Method:    c.Request.Method,
		Path:      c.Request.URL.Path,
		ClientIP:  c.ClientIP(),
	}

	logger.Info("Health check requested", fields)

	// 检查各组件状态
	components := make(map[string]Component)
	overallStatus := StatusHealthy

	// 检查数据库
	if db, exists := c.Get("db"); exists {
		if gormDB, ok := db.(*gorm.DB); ok {
			dbComponent := checkDatabase(gormDB)
			components["database"] = dbComponent
			if dbComponent.Status != StatusHealthy {
				overallStatus = StatusDegraded
			}
		}
	}

	// 检查Redis
	if redisVal, exists := c.Get("redis"); exists {
		if redisClient, ok := redisVal.(*redis.Client); ok {
			redisComponent := checkRedis(redisClient)
			components["redis"] = redisComponent
			if redisComponent.Status != StatusHealthy {
				overallStatus = StatusDegraded
			}
		}
	}

	// 检查存储服务
	if storage, exists := c.Get(StorageKey); exists {
		storageComponent := checkStorage(storage)
		components["storage"] = storageComponent
		if storageComponent.Status != StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	// 获取系统信息
	systemInfo := getSystemInfo()

	// 构建响应
	response := EnhancedHealthCheckResponse{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Version:    "1.0.0",
		Uptime:     time.Since(startTime).String(),
		RequestID:  requestID,
		Components: components,
		SystemInfo: systemInfo,
	}

	// 记录处理时间
	duration := time.Since(start)
	fields.Duration = duration

	// 检查响应时间是否超过100ms
	if duration > 100*time.Millisecond {
		logger.Warn("Health check response time exceeded 100ms", fields)
	}

	// 根据整体状态返回相应的HTTP状态码
	var statusCode int
	switch overallStatus {
	case StatusHealthy:
		statusCode = http.StatusOK
	case StatusDegraded:
		statusCode = http.StatusOK // 降级状态仍返回200，但在响应体中标明
	case StatusUnhealthy:
		statusCode = http.StatusServiceUnavailable
	}

	fields.StatusCode = statusCode
	logger.Info("Health check completed", fields)

	c.JSON(statusCode, response)
}

// checkDatabase 检查数据库连接
func checkDatabase(db *gorm.DB) Component {
	start := time.Now()

	// 获取底层sql.DB
	sqlDB, err := db.DB()
	if err != nil {
		return Component{
			Status:       StatusUnhealthy,
			Message:      "Failed to get database connection",
			ResponseTime: time.Since(start).String(),
		}
	}

	// 执行ping测试
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return Component{
			Status:       StatusUnhealthy,
			Message:      "Database ping failed",
			ResponseTime: time.Since(start).String(),
			Details:      map[string]interface{}{"error": err.Error()},
		}
	}

	// 获取连接池状态
	stats := sqlDB.Stats()
	details := map[string]interface{}{
		"open_connections": stats.OpenConnections,
		"in_use":           stats.InUse,
		"idle":             stats.Idle,
		"max_open":         stats.MaxOpenConnections,
	}

	// 检查连接池是否健康
	status := StatusHealthy
	message := "Database is healthy"

	if stats.OpenConnections > int(float64(stats.MaxOpenConnections)*0.8) {
		status = StatusDegraded
		message = "Database connection pool usage is high"
	}

	return Component{
		Status:       status,
		Message:      message,
		ResponseTime: time.Since(start).String(),
		Details:      details,
	}
}

// checkRedis 检查Redis连接
func checkRedis(client *redis.Client) Component {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 执行ping测试
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		return Component{
			Status:       StatusUnhealthy,
			Message:      "Redis ping failed",
			ResponseTime: time.Since(start).String(),
			Details:      map[string]interface{}{"error": err.Error()},
		}
	}

	// 获取Redis信息
	info, err := client.Info(ctx).Result()
	if err != nil {
		return Component{
			Status:       StatusDegraded,
			Message:      "Redis info command failed",
			ResponseTime: time.Since(start).String(),
			Details:      map[string]interface{}{"pong": pong},
		}
	}

	return Component{
		Status:       StatusHealthy,
		Message:      "Redis is healthy",
		ResponseTime: time.Since(start).String(),
		Details: map[string]interface{}{
			"pong":           pong,
			"info_available": len(info) > 0,
		},
	}
}

// checkStorage 检查存储服务
func checkStorage(storage interface{}) Component {
	start := time.Now()

	// 这里可以根据具体的存储实现进行检查
	// 目前返回基本状态
	return Component{
		Status:       StatusHealthy,
		Message:      "Storage service is available",
		ResponseTime: time.Since(start).String(),
		Details: map[string]interface{}{
			"type": fmt.Sprintf("%T", storage),
		},
	}
}

// getSystemInfo 获取系统信息
func getSystemInfo() SystemInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memInfo := MemoryInfo{
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		AllocMB:    float64(m.Alloc) / 1024 / 1024,
		SysMB:      float64(m.Sys) / 1024 / 1024,
	}

	diskInfo := getDiskInfo()

	return SystemInfo{
		MemoryUsage:    memInfo,
		DiskUsage:      diskInfo,
		GoroutineCount: runtime.NumGoroutine(),
		CPUCount:       runtime.NumCPU(),
	}
}

// getDiskInfo 获取磁盘信息
func getDiskInfo() DiskInfo {
	var stat syscall.Statfs_t

	// 获取当前目录的磁盘信息
	if err := syscall.Statfs(".", &stat); err != nil {
		return DiskInfo{}
	}

	// 计算磁盘空间
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	usedPercent := float64(used) / float64(total) * 100

	return DiskInfo{
		Total:       total,
		Free:        free,
		Used:        used,
		UsedPercent: usedPercent,
		TotalGB:     float64(total) / 1024 / 1024 / 1024,
		FreeGB:      float64(free) / 1024 / 1024 / 1024,
		UsedGB:      float64(used) / 1024 / 1024 / 1024,
	}
}
