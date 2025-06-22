package metrics

import (
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MetricsCollector 监控指标收集器
type MetricsCollector struct {
	// HTTP 请求指标
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPResponseSize    *prometheus.HistogramVec
	HTTPRequestSize     *prometheus.HistogramVec
	HTTPActiveRequests  prometheus.Gauge

	// 数据库指标
	DBConnectionsActive  prometheus.Gauge
	DBConnectionsIdle    prometheus.Gauge
	DBConnectionsWaiting prometheus.Gauge
	DBQueryDuration      *prometheus.HistogramVec
	DBQueryTotal         *prometheus.CounterVec

	// Redis 指标
	RedisConnectionsActive prometheus.Gauge
	RedisCommandDuration   *prometheus.HistogramVec
	RedisCommandTotal      *prometheus.CounterVec

	// 文件操作指标
	FileOperationsTotal   *prometheus.CounterVec
	FileOperationDuration *prometheus.HistogramVec
	FileUploadSize        *prometheus.HistogramVec
	FileDownloadSize      *prometheus.HistogramVec

	// 系统指标
	SystemMemoryUsage prometheus.Gauge
	SystemCPUUsage    prometheus.Gauge
	SystemDiskUsage   prometheus.Gauge
	SystemGoroutines  prometheus.Gauge

	// 业务指标
	ActiveUsers      prometheus.Gauge
	TotalUsers       prometheus.Gauge
	TotalFiles       prometheus.Gauge
	TotalStorageUsed prometheus.Gauge
}

var (
	// DefaultCollector 默认指标收集器
	DefaultCollector *MetricsCollector
	once             sync.Once
)

// NewMetricsCollector 创建新的指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		// HTTP 请求指标
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status_code"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path", "status_code"},
		),
		HTTPResponseSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPRequestSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_size_bytes",
				Help:    "HTTP request size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),
		HTTPActiveRequests: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "http_active_requests",
				Help: "Number of active HTTP requests",
			},
		),

		// 数据库指标
		DBConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_active",
				Help: "Number of active database connections",
			},
		),
		DBConnectionsIdle: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_idle",
				Help: "Number of idle database connections",
			},
		),
		DBConnectionsWaiting: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "db_connections_waiting",
				Help: "Number of waiting database connections",
			},
		),
		DBQueryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_query_duration_seconds",
				Help:    "Database query duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "table"},
		),
		DBQueryTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "db_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"operation", "table", "status"},
		),

		// Redis 指标
		RedisConnectionsActive: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "redis_connections_active",
				Help: "Number of active Redis connections",
			},
		),
		RedisCommandDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "redis_command_duration_seconds",
				Help:    "Redis command duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"command"},
		),
		RedisCommandTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "redis_commands_total",
				Help: "Total number of Redis commands",
			},
			[]string{"command", "status"},
		),

		// 文件操作指标
		FileOperationsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "file_operations_total",
				Help: "Total number of file operations",
			},
			[]string{"operation", "status"},
		),
		FileOperationDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "file_operation_duration_seconds",
				Help:    "File operation duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		FileUploadSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "file_upload_size_bytes",
				Help:    "File upload size in bytes",
				Buckets: prometheus.ExponentialBuckets(1024, 10, 10), // 1KB to ~1TB
			},
			[]string{"user_id"},
		),
		FileDownloadSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "file_download_size_bytes",
				Help:    "File download size in bytes",
				Buckets: prometheus.ExponentialBuckets(1024, 10, 10), // 1KB to ~1TB
			},
			[]string{"user_id"},
		),

		// 系统指标
		SystemMemoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "system_memory_usage_bytes",
				Help: "System memory usage in bytes",
			},
		),
		SystemCPUUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "system_cpu_usage_percent",
				Help: "System CPU usage percentage",
			},
		),
		SystemDiskUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "system_disk_usage_percent",
				Help: "System disk usage percentage",
			},
		),
		SystemGoroutines: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "system_goroutines",
				Help: "Number of goroutines",
			},
		),

		// 业务指标
		ActiveUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "active_users",
				Help: "Number of active users",
			},
		),
		TotalUsers: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "total_users",
				Help: "Total number of users",
			},
		),
		TotalFiles: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "total_files",
				Help: "Total number of files",
			},
		),
		TotalStorageUsed: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "total_storage_used_bytes",
				Help: "Total storage used in bytes",
			},
		),
	}
}

