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
	Prompt         PromptConfig
}

type fileConfig struct {
	TurnTimeoutMS    int64        `json:"turn_timeout_ms"`
	LLMTimeoutMS     int64        `json:"llm_timeout_ms"`
	ObserveTimeoutMS int64        `json:"observe_timeout_ms"`
	ActionTimeoutMS  int64        `json:"action_timeout_ms"`
	Prompt           PromptConfig `json:"prompt"`
}

type PromptConfig struct {
	Language        string `json:"language"`
	NPCStyle        string `json:"npc_style"`
	MaxSpeakChars   int    `json:"max_speak_chars"`
	ToolInstruction string `json:"tool_instruction"`
}

func DefaultConfig() Config {
	return Config{
		TurnTimeout:    15 * time.Second,
		LLMTimeout:     8 * time.Second,
		ObserveTimeout: 3 * time.Second,
		ActionTimeout:  3 * time.Second,
		Prompt: PromptConfig{
			Language:        "Simplified Chinese",
			NPCStyle:        "自然、简短、符合当前游戏 NPC 的语气",
			MaxSpeakChars:   60,
			ToolInstruction: "Use exactly one available tool. Prefer speak for dialogue; use emote only for clear emotional reactions.",
		},
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
		Prompt:         raw.Prompt,
	}.WithDefaults(), nil
}

func (p PromptConfig) WithDefaults() PromptConfig {
	defaults := DefaultConfig().Prompt

	if p.Language == "" {
		p.Language = defaults.Language
	}
	if p.NPCStyle == "" {
		p.NPCStyle = defaults.NPCStyle
	}
	if p.MaxSpeakChars <= 0 {
		p.MaxSpeakChars = defaults.MaxSpeakChars
	}
	if p.ToolInstruction == "" {
		p.ToolInstruction = defaults.ToolInstruction
	}

	return p
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
	c.Prompt = c.Prompt.WithDefaults()
	return c
}

func durationMS(v int64) time.Duration {
	if v <= 0 {
		return 0
	}
	return time.Duration(v) * time.Millisecond
}
