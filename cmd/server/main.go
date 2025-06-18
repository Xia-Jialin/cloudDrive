package main

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/handler"
	"cloudDrive/internal/storage"
	"cloudDrive/internal/user"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "cloudDrive/docs"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
	goredis "github.com/go-redis/redis/v8"
	"github.com/spf13/viper"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	goetcd "go.etcd.io/etcd/client/v3"
)

// @title CloudDrive API
// @version 1.0
// @description 云盘系统后端 API 文档
// @host localhost:8080
// @BasePath /api

var db *gorm.DB

// Redis事件驱动分片清理
func StartChunkCleaner(redisClient *goredis.Client, baseDir string) {
	ctx := context.Background()
	pubsub := redisClient.PSubscribe(ctx, "__keyevent@0__:expired")
	log.Println("分片清理监听已启动...")
	for msg := range pubsub.Channel() {
		if strings.HasPrefix(msg.Payload, "upload:") {
			uploadId := strings.TrimPrefix(msg.Payload, "upload:")
			chunkDir := filepath.Join(baseDir, "multipart", uploadId)
			if err := os.RemoveAll(chunkDir); err == nil {
				log.Printf("已自动清理分片目录: %s\n", chunkDir)
			}
		}
	}
}

// 定时兜底分片清理
func CleanOrphanChunks(redisClient *goredis.Client, baseDir string) {
	ctx := context.Background()
	multipartDir := filepath.Join(baseDir, "multipart")
	entries, _ := os.ReadDir(multipartDir)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		uploadId := entry.Name()
		exists, _ := redisClient.Exists(ctx, "upload:"+uploadId).Result()
		if exists == 0 {
			os.RemoveAll(filepath.Join(multipartDir, uploadId))
			log.Printf("定时兜底清理分片目录: %s\n", filepath.Join(multipartDir, uploadId))
		}
	}
}

func StartChunkCleanerCron(redisClient *goredis.Client, baseDir string) {
	go func() {
		for {
			CleanOrphanChunks(redisClient, baseDir)
			time.Sleep(12 * time.Hour)
		}
	}()
}

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
	flag.Parse()

	// 优先用环境变量
	if env := os.Getenv("ETCD_ENDPOINT"); env != "" {
		*etcdEndpoint = env
	}
	if env := os.Getenv("ETCD_KEY"); env != "" {
		*etcdKey = env
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

	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	// 自动迁移用户表和文件表，并捕获错误
	err = db.AutoMigrate(&user.User{}, &file.File{}, &file.FileContent{}, &file.UserRoot{}, &file.Share{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	r := gin.Default()

	// 添加 CORS 跨域中间件，允许携带 cookie
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
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

	// 读取storage配置
	storageType := viper.GetString("storage.type")
	localDir := viper.GetString("storage.local_dir")
	// 读取minio配置
	minioEndpoint := viper.GetString("storage.minio.endpoint")
	minioAccessKey := viper.GetString("storage.minio.access_key")
	minioSecretKey := viper.GetString("storage.minio.secret_key")
	minioBucket := viper.GetString("storage.minio.bucket")
	minioUseSSL := viper.GetBool("storage.minio.use_ssl")
	var storageInst interface{} // 用于注入
	switch storageType {
	case "local":
		storageInst = &storage.LocalFileStorage{Dir: localDir}
	case "minio":
		minioInst, err := storage.NewMinioStorage(minioEndpoint, minioAccessKey, minioSecretKey, minioBucket, minioUseSSL)
		if err != nil {
			log.Fatalf("Minio 初始化失败: %v", err)
		}
		storageInst = minioInst
	// 未来可扩展更多类型，如oss、ftp等
	default:
		log.Fatalf("不支持的存储类型: %s", storageType)
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

	// 新增分片清理监听和定时兜底
	go StartChunkCleaner(redisClient, localDir)
	StartChunkCleanerCron(redisClient, localDir)

	r.Run(":8080")
}
