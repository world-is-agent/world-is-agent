package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

const (
	defaultConfigPath = "runtime/config/agent.json"
	configPathEnv     = "GAMEAGENT_AGENT_CONFIG"
)

type Config struct {
	TurnTimeout    time.Duration
	LLMTimeout     time.Duration
	ObserveTimeout time.Duration
	ActionTimeout  time.Duration
}

type fileConfig struct {
	TurnTimeoutMS    int64 `json:"turn_timeout_ms"`
	LLMTimeoutMS     int64 `json:"llm_timeout_ms"`
	ObserveTimeoutMS int64 `json:"observe_timeout_ms"`
	ActionTimeoutMS  int64 `json:"action_timeout_ms"`
}

func DefaultConfig() Config {
	return Config{
		TurnTimeout:    15 * time.Second,
		LLMTimeout:     8 * time.Second,
		ObserveTimeout: 3 * time.Second,
		ActionTimeout:  3 * time.Second,
	}
}

func ConfigPathFromEnv() string {
	if path := os.Getenv(configPathEnv); path != "" {
		return path
	}
	return defaultConfigPath
}

func LoadConfigFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("read agent config: %w", err)
	}

	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parse agent config: %w", err)
	}

	return Config{
		TurnTimeout:    durationMS(raw.TurnTimeoutMS),
		LLMTimeout:     durationMS(raw.LLMTimeoutMS),
		ObserveTimeout: durationMS(raw.ObserveTimeoutMS),
		ActionTimeout:  durationMS(raw.ActionTimeoutMS),
	}.WithDefaults(), nil
}

func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()

	if c.TurnTimeout <= 0 {
		c.TurnTimeout = defaults.TurnTimeout
	}
	if c.LLMTimeout <= 0 {
		c.LLMTimeout = defaults.LLMTimeout
	}
	if c.ObserveTimeout <= 0 {
		c.ObserveTimeout = defaults.ObserveTimeout
	}
	if c.ActionTimeout <= 0 {
		c.ActionTimeout = defaults.ActionTimeout
	}

	return c
}

func durationMS(v int64) time.Duration {
	if v <= 0 {
		return 0
	}
	return time.Duration(v) * time.Millisecond
}