// RecordHTTPRequest 记录HTTP请求指标
func (c *MetricsCollector) RecordHTTPRequest(method, path string, statusCode int, duration time.Duration, requestSize, responseSize int64) {
	status := strconv.Itoa(statusCode)

	c.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	c.HTTPRequestDuration.WithLabelValues(method, path, status).Observe(duration.Seconds())

	if requestSize > 0 {
		c.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(requestSize))
	}
	if responseSize > 0 {
		c.HTTPResponseSize.WithLabelValues(method, path).Observe(float64(responseSize))
	}
}

// IncActiveRequests 增加活跃请求数
func (c *MetricsCollector) IncActiveRequests() {
	c.HTTPActiveRequests.Inc()
}

// DecActiveRequests 减少活跃请求数
func (c *MetricsCollector) DecActiveRequests() {
	c.HTTPActiveRequests.Dec()
}

// RecordDBQuery 记录数据库查询指标
func (c *MetricsCollector) RecordDBQuery(operation, table, status string, duration time.Duration) {
	c.DBQueryTotal.WithLabelValues(operation, table, status).Inc()
	c.DBQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// UpdateDBConnections 更新数据库连接指标
func (c *MetricsCollector) UpdateDBConnections(active, idle, waiting int) {
	c.DBConnectionsActive.Set(float64(active))
	c.DBConnectionsIdle.Set(float64(idle))
	c.DBConnectionsWaiting.Set(float64(waiting))
}

// RecordRedisCommand 记录Redis命令指标
func (c *MetricsCollector) RecordRedisCommand(command, status string, duration time.Duration) {
	c.RedisCommandTotal.WithLabelValues(command, status).Inc()
	c.RedisCommandDuration.WithLabelValues(command).Observe(duration.Seconds())
}

// UpdateRedisConnections 更新Redis连接指标
func (c *MetricsCollector) UpdateRedisConnections(active int) {
	c.RedisConnectionsActive.Set(float64(active))
}

// RecordFileOperation 记录文件操作指标
func (c *MetricsCollector) RecordFileOperation(operation, status string, duration time.Duration) {
	c.FileOperationsTotal.WithLabelValues(operation, status).Inc()
	c.FileOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordFileUpload 记录文件上传指标
func (c *MetricsCollector) RecordFileUpload(userID string, size int64) {
	c.FileUploadSize.WithLabelValues(userID).Observe(float64(size))
}

// RecordFileDownload 记录文件下载指标
func (c *MetricsCollector) RecordFileDownload(userID string, size int64) {
	c.FileDownloadSize.WithLabelValues(userID).Observe(float64(size))
}

// UpdateSystemMetrics 更新系统指标
func (c *MetricsCollector) UpdateSystemMetrics(memoryUsage uint64, cpuUsage, diskUsage float64, goroutines int) {
	c.SystemMemoryUsage.Set(float64(memoryUsage))
	c.SystemCPUUsage.Set(cpuUsage)
	c.SystemDiskUsage.Set(diskUsage)
	c.SystemGoroutines.Set(float64(goroutines))
}

// UpdateBusinessMetrics 更新业务指标
func (c *MetricsCollector) UpdateBusinessMetrics(activeUsers, totalUsers, totalFiles int, totalStorageUsed uint64) {
	c.ActiveUsers.Set(float64(activeUsers))
	c.TotalUsers.Set(float64(totalUsers))
	c.TotalFiles.Set(float64(totalFiles))
	c.TotalStorageUsed.Set(float64(totalStorageUsed))
}

// GetDefaultCollector 获取默认收集器（线程安全）
func GetDefaultCollector() *MetricsCollector {
	once.Do(func() {
		DefaultCollector = NewMetricsCollector()
	})
	return DefaultCollector
}

// init 初始化默认收集器
func init() {
	DefaultCollector = GetDefaultCollector()
}
