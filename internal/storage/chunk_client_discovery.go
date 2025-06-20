package storage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"cloudDrive/internal/discovery"

	"github.com/go-redis/redis/v8"
)

// ChunkServerDiscovery 块存储服务发现客户端
type ChunkServerDiscovery struct {
	discovery      discovery.ServiceDiscovery
	serviceName    string
	instances      []discovery.ServiceInfo
	instancesMutex sync.RWMutex
	watchCtx       context.Context
	watchCancel    context.CancelFunc
	redisClient    *redis.Client
	tempDir        string
	clients        map[string]*ChunkServerStorage
	clientsMutex   sync.RWMutex
}

// NewChunkServerDiscovery 创建块存储服务发现客户端
func NewChunkServerDiscovery(etcdEndpoints []string, serviceName string, redisClient *redis.Client, tempDir string) (*ChunkServerDiscovery, error) {
	// 创建服务发现实例
	serviceDiscovery, err := discovery.NewEtcdServiceDiscovery(etcdEndpoints)
	if err != nil {
		return nil, err
	}

	// 创建上下文，用于监听服务变化
	watchCtx, watchCancel := context.WithCancel(context.Background())

	discovery := &ChunkServerDiscovery{
		discovery:   serviceDiscovery,
		serviceName: serviceName,
		instances:   make([]discovery.ServiceInfo, 0),
		redisClient: redisClient,
		tempDir:     tempDir,
		clients:     make(map[string]*ChunkServerStorage),
		watchCtx:    watchCtx,
		watchCancel: watchCancel,
	}

	// 初始化实例列表
	if err := discovery.refreshInstances(); err != nil {
		return nil, err
	}

	// 启动监听服务变化
	go discovery.watchServiceChanges()

	return discovery, nil
}

// GetChunkServerClient 获取块存储服务客户端
func (d *ChunkServerDiscovery) GetChunkServerClient() (*ChunkServerStorage, error) {
	d.instancesMutex.RLock()
	defer d.instancesMutex.RUnlock()

	if len(d.instances) == 0 {
		return nil, errors.New("没有可用的块存储服务实例")
	}

	// 简单的轮询负载均衡，这里可以根据需要实现更复杂的负载均衡策略
	instance := d.instances[time.Now().UnixNano()%int64(len(d.instances))]

	// 检查是否已经有该实例的客户端
	d.clientsMutex.RLock()
	client, exists := d.clients[instance.ID]
	d.clientsMutex.RUnlock()

	if exists {
		return client, nil
	}

	// 创建新的客户端
	baseURL := fmt.Sprintf("http://%s:%d", instance.Address, instance.Port)
	client, err := NewChunkServerStorage(baseURL, d.redisClient, d.tempDir)
	if err != nil {
		return nil, err
	}

	// 缓存客户端
	d.clientsMutex.Lock()
	d.clients[instance.ID] = client
	d.clientsMutex.Unlock()

	return client, nil
}

// refreshInstances 刷新服务实例列表
func (d *ChunkServerDiscovery) refreshInstances() error {
	instances, err := d.discovery.GetService(context.Background(), d.serviceName)
	if err != nil {
		return err
	}

	d.instancesMutex.Lock()
	d.instances = instances
	d.instancesMutex.Unlock()

	// 清理不存在的实例客户端
	d.cleanupClients(instances)

	return nil
}

// cleanupClients 清理不存在的实例客户端
func (d *ChunkServerDiscovery) cleanupClients(instances []discovery.ServiceInfo) {
	// 创建当前实例ID集合
	instanceIDs := make(map[string]bool)
	for _, instance := range instances {
		instanceIDs[instance.ID] = true
	}

	// 删除不存在的实例客户端
	d.clientsMutex.Lock()
	defer d.clientsMutex.Unlock()

	for id := range d.clients {
		if !instanceIDs[id] {
			delete(d.clients, id)
		}
	}
}

// watchServiceChanges 监听服务变化
func (d *ChunkServerDiscovery) watchServiceChanges() {
	// 监听服务变化
	servicesCh, err := d.discovery.WatchService(d.watchCtx, d.serviceName)
	if err != nil {
		log.Printf("监听服务变化失败: %v", err)
		return
	}

	for {
		select {
		case <-d.watchCtx.Done():
			return
		case services, ok := <-servicesCh:
			if !ok {
				log.Printf("服务监听通道已关闭")
				return
			}

			// 更新实例列表
			d.instancesMutex.Lock()
			d.instances = services
			d.instancesMutex.Unlock()

			// 清理不存在的实例客户端
			d.cleanupClients(services)

			log.Printf("块存储服务列表已更新，当前有 %d 个实例", len(services))
		}
	}
}

// Close 关闭服务发现客户端
func (d *ChunkServerDiscovery) Close() error {
	// 取消监听
	d.watchCancel()

	// 关闭服务发现
	return d.discovery.Close()
}

// GetAllInstances 获取所有服务实例
func (d *ChunkServerDiscovery) GetAllInstances() []discovery.ServiceInfo {
	d.instancesMutex.RLock()
	defer d.instancesMutex.RUnlock()

	// 复制实例列表
	instances := make([]discovery.ServiceInfo, len(d.instances))
	copy(instances, d.instances)

	return instances
}
