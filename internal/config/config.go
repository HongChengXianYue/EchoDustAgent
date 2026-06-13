package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultBaseURL  = "https://api.openai.com/v1"
	defaultModel    = "gpt-4.1-mini"
	defaultMaxSteps = 10
)

type Config struct {
	APIKey   string
	BaseURL  string
	Model    string
	MaxSteps int
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		APIKey:   strings.TrimSpace(os.Getenv("AGENT_API_KEY")),
		BaseURL:  strings.TrimSpace(os.Getenv("AGENT_BASE_URL")),
		Model:    strings.TrimSpace(os.Getenv("AGENT_MODEL")),
		MaxSteps: defaultMaxSteps,
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_MAX_STEPS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("AGENT_MAX_STEPS must be a positive integer")
		}
		cfg.MaxSteps = n
	}
	if cfg.APIKey == "" {
		return Config{}, fmt.Errorf("AGENT_API_KEY is required")
	}
	return cfg, nil
}
