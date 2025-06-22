package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetDefaultCollector(t *testing.T) {
	collector := GetDefaultCollector()
	assert.NotNil(t, collector)
	assert.NotNil(t, collector.HTTPRequestsTotal)
	assert.NotNil(t, collector.HTTPRequestDuration)
	assert.NotNil(t, collector.DBConnectionsActive)
	assert.NotNil(t, collector.RedisConnectionsActive)
	assert.NotNil(t, collector.FileOperationsTotal)
	assert.NotNil(t, collector.SystemMemoryUsage)
	assert.NotNil(t, collector.ActiveUsers)

	// 测试多次调用返回同一个实例
	collector2 := GetDefaultCollector()
	assert.Equal(t, collector, collector2)
}

func TestRecordHTTPRequest(t *testing.T) {
	collector := GetDefaultCollector()

	// 记录HTTP请求，不应该panic
	assert.NotPanics(t, func() {
		collector.RecordHTTPRequest("GET", "/api/files", 200, time.Millisecond*150, 0, 1024)
	})

	// 测试不同的状态码
	assert.NotPanics(t, func() {
		collector.RecordHTTPRequest("POST", "/api/upload", 201, time.Millisecond*300, 1024, 0)
		collector.RecordHTTPRequest("GET", "/api/files", 404, time.Millisecond*50, 0, 256)
		collector.RecordHTTPRequest("POST", "/api/files", 500, time.Millisecond*1000, 2048, 0)
	})
}

func TestRecordDBQuery(t *testing.T) {
	collector := GetDefaultCollector()

	// 记录数据库查询，不应该panic
	assert.NotPanics(t, func() {
		collector.RecordDBQuery("SELECT", "users", "success", time.Millisecond*50)
		collector.RecordDBQuery("INSERT", "files", "success", time.Millisecond*100)
		collector.RecordDBQuery("UPDATE", "users", "error", time.Millisecond*200)
	})
}

func TestUpdateDBConnections(t *testing.T) {
	collector := GetDefaultCollector()

	// 更新数据库连接指标，不应该panic
	assert.NotPanics(t, func() {
		collector.UpdateDBConnections(5, 3, 0)
		collector.UpdateDBConnections(10, 5, 2)
		collector.UpdateDBConnections(0, 0, 0)
	})
}

func TestRecordRedisCommand(t *testing.T) {
	collector := GetDefaultCollector()

	// 记录Redis命令，不应该panic
	assert.NotPanics(t, func() {
		collector.RecordRedisCommand("GET", "success", time.Millisecond*10)
		collector.RecordRedisCommand("SET", "success", time.Millisecond*15)
		collector.RecordRedisCommand("DEL", "error", time.Millisecond*5)
	})
}

func TestUpdateRedisConnections(t *testing.T) {
	collector := GetDefaultCollector()

	// 更新Redis连接指标，不应该panic
	assert.NotPanics(t, func() {
		collector.UpdateRedisConnections(3)
		collector.UpdateRedisConnections(5)
		collector.UpdateRedisConnections(0)
	})
}

func TestRecordFileOperation(t *testing.T) {
	collector := GetDefaultCollector()

	// 记录文件操作，不应该panic
	assert.NotPanics(t, func() {
		collector.RecordFileOperation("upload", "success", time.Second*2)
		collector.RecordFileOperation("download", "success", time.Millisecond*500)
		collector.RecordFileOperation("delete", "error", time.Millisecond*100)
	})
}

func TestRecordFileUploadDownload(t *testing.T) {
	collector := GetDefaultCollector()

	// 记录文件上传下载，不应该panic
	assert.NotPanics(t, func() {
		collector.RecordFileUpload("user123", 1024*1024)   // 1MB
		collector.RecordFileDownload("user456", 2048*1024) // 2MB
		collector.RecordFileUpload("user789", 0)           // 空文件
	})
}

func TestUpdateSystemMetrics(t *testing.T) {
	collector := GetDefaultCollector()

	// 更新系统指标，不应该panic
	assert.NotPanics(t, func() {
		collector.UpdateSystemMetrics(1024*1024*64, 25.5, 75.2, 42)   // 64MB, 25.5% CPU, 75.2% disk, 42 goroutines
		collector.UpdateSystemMetrics(0, 0, 0, 1)                     // 最小值
		collector.UpdateSystemMetrics(1024*1024*1024, 100, 100, 1000) // 大值
	})
}

func TestUpdateBusinessMetrics(t *testing.T) {
	collector := GetDefaultCollector()

	// 更新业务指标，不应该panic
	assert.NotPanics(t, func() {
		collector.UpdateBusinessMetrics(100, 1000, 5000, 1024*1024*1024) // 100活跃用户，1000总用户，5000文件，1GB存储
		collector.UpdateBusinessMetrics(0, 0, 0, 0)                      // 零值
		collector.UpdateBusinessMetrics(1, 1, 1, 1)                      // 最小值
	})
}

func TestIncDecActiveRequests(t *testing.T) {
	collector := GetDefaultCollector()

	// 测试活跃请求计数，不应该panic
	assert.NotPanics(t, func() {
		collector.IncActiveRequests()
		collector.IncActiveRequests()
		collector.DecActiveRequests()
		collector.DecActiveRequests()
	})
}

func TestDefaultCollector(t *testing.T) {
	// 测试默认收集器
	assert.NotNil(t, DefaultCollector)
	assert.NotNil(t, DefaultCollector.HTTPRequestsTotal)
	assert.NotNil(t, DefaultCollector.DBConnectionsActive)
	assert.NotNil(t, DefaultCollector.SystemMemoryUsage)
	assert.NotNil(t, DefaultCollector.ActiveUsers)

	// 测试默认收集器的基本功能
	assert.NotPanics(t, func() {
		DefaultCollector.RecordHTTPRequest("GET", "/test", 200, time.Millisecond*100, 0, 0)
		DefaultCollector.IncActiveRequests()
		DefaultCollector.DecActiveRequests()
	})
}
