package config

import "testing"

// TestLoadDefaults 验证未设置环境变量时使用本机 Mongo 和 adm_jev 库。
func TestLoadDefaults(t *testing.T) {
	t.Setenv("ADM_JEV_MONGO_URI", "")
	t.Setenv("ADM_JEV_MONGO_DATABASE", "")
	cfg := Load()
	if cfg.MongoURI != "mongodb://127.0.0.1:27017" || cfg.MongoDatabase != "adm_jev" {
		t.Fatalf("uri=%s database=%s", cfg.MongoURI, cfg.MongoDatabase)
	}
}

// TestLoadFromEnv 验证环境变量覆盖 Mongo 地址和库名，并去掉两端空白。
func TestLoadFromEnv(t *testing.T) {
	t.Setenv("ADM_JEV_MONGO_URI", " mongodb://10.0.0.8:27017 ")
	t.Setenv("ADM_JEV_MONGO_DATABASE", " adm_jev_test ")
	cfg := Load()
	if cfg.MongoURI != "mongodb://10.0.0.8:27017" || cfg.MongoDatabase != "adm_jev_test" {
		t.Fatalf("uri=%s database=%s", cfg.MongoURI, cfg.MongoDatabase)
	}
}
