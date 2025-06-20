package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// 服务注册的默认TTL
	defaultTTL = 10
)

// ServiceInfo 服务信息
type ServiceInfo struct {
	Name        string            `json:"name"`        // 服务名称
	ID          string            `json:"id"`          // 服务实例ID
	Address     string            `json:"address"`     // 服务地址
	Port        int               `json:"port"`        // 服务端口
	Version     string            `json:"version"`     // 服务版本
	Metadata    map[string]string `json:"metadata"`    // 服务元数据
	Endpoints   map[string]string `json:"endpoints"`   // 服务提供的端点
	StartTime   time.Time         `json:"start_time"`  // 服务启动时间
	Environment string            `json:"environment"` // 运行环境
}

// ServiceRegistry 服务注册接口
type ServiceRegistry interface {
	// Register 注册服务
	Register(ctx context.Context) error
	// Deregister 注销服务
	Deregister(ctx context.Context) error
	// GetServiceInfo 获取服务信息
	GetServiceInfo() ServiceInfo
}

// EtcdServiceRegistry 基于etcd的服务注册
type EtcdServiceRegistry struct {
	client     *clientv3.Client
	leaseID    clientv3.LeaseID
	serviceKey string
	service    ServiceInfo
	ttl        int64
	closeCh    chan struct{}
}

// NewEtcdServiceRegistry 创建etcd服务注册
func NewEtcdServiceRegistry(endpoints []string, service ServiceInfo, ttl int64) (*EtcdServiceRegistry, error) {
	if ttl <= 0 {
		ttl = defaultTTL
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	// 如果没有指定服务ID，则自动生成
	if service.ID == "" {
		hostname, _ := os.Hostname()
		service.ID = fmt.Sprintf("%s-%s-%d", service.Name, hostname, time.Now().UnixNano())
	}

	// 如果没有指定服务地址，尝试获取本地IP
	if service.Address == "" {
		service.Address = getLocalIP()
	}

	registry := &EtcdServiceRegistry{
		client:     cli,
		service:    service,
		ttl:        ttl,
		serviceKey: fmt.Sprintf("/services/%s/%s", service.Name, service.ID),
		closeCh:    make(chan struct{}),
	}

	return registry, nil
}

// Register 注册服务
func (r *EtcdServiceRegistry) Register(ctx context.Context) error {
	// 创建租约
	resp, err := r.client.Grant(ctx, r.ttl)
	if err != nil {
		return err
	}
	r.leaseID = resp.ID

	// 将服务信息序列化为JSON
	value, err := json.Marshal(r.service)
	if err != nil {
		return err
	}

	// 注册服务
	_, err = r.client.Put(ctx, r.serviceKey, string(value), clientv3.WithLease(r.leaseID))
	if err != nil {
		return err
	}

	// 启动自动续租
	go r.keepAlive(ctx)

	log.Printf("服务 %s 已注册到 etcd，服务ID: %s", r.service.Name, r.service.ID)
	return nil
}

// Deregister 注销服务
func (r *EtcdServiceRegistry) Deregister(ctx context.Context) error {
	// 关闭续租
	close(r.closeCh)

	// 撤销租约
	_, err := r.client.Revoke(ctx, r.leaseID)
	if err != nil {
		return err
	}

	log.Printf("服务 %s 已从 etcd 注销，服务ID: %s", r.service.Name, r.service.ID)
	return r.client.Close()
}

// GetServiceInfo 获取服务信息
func (r *EtcdServiceRegistry) GetServiceInfo() ServiceInfo {
	return r.service
}

// keepAlive 保持租约有效
func (r *EtcdServiceRegistry) keepAlive(ctx context.Context) {
	// 创建一个新的context，避免外部context取消影响续租
	keepAliveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动自动续租
	keepAliveCh, err := r.client.KeepAlive(keepAliveCtx, r.leaseID)
	if err != nil {
		log.Printf("启动自动续租失败: %v", err)
		return
	}

	for {
		select {
		case <-r.closeCh:
			// 服务注销，停止续租
			return
		case resp, ok := <-keepAliveCh:
			// 续租响应
			if !ok {
				log.Printf("租约 %d 已过期或续租失败，尝试重新注册", r.leaseID)
				// 尝试重新注册
				if err := r.Register(ctx); err != nil {
					log.Printf("重新注册服务失败: %v", err)
				}
				return
			}
			log.Printf("成功续租 %d，TTL: %d", resp.ID, resp.TTL)
		}
	}
}

// getLocalIP 获取本地IP地址
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// 检查IP地址是否为回环地址
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

// 服务发现相关函数

// ServiceDiscovery 服务发现接口
type ServiceDiscovery interface {
	// GetService 获取指定服务的所有实例
	GetService(ctx context.Context, serviceName string) ([]ServiceInfo, error)
	// WatchService 监听服务变化
	WatchService(ctx context.Context, serviceName string) (<-chan []ServiceInfo, error)
	// Close 关闭服务发现
	Close() error
}

// EtcdServiceDiscovery 基于etcd的服务发现
type EtcdServiceDiscovery struct {
	client *clientv3.Client
}

// NewEtcdServiceDiscovery 创建etcd服务发现
func NewEtcdServiceDiscovery(endpoints []string) (*EtcdServiceDiscovery, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	return &EtcdServiceDiscovery{
		client: cli,
	}, nil
}

// GetService 获取指定服务的所有实例
func (d *EtcdServiceDiscovery) GetService(ctx context.Context, serviceName string) ([]ServiceInfo, error) {
	resp, err := d.client.Get(ctx, fmt.Sprintf("/services/%s/", serviceName), clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	var services []ServiceInfo
	for _, kv := range resp.Kvs {
		var service ServiceInfo
		if err := json.Unmarshal(kv.Value, &service); err != nil {
			log.Printf("解析服务信息失败: %v", err)
			continue
		}
		services = append(services, service)
	}

	return services, nil
}

// WatchService 监听服务变化
func (d *EtcdServiceDiscovery) WatchService(ctx context.Context, serviceName string) (<-chan []ServiceInfo, error) {
	serviceCh := make(chan []ServiceInfo, 10)

	// 先获取当前所有服务
	services, err := d.GetService(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	// 发送初始服务列表
	serviceCh <- services

	// 监听服务变化
	go func() {
		defer close(serviceCh)

		watchCh := d.client.Watch(ctx, fmt.Sprintf("/services/%s/", serviceName), clientv3.WithPrefix())
		for {
			select {
			case <-ctx.Done():
				return
			case watchResp := <-watchCh:
				if watchResp.Canceled {
					log.Printf("监听服务 %s 被取消", serviceName)
					return
				}

				// 有变化，重新获取所有服务
				services, err := d.GetService(ctx, serviceName)
				if err != nil {
					log.Printf("获取服务 %s 失败: %v", serviceName, err)
					continue
				}

				// 发送更新后的服务列表
				serviceCh <- services
			}
		}
	}()

	return serviceCh, nil
}

// Close 关闭服务发现
func (d *EtcdServiceDiscovery) Close() error {
	return d.client.Close()
}
