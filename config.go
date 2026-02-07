package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	APIToken   string `json:"api_token"`
	ZoneID     string `json:"zone_id"`
	RecordName string `json:"record_name"`
	RecordType string `json:"record_type"`
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果无法获取用户目录，使用当前目录
		return "config.json"
	}
	return filepath.Join(homeDir, ".go_dns_manager", "config.json")
}

func LoadConfig() *Config {
	configPath := getConfigPath()
	
	// 确保配置目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Printf("警告: 无法创建配置目录: %v\n", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// 文件不存在，返回默认配置
		return &Config{
			RecordType: "A",
		}
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("警告: 配置文件格式错误: %v\n", err)
		return &Config{
			RecordType: "A",
		}
	}

	// 设置默认值
	if config.RecordType == "" {
		config.RecordType = "A"
	}

	return &config
}

func SaveConfig(config *Config) error {
	configPath := getConfigPath()
	
	// 确保配置目录存在
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %v", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	return nil
}

// DeleteConfig 删除配置文件
func DeleteConfig() error {
	configPath := getConfigPath()
	
	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 文件不存在，无需删除
		return nil
	}

	// 删除配置文件
	if err := os.Remove(configPath); err != nil {
		return fmt.Errorf("删除配置文件失败: %v", err)
	}

	return nil
}
