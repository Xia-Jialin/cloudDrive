package main

import (
	"cloudDrive/internal/file"
	"cloudDrive/internal/handler"
	"cloudDrive/internal/storage"
	"cloudDrive/internal/user"
	"flag"
	"fmt"
	"log"
	"net/http"

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
)

// @title CloudDrive API
// @version 1.0
// @description 云盘系统后端 API 文档
// @host localhost:8080
// @BasePath /api

var db *gorm.DB

func main() {
	// 支持通过命令行参数指定配置文件路径
	configPath := flag.String("config", "./configs/config.yaml", "配置文件路径")
	flag.Parse()

	viper.SetConfigFile(*configPath)
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
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

	var err error
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
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")
	redisPoolSize := viper.GetInt("redis.pool_size")

	// 新增 go-redis/v8 客户端初始化
	redisClient := goredis.NewClient(&goredis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       redisDB,
		PoolSize: redisPoolSize,
	})

	store, err := redis.NewStoreWithDB(redisPoolSize, "tcp", redisAddr, "", redisPassword, fmt.Sprintf("%d", redisDB), []byte("secret"))
	if err != nil {
		log.Fatalf("Redis session store 初始化失败: %v", err)
	}
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7天
		HttpOnly: true,
		SameSite: http.SameSiteNoneMode, // 允许跨域携带 cookie
		Secure:   false,                 // 本地开发 false，生产环境建议 true
	})
	r.Use(sessions.Sessions("cloudsession", store))

	// 读取storage配置
	storageType := viper.GetString("storage.type")
	localDir := viper.GetString("storage.local_dir")
	var storageInst interface{} // 用于注入
	switch storageType {
	case "local":
		storageInst = &storage.LocalFileStorage{Dir: localDir}
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

	r.POST("/api/share/public", handler.CreatePublicShareHandler)
	r.GET("/api/share/public", handler.GetPublicShareHandler)
	r.GET("/api/share/:token", handler.AccessShareHandler)
	r.GET("/api/share/download/:token", handler.ShareDownloadHandler)
	r.POST("/api/share/private", handler.CreatePrivateShareHandler)
	r.GET("/api/share/private", handler.GetPrivateShareHandler)
	r.DELETE("/api/share", handler.CancelShareHandler)

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.Run(":8080")
}
