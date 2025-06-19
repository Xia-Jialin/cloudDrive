package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Config 存储服务配置结构
type Config struct {
	Server struct {
		GRPCPort      int   `mapstructure:"grpc_port"`
		HTTPPort      int   `mapstructure:"http_port"`
		UploadMaxSize int64 `mapstructure:"upload_max_size"`
	} `mapstructure:"server"`

	Redis struct {
		Addr     string `mapstructure:"addr"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db"`
		PoolSize int    `mapstructure:"pool_size"`
	} `mapstructure:"redis"`

	Storage struct {
		Type     string `mapstructure:"type"`
		LocalDir string `mapstructure:"local_dir"`
		Minio    struct {
			Endpoint  string `mapstructure:"endpoint"`
			AccessKey string `mapstructure:"access_key"`
			SecretKey string `mapstructure:"secret_key"`
			Bucket    string `mapstructure:"bucket"`
			UseSSL    bool   `mapstructure:"use_ssl"`
		} `mapstructure:"minio"`
	} `mapstructure:"storage"`

	Security struct {
		JWTSecret string `mapstructure:"jwt_secret"`
	} `mapstructure:"security"`
}

// LoadConfig 从配置文件或ETCD加载配置
func LoadConfig(configPath, etcdEndpoint, etcdKey string) (*Config, error) {
	var config Config

	// 优先从etcd加载配置
	configBytes, err := loadConfigFromEtcd(etcdEndpoint, etcdKey)
	if err != nil {
		log.Printf("从etcd获取配置失败: %v，尝试读取本地配置文件...", err)
		viper.SetConfigFile(configPath)
		err = viper.ReadInConfig()
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %v", err)
		}
	} else {
		viper.SetConfigType("yaml")
		err = viper.ReadConfig(strings.NewReader(string(configBytes)))
		if err != nil {
			return nil, fmt.Errorf("解析etcd配置失败: %v", err)
		}
	}

	// 绑定配置到结构体
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("解析配置失败: %v", err)
	}

	return &config, nil
}

// 从etcd加载配置
func loadConfigFromEtcd(endpoint, key string) ([]byte, error) {
	cli, err := clientv3.New(clientv3.Config{
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

// EnsureDirectories 确保必要的目录存在
func EnsureDirectories(config *Config) error {
	if config.Storage.Type == "local" {
		// 确保上传目录存在
		if err := os.MkdirAll(config.Storage.LocalDir, 0755); err != nil {
			return fmt.Errorf("创建上传目录失败: %v", err)
		}

		// 确保分片目录存在
		multipartDir := fmt.Sprintf("%s/multipart", config.Storage.LocalDir)
		if err := os.MkdirAll(multipartDir, 0755); err != nil {
			return fmt.Errorf("创建分片目录失败: %v", err)
		}
	}
	return nil
}
