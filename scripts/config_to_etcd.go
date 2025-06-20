package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var (
	configFile   = flag.String("config", "", "配置文件路径")
	etcdEndpoint = flag.String("etcd", "localhost:2379", "ETCD服务器地址")
	etcdKey      = flag.String("key", "", "ETCD中的配置键")
)

func main() {
	flag.Parse()

	// 检查必要参数
	if *configFile == "" {
		log.Fatal("请指定配置文件路径 --config")
	}
	if *etcdKey == "" {
		log.Fatal("请指定ETCD键 --key")
	}

	// 读取配置文件
	configData, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatalf("读取配置文件失败: %v", err)
	}

	// 连接ETCD
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{*etcdEndpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		log.Fatalf("连接ETCD失败: %v", err)
	}
	defer cli.Close()

	// 写入配置到ETCD
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Put(ctx, *etcdKey, string(configData))
	if err != nil {
		log.Fatalf("写入配置到ETCD失败: %v", err)
	}

	fmt.Printf("成功将配置文件 %s 写入ETCD键 %s\n", *configFile, *etcdKey)
}
