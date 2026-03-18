package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig    `mapstructure:"server"`
	ATSNodes []ATSNodeConfig `mapstructure:"ats_nodes"`
}

type ServerConfig struct {
	HTTP  HTTPConfig  `mapstructure:"http"`
	HTTPS HTTPSConfig `mapstructure:"https"`
}

type HTTPConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type HTTPSConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

type ATSNodeConfig struct {
	Name     string  `mapstructure:"name"`
	Domain   string  `mapstructure:"domain"`
	StatsURL string  `mapstructure:"stats_url"`
	MaxConns float64 `mapstructure:"max_conns"`
}

func Load() *Config {
	setupViper()

	if configPath := os.Getenv("SCHEDULER_CONFIG"); configPath != "" {
		viper.SetConfigFile(configPath)
	}

	if err := viper.ReadInConfig(); err != nil {
		if errors.Is(err, viper.ConfigFileNotFoundError{}) {
			log.Printf("未找到配置文件，使用默認值與環境變數")
		} else {
			log.Fatalf("讀取配置文件失敗: %v", err)
		}
	} else {
		log.Printf("已加載配置文件: %s", viper.ConfigFileUsed())
	}

	cfg := &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		log.Fatalf("解析配置失敗: %v", err)
	}

	validateConfig(cfg)
	return cfg
}

func (c *Config) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.HTTP.Host, c.Server.HTTP.Port)
}

func (c *Config) HTTPSAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.HTTPS.Host, c.Server.HTTPS.Port)
}

func setupViper() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("SCHEDULER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("server.http.host", "0.0.0.0")
	viper.SetDefault("server.http.port", 80)
	viper.SetDefault("server.https.host", "0.0.0.0")
	viper.SetDefault("server.https.port", 443)
	viper.SetDefault("server.https.cert_file", "server.crt")
	viper.SetDefault("server.https.key_file", "server.key")
	viper.SetDefault("ats_nodes", []map[string]any{
		{
			"name":      "ats1",
			"domain":    "127.0.0.1:8081",
			"stats_url": "http://127.0.0.1:8081/stats",
			"max_conns": 1000,
		},
		{
			"name":      "ats2",
			"domain":    "127.0.0.1:8082",
			"stats_url": "http://127.0.0.1:8082/stats",
			"max_conns": 1000,
		},
		{
			"name":      "ats3",
			"domain":    "127.0.0.1:8083",
			"stats_url": "http://127.0.0.1:8083/stats",
			"max_conns": 1000,
		},
	})
}

func validateConfig(cfg *Config) {
	if len(cfg.ATSNodes) == 0 {
		log.Fatalf("配置錯誤: ats_nodes 不能為空")
	}

	for i, node := range cfg.ATSNodes {
		if node.Name == "" || node.Domain == "" || node.StatsURL == "" || node.MaxConns <= 0 {
			log.Fatalf("配置錯誤: ats_nodes[%d] 欄位不完整或 max_conns 非法", i)
		}
	}
}
