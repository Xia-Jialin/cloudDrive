package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloudDrive/cmd/chunkserver/internal/api"
	"cloudDrive/cmd/chunkserver/internal/config"
	"cloudDrive/cmd/chunkserver/internal/service"
	"cloudDrive/internal/discovery"
	"cloudDrive/internal/storage"

	"github.com/go-redis/redis/v8"
)

var (
	configPath   = flag.String("config", "configs/chunkserver.yaml", "配置文件路径")
	etcdEndpoint = flag.String("etcd", "", "ETCD服务器地址")
	etcdKey      = flag.String("etcd-key", "/clouddrive/chunkserver/config", "ETCD中的配置键")
)

func main() {
	flag.Parse()

	// 从环境变量获取配置
	if env := os.Getenv("ETCD_ENDPOINT"); env != "" {
		*etcdEndpoint = env
	}
	if env := os.Getenv("ETCD_KEY"); env != "" {
		*etcdKey = env
	}

	// 加载配置
	cfg, err := config.LoadConfig(*configPath, *etcdEndpoint, *etcdKey)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 确保必要的目录存在
	if err := config.EnsureDirectories(cfg); err != nil {
		log.Fatalf("创建目录失败: %v", err)
	}

	// 初始化Redis连接
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.User,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
		PoolSize: cfg.Redis.PoolSize,
	})
	defer rdb.Close()

	// 测试Redis连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("连接Redis失败: %v", err)
	}
	log.Println("成功连接到Redis")

	// 初始化存储后端
	var storageBackend storage.Storage
	switch cfg.Storage.Type {
	case "local":
		// 创建本地存储实例
		localStorage := &storage.LocalFileStorage{Dir: cfg.Storage.LocalDir}
		storageBackend = localStorage
		log.Printf("使用本地文件存储: %s", cfg.Storage.LocalDir)
	case "minio":
		// 创建MinIO存储实例
		minioStorage, err := storage.NewMinioStorage(
			cfg.Storage.Minio.Endpoint,
			cfg.Storage.Minio.AccessKey,
			cfg.Storage.Minio.SecretKey,
			cfg.Storage.Minio.Bucket,
			cfg.Storage.Minio.UseSSL,
		)
		if err != nil {
			log.Fatalf("初始化MinIO存储失败: %v", err)
		}
		storageBackend = minioStorage
		log.Printf("使用MinIO存储: %s", cfg.Storage.Minio.Endpoint)
	default:
		log.Fatalf("不支持的存储类型: %s", cfg.Storage.Type)
	}

	// 创建存储服务
	storageService := service.NewStorageService(storageBackend, rdb)

	// 创建HTTP服务器
	httpServer := api.NewHTTPServer(storageService, rdb, cfg.Server.HTTPPort)

	// 创建gRPC服务器
	grpcServer := api.NewGRPCServer(storageService)

	// 注册服务到ETCD
	if *etcdEndpoint != "" {
		etcdEndpoints := strings.Split(*etcdEndpoint, ",")

		// 准备服务信息
		serviceInfo := discovery.ServiceInfo{
			Name:      "clouddrive-chunkserver",
			Address:   "", // 自动获取
			Port:      cfg.Server.HTTPPort,
			Version:   "1.0.0",
			StartTime: time.Now(),
			Metadata: map[string]string{
				"storage_type": cfg.Storage.Type,
				"grpc_port":    strconv.Itoa(cfg.Server.GRPCPort),
			},
			Endpoints: map[string]string{
				"http": fmt.Sprintf("/api"),
				"grpc": fmt.Sprintf(":%d", cfg.Server.GRPCPort),
			},
		}

		// 创建服务注册实例
		registry, err := discovery.NewEtcdServiceRegistry(etcdEndpoints, serviceInfo, 15)
		if err != nil {
			log.Printf("创建服务注册失败: %v", err)
		} else {
			// 注册服务
			ctx := context.Background()
			if err := registry.Register(ctx); err != nil {
				log.Printf("注册服务到ETCD失败: %v", err)
			} else {
				log.Printf("服务已成功注册到ETCD")
			}

			// 优雅关闭时注销服务
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := registry.Deregister(ctx); err != nil {
					log.Printf("注销服务失败: %v", err)
				} else {
					log.Printf("服务已成功从ETCD注销")
				}
			}()
		}
	} else {
		log.Println("未配置ETCD地址，跳过服务注册")
	}

	// 启动HTTP服务器
	go func() {
		log.Printf("HTTP服务器启动在端口 %d", cfg.Server.HTTPPort)
		if err := httpServer.Start(); err != nil {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()

	// 启动gRPC服务器
	go func() {
		log.Printf("gRPC服务器启动在端口 %d", cfg.Server.GRPCPort)
		if err := grpcServer.Start(cfg.Server.GRPCPort); err != nil {
			log.Fatalf("gRPC服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 优雅关闭服务器
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v", err)
	}

	log.Println("服务器已关闭")
}
