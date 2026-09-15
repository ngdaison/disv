package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type MySQLConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Config struct {
	BotToken      string      `json:"bot_token"`
	RapidAPIKey   string      `json:"rapid_api_key"`
	AIAPIKey      string      `json:"ai_api_key"`
	ChatAIToken   string      `json:"chatai_token"`
	BotPrefix     string      `json:"bot_prefix"`
	StatusMessage string      `json:"status_message"`
	MySQL         MySQLConfig `json:"mysql"`
}

func LoadConfig() (*Config, error) {
	possiblePaths := []string{
		"config.json",
		"../config.json",
		filepath.Join(".", "config.json"),
	}

	var data []byte
	var err error
	var foundPath string

	for _, p := range possiblePaths {
		data, err = os.ReadFile(p)
		if err == nil {
			foundPath = p
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("không tìm thấy file config.json ở các đường dẫn dự phòng: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("lỗi phân tích cú pháp json từ file %s: %w", foundPath, err)
	}

	if cfg.BotToken == "" {
		return nil, fmt.Errorf("bot_token không được để trống trong %s", foundPath)
	}

	return &cfg, nil
}
