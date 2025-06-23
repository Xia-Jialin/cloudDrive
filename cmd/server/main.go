package main

import (
	"cloudDrive/internal/discovery"
	"cloudDrive/internal/file"
	"cloudDrive/internal/handler"
	"cloudDrive/internal/logger"
	"cloudDrive/internal/middleware"
	"cloudDrive/internal/storage"
	"cloudDrive/internal/user"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "cloudDrive/docs"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	goredis "github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	goetcd "go.etcd.io/etcd/client/v3"
)

// @title CloudDrive API
// @version 1.0
// @description 云盘系统后端 API 文档
// @host localhost:8080
// @BasePath /api

var db *gorm.DB

// 新增：从etcd加载配置的函数
func loadConfigFromEtcd(endpoint, key string) ([]byte, error) {
	cli, err := goetcd.New(goetcd.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cli.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("key %s not found in etcd", key)
	}
	return resp.Kvs[0].Value, nil
}

func main() {
	// 支持通过命令行参数指定配置文件路径
	configPath := flag.String("config", "./configs/config.yaml", "配置文件路径")
	etcdEndpoint := flag.String("etcd-endpoint", "etcd:2379", "etcd服务地址")
	etcdKey := flag.String("etcd-key", "clouddrive-server", "etcd配置key")
	servicePort := flag.Int("port", 8080, "服务端口")
	flag.Parse()

	// 优先用环境变量
	if env := os.Getenv("ETCD_ENDPOINT"); env != "" {
		*etcdEndpoint = env
	}
	if env := os.Getenv("ETCD_KEY"); env != "" {
		*etcdKey = env
	}
	if env := os.Getenv("PORT"); env != "" {
		if port, err := strconv.Atoi(env); err == nil {
			*servicePort = port
		}
	}

	// 优先从etcd加载配置
	configBytes, err := loadConfigFromEtcd(*etcdEndpoint, *etcdKey)
	if err != nil {
		log.Printf("从etcd获取配置失败: %v，尝试读取本地配置文件...", err)
		viper.SetConfigFile(*configPath)
		err = viper.ReadInConfig()
		if err != nil {
			log.Fatalf("读取配置文件失败: %v", err)
		}
	} else {
		viper.SetConfigType("yaml")
		err = viper.ReadConfig(strings.NewReader(string(configBytes)))
		if err != nil {
			log.Fatalf("解析etcd配置失败: %v", err)
		}
	}

	// 初始化结构化日志
	logConfig := &logger.Config{
		Level:       viper.GetString("monitoring.log_level"),
		File:        viper.GetString("monitoring.log_file"),
		MaxSize:     viper.GetInt("monitoring.log_max_size"),
		MaxAge:      viper.GetInt("monitoring.log_max_age"),
		MaxBackups:  viper.GetInt("monitoring.log_max_backups"),
		Compress:    viper.GetBool("monitoring.log_compress"),
		Development: viper.GetString("environment") == "development",
	}

	if err := logger.InitLogger(logConfig); err != nil {
		log.Fatalf("初始化日志器失败: %v", err)
	}
	defer logger.Sync()

	logger.Info("CloudDrive服务启动", &logger.LogFields{})

	dbUser := viper.GetString("database.user")
	dbPassword := viper.GetString("database.password")
	dbHost := viper.GetString("database.host")
	dbPort := viper.GetInt("database.port")
	dbName := viper.GetString("database.name")
	dbCharset := viper.GetString("database.charset")
	parseTime := viper.GetBool("database.parseTime")
	loc := viper.GetString("database.loc")

	parseTimeStr := "False"
	if parseTime {
		parseTimeStr = "True"
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%s&loc=%s",
		dbUser, dbPassword, dbHost, dbPort, dbName, dbCharset, parseTimeStr, loc)

	// 配置GORM日志
	gormLogger := gormlogger.Default
	if viper.GetString("environment") == "production" {
		// 生产环境只记录错误
		gormLogger = gormlogger.Default.LogMode(gormlogger.Error)
	} else {
		// 开发环境记录信息
		gormLogger = gormlogger.Default.LogMode(gormlogger.Info)
	}

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表和文件表，并捕获错误
	err = db.AutoMigrate(&user.User{}, &file.File{}, &file.FileContent{}, &file.UserRoot{}, &file.Share{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	// 创建gin实例，不使用默认中间件
	r := gin.New()

	// 设置最大multipart内存为100MB，用于处理大文件上传
	r.MaxMultipartMemory = 100 << 20 // 100MB

	// 添加自定义中间件
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.ErrorHandlerMiddleware())

	// 添加Prometheus监控中间件（在日志中间件之前）
	enableMetrics := viper.GetBool("monitoring.metrics_enabled")
	if enableMetrics {
		r.Use(middleware.MetricsMiddleware())
		logger.Info("Prometheus监控已启用", &logger.LogFields{})
	}

	r.Use(middleware.SkipLoggingMiddleware("/health", "/api/health", "/metrics"))

	// 添加 CORS 跨域中间件，允许携带 cookie
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:30081", "http://198.19.249.2:30081"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
	}))

	redisAddr := viper.GetString("redis.addr")
	redisUser := viper.GetString("redis.user") // 预留用户名获取
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")
	redisPoolSize := viper.GetInt("redis.pool_size")

	// 新增 go-redis/v8 客户端初始化
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:     redisAddr,
		Username: redisUser,
		Password: redisPassword,
		DB:       redisDB,
		PoolSize: redisPoolSize,
	})

	store, err := redis.NewStoreWithDB(redisPoolSize, "tcp", redisAddr, redisUser, redisPassword, fmt.Sprintf("%d", redisDB), []byte("secret"))
	if err != nil {
		log.Fatalf("Redis session store 初始化失败: %v", err)
	}
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7天
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode, // 允许跨域携带 cookie
		Secure:   true,                  // 必须为 true，哪怕本地开发
	})
	r.Use(sessions.Sessions("cloudsession", store))

	// 读取块存储服务配置
	chunkServerEnabled := viper.GetBool("storage.chunk_server.enabled")
	chunkServerURL := viper.GetString("storage.chunk_server.url")
	chunkServerTempDir := viper.GetString("storage.chunk_server.temp_dir")
	useServiceDiscovery := viper.GetBool("storage.chunk_server.use_service_discovery")

	var storageInst storage.Storage // 用于注入

	if !chunkServerEnabled {
		log.Fatalf("必须启用块存储服务，请在配置文件中设置 storage.chunk_server.enabled=true")
	}

	// 确保临时目录存在
	if chunkServerTempDir == "" {
		chunkServerTempDir = filepath.Join(os.TempDir(), "chunk_client")
	}
	if err := os.MkdirAll(chunkServerTempDir, 0755); err != nil {
		log.Fatalf("创建块存储服务临时目录失败: %v", err)
	}

	// 使用服务发现或直接连接
	if useServiceDiscovery && *etcdEndpoint != "" {
		log.Println("使用服务发现获取块存储服务")
		// 创建块存储服务发现客户端
		chunkServerDiscovery, err := storage.NewChunkServerDiscovery(
			strings.Split(*etcdEndpoint, ","),
			"clouddrive-chunkserver",
			redisClient,
			chunkServerTempDir,
		)
		if err != nil {
			log.Printf("创建块存储服务发现客户端失败: %v，将使用静态配置", err)
		} else {
			// 获取块存储服务客户端
			chunkStorage, err := chunkServerDiscovery.GetChunkServerClient()
			if err != nil {
				log.Printf("获取块存储服务客户端失败: %v，将使用静态配置", err)
			} else {
				storageInst = chunkStorage
				log.Printf("已通过服务发现连接到块存储服务")

				// 在退出时关闭服务发现客户端
				defer chunkServerDiscovery.Close()

				// 定期打印当前可用的块存储服务实例
				go func() {
					ticker := time.NewTicker(30 * time.Second)
					defer ticker.Stop()

					for {
						select {
						case <-ticker.C:
							instances := chunkServerDiscovery.GetAllInstances()
							log.Printf("当前可用的块存储服务实例数量: %d", len(instances))
							for i, instance := range instances {
								log.Printf("实例 #%d: %s (%s:%d)", i+1, instance.ID, instance.Address, instance.Port)
							}
						}
					}
				}()
			}
		}
	}

	// 如果服务发现失败或未启用，则使用静态配置
	if storageInst == nil {
		// 初始化块存储服务客户端
		chunkStorage, err := storage.NewChunkServerStorage(chunkServerURL, redisClient, chunkServerTempDir)
		if err != nil {
			log.Fatalf("初始化块存储服务客户端失败: %v", err)
		}

		// 设置公共URL（如果配置中有）
		publicURL := viper.GetString("storage.chunk_server.public_url")
		if publicURL != "" {
			chunkStorage.SetPublicURL(publicURL)
			log.Printf("块存储服务公共URL设置为: %s", publicURL)
		}

		storageInst = chunkStorage
		log.Printf("已直接连接到块存储服务: %s", chunkServerURL)
	}

	// 启动监控指标收集器
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if enableMetrics {
		// 启动系统指标收集器
		systemCollector := middleware.NewSystemMetricsCollector()
		go systemCollector.Start(ctx)

		// 启动数据库指标收集器
		dbCollector := middleware.NewDatabaseMetricsCollector(db)
		go dbCollector.Start(ctx)

		// 启动Redis指标收集器
		redisCollector := middleware.NewRedisMetricsCollector(redisClient)
		go redisCollector.Start(ctx)

		logger.Info("监控指标收集器已启动", &logger.LogFields{})
	}

	// 注入 db、redis、storage 到 gin.Context
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("redis", redisClient)
		c.Set(handler.StorageKey, storageInst)
		c.Next()
	})

	r.POST("/api/user/register", handler.RegisterHandler)
	r.POST("/api/user/login", handler.LoginHandler)
	r.POST("/api/user/logout", handler.LogoutHandler)

	// 需要登录的接口
	apiAuth := r.Group("/api")
	apiAuth.Use(handler.SessionAuth())
	apiAuth.GET("/user/storage", handler.UserStorageHandler)
	apiAuth.GET("/user/me", handler.UserMeHandler)
	apiAuth.GET("/files", handler.FileListHandler)
	apiAuth.POST("/files/upload", handler.FileUploadHandler)
	apiAuth.GET("/files/download/:id", handler.FileDownloadHandler)
	apiAuth.DELETE("/files/:id", handler.FileDeleteHandler)
	apiAuth.PUT("/files/:id/rename", handler.FileRenameHandler)
	apiAuth.POST("/folders", handler.CreateFolderHandler)
	apiAuth.PUT("/files/:id/move", handler.FileMoveHandler)
	apiAuth.GET("/files/search", handler.FileSearchHandler)
	apiAuth.GET("/files/preview/:id", handler.FilePreviewHandler)
	apiAuth.POST("/files/multipart/init", handler.MultipartInitHandler)
	apiAuth.POST("/files/multipart/upload", handler.MultipartUploadPartHandler)
	apiAuth.GET("/files/multipart/status", handler.MultipartStatusHandler)
	apiAuth.POST("/files/multipart/complete", handler.MultipartCompleteHandler)
	apiAuth.POST("/files/multipart/refresh-token", handler.MultipartRefreshTokenHandler)

	// 添加临时URL API
	apiAuth.GET("/files/upload-url", handler.GetUploadURLHandler)
	apiAuth.GET("/files/download-url/:id", handler.GetDownloadURLHandler)
	apiAuth.POST("/files/upload-complete", handler.UploadCompleteHandler)

	r.POST("/api/share/public", handler.CreatePublicShareHandler)
	r.GET("/api/share/public", handler.GetPublicShareHandler)
	r.GET("/api/share/:token", handler.AccessShareHandler)
	r.GET("/api/share/download/:token", handler.ShareDownloadHandler)
	r.POST("/api/share/private", handler.CreatePrivateShareHandler)
	r.GET("/api/share/private", handler.GetPrivateShareHandler)
	r.DELETE("/api/share", handler.CancelShareHandler)

	// 注册回收站相关API
	apiAuth.GET("/recycle", handler.RecycleBinListHandler)
	apiAuth.POST("/recycle/restore", handler.RecycleBinRestoreHandler)
	apiAuth.DELETE("/recycle", handler.RecycleBinDeleteHandler)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 服务健康检查端点
	r.GET("/health", handler.HealthCheck)
	r.GET("/api/health", handler.HealthCheck)

	// Prometheus监控端点
	if enableMetrics {
		r.GET("/metrics", gin.WrapH(promhttp.Handler()))
		logger.Info("Prometheus /metrics端点已启用", &logger.LogFields{})
	}

	// 注册服务到ETCD
	etcdEndpoints := strings.Split(*etcdEndpoint, ",")
	serviceInfo := discovery.ServiceInfo{
		Name:        "clouddrive-server",
		Address:     "", // 自动获取
		Port:        *servicePort,
		Version:     "1.0.0",
		StartTime:   time.Now(),
		Environment: viper.GetString("environment"),
		Metadata: map[string]string{
			"api_version": "v1",
		},
		Endpoints: map[string]string{
			"health":  "/api/health",
			"api":     "/api",
			"metrics": "/metrics",
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

	// 优雅关闭
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *servicePort),
		Handler: r,
	}

	// 在goroutine中启动服务器
	go func() {
		log.Printf("服务器启动在端口 %d", *servicePort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("启动服务器失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务器...")

	// 取消监控指标收集器
	cancel()

	// 设置关闭超时
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 优雅关闭
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v", err)
	}

	log.Println("服务器已关闭")
}
