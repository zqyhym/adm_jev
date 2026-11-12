package config

import (
	"os"
	"strings"
)

// Config 是进程启动时读取的基础设施配置。
type Config struct {
	MongoURI      string
	MongoDatabase string
}

// Load 读取 Mongo 地址和库名。未设置时使用本机默认值。
func Load() Config {
	uri := strings.TrimSpace(os.Getenv("ADM_JEV_MONGO_URI"))
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}
	database := strings.TrimSpace(os.Getenv("ADM_JEV_MONGO_DATABASE"))
	if database == "" {
		database = "adm_jev"
	}
	return Config{MongoURI: uri, MongoDatabase: database}
}
