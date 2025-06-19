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

	// 读取块存储服务配置
	chunkServerEnabled := viper.GetBool("storage.chunk_server.enabled")
	chunkServerURL := viper.GetString("storage.chunk_server.url")
	chunkServerTempDir := viper.GetString("storage.chunk_server.temp_dir")

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

	// 初始化块存储服务客户端
	chunkStorage, err := storage.NewChunkServerStorage(chunkServerURL, redisClient, chunkServerTempDir)
	if err != nil {
		log.Fatalf("初始化块存储服务客户端失败: %v", err)
	}

	storageInst = chunkStorage
	log.Printf("已连接到块存储服务: %s", chunkServerURL)

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

	r.Run(":8080")
}
