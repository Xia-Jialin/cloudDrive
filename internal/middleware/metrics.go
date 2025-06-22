package middleware

import (
	"context"
	"runtime"
	"time"

	"cloudDrive/internal/logger"
	"cloudDrive/internal/metrics"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// MetricsMiddleware Prometheus监控中间件
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := GetRequestID(c)

		// 增加活跃请求计数
		metrics.DefaultCollector.IncActiveRequests()
		defer metrics.DefaultCollector.DecActiveRequests()

		// 获取请求大小
		requestSize := c.Request.ContentLength
		if requestSize < 0 {
			requestSize = 0
		}

		// 处理请求
		c.Next()

		// 计算处理时间
		duration := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		// 获取响应大小
		responseSize := int64(c.Writer.Size())
		if responseSize < 0 {
			responseSize = 0
		}

		// 记录HTTP请求指标
		metrics.DefaultCollector.RecordHTTPRequest(
			method,
			path,
			statusCode,
			duration,
			requestSize,
			responseSize,
		)

		// 记录详细日志
		fields := &logger.LogFields{
			RequestID:  requestID,
			Method:     method,
			Path:       path,
			StatusCode: statusCode,
			Duration:   duration,
			ClientIP:   c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		}

		// 根据状态码记录不同级别的日志
		switch {
		case statusCode >= 500:
			logger.Error("HTTP request completed with server error", fields, nil)
		case statusCode >= 400:
			logger.Warn("HTTP request completed with client error", fields)
		default:
			logger.Info("HTTP request completed", fields)
		}
	}
}

// SystemMetricsCollector 系统指标收集器
type SystemMetricsCollector struct {
	stopCh chan struct{}
}

// NewSystemMetricsCollector 创建系统指标收集器
func NewSystemMetricsCollector() *SystemMetricsCollector {
	return &SystemMetricsCollector{
		stopCh: make(chan struct{}),
	}
}

// Start 启动系统指标收集
func (s *SystemMetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // 每30秒收集一次
	defer ticker.Stop()

	// 立即收集一次
	s.collectSystemMetrics()

	for {
		select {
		case <-ctx.Done():
			logger.Info("System metrics collector stopped", nil)
			return
		case <-s.stopCh:
			logger.Info("System metrics collector stopped manually", nil)
			return
		case <-ticker.C:
			s.collectSystemMetrics()
		}
	}
}

// Stop 停止系统指标收集
func (s *SystemMetricsCollector) Stop() {
	close(s.stopCh)
}

// collectSystemMetrics 收集系统指标
func (s *SystemMetricsCollector) collectSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取Goroutine数量
	goroutines := runtime.NumGoroutine()

	// 更新系统指标
	metrics.DefaultCollector.UpdateSystemMetrics(
		m.Alloc,    // 当前内存使用
		0,          // CPU使用率（需要额外实现）
		0,          // 磁盘使用率（需要额外实现）
		goroutines, // Goroutine数量
	)

	// 记录调试日志，使用简单的字段
	logger.Debug("System metrics collected", &logger.LogFields{})
}

// DatabaseMetricsCollector 数据库指标收集器
type DatabaseMetricsCollector struct {
	db *gorm.DB
}

// NewDatabaseMetricsCollector 创建数据库指标收集器
func NewDatabaseMetricsCollector(db *gorm.DB) *DatabaseMetricsCollector {
	return &DatabaseMetricsCollector{db: db}
}

// Start 启动数据库指标收集
func (d *DatabaseMetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // 每60秒收集一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.collectDBMetrics()
		}
	}
}

// collectDBMetrics 收集数据库指标
func (d *DatabaseMetricsCollector) collectDBMetrics() {
	if d.db == nil {
		return
	}

	sqlDB, err := d.db.DB()
	if err != nil {
		logger.Error("Failed to get database instance", &logger.LogFields{
			Error: err.Error(),
		}, err)
		return
	}

	stats := sqlDB.Stats()

	// 更新数据库连接指标
	metrics.DefaultCollector.UpdateDBConnections(
		stats.OpenConnections,
		stats.Idle,
		int(stats.WaitCount), // 转换为int
	)

	logger.Debug("Database metrics collected", &logger.LogFields{})
}

// RedisMetricsCollector Redis指标收集器
type RedisMetricsCollector struct {
	client *redis.Client
}

// NewRedisMetricsCollector 创建Redis指标收集器
func NewRedisMetricsCollector(client *redis.Client) *RedisMetricsCollector {
	return &RedisMetricsCollector{client: client}
}

// Start 启动Redis指标收集
func (r *RedisMetricsCollector) Start(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second) // 每60秒收集一次
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.collectRedisMetrics()
		}
	}
}

// collectRedisMetrics 收集Redis指标
func (r *RedisMetricsCollector) collectRedisMetrics() {
	if r.client == nil {
		return
	}

	// 获取Redis连接池统计信息
	poolStats := r.client.PoolStats()

	// 更新Redis连接指标
	metrics.DefaultCollector.UpdateRedisConnections(int(poolStats.TotalConns))

	logger.Debug("Redis metrics collected", &logger.LogFields{})
}

// RecordDBOperation 记录数据库操作指标
func RecordDBOperation(operation, table string, start time.Time, err error) {
	duration := time.Since(start)
	status := "success"
	if err != nil {
		status = "error"
	}

	metrics.DefaultCollector.RecordDBQuery(operation, table, status, duration)

	// 记录日志
	fields := &logger.LogFields{
		Duration: duration,
	}

	if err != nil {
		fields.Error = err.Error()
		logger.Error("Database operation failed", fields, err)
	} else {
		logger.Debug("Database operation completed", fields)
	}
}

// RecordRedisOperation 记录Redis操作指标
func RecordRedisOperation(command string, start time.Time, err error) {
	duration := time.Since(start)
	status := "success"
	if err != nil {
		status = "error"
	}

	metrics.DefaultCollector.RecordRedisCommand(command, status, duration)

	// 记录日志
	fields := &logger.LogFields{
		Duration: duration,
	}

	if err != nil {
		fields.Error = err.Error()
		logger.Error("Redis operation failed", fields, err)
	} else {
		logger.Debug("Redis operation completed", fields)
	}
}

// RecordFileOperation 记录文件操作指标
func RecordFileOperation(operation string, start time.Time, err error, userID string, size int64) {
	duration := time.Since(start)
	status := "success"
	if err != nil {
		status = "error"
	}

	metrics.DefaultCollector.RecordFileOperation(operation, status, duration)

	// 记录文件大小指标
	if size > 0 && err == nil {
		switch operation {
		case "upload":
			metrics.DefaultCollector.RecordFileUpload(userID, size)
		case "download":
			metrics.DefaultCollector.RecordFileDownload(userID, size)
		}
	}

	// 记录日志
	fields := &logger.LogFields{
		Duration: duration,
		UserID:   userID,
	}

	if err != nil {
		fields.Error = err.Error()
		logger.Error("File operation failed", fields, err)
	} else {
		logger.Info("File operation completed", fields)
	}
}
