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

	defaultMemoryEnabled = true
)

type Config struct {
	TurnTimeout            time.Duration
	LLMTimeout             time.Duration
	ObserveTimeout         time.Duration
	ActionTimeout          time.Duration
	MemoryEnabled          *bool
	RecentMemoryLimit      int
	MemoryContextSizeLimit int
	Prompt                 PromptConfig
}

type fileConfig struct {
	TurnTimeoutMS          int64        `json:"turn_timeout_ms"`
	LLMTimeoutMS           int64        `json:"llm_timeout_ms"`
	ObserveTimeoutMS       int64        `json:"observe_timeout_ms"`
	ActionTimeoutMS        int64        `json:"action_timeout_ms"`
	MemoryEnabled          *bool        `json:"memory_enabled"`
	RecentMemoryLimit      int          `json:"recent_memory_limit"`
	MemoryContextSizeLimit int          `json:"memory_context_size_limit"`
	Prompt                 PromptConfig `json:"prompt"`
}

type PromptConfig struct {
	Language        string `json:"language"`
	NPCStyle        string `json:"npc_style"`
	MaxSpeakChars   int    `json:"max_speak_chars"`
	ToolInstruction string `json:"tool_instruction"`
}

// DefaultConfig 返回 Agent Runtime 的默认运行配置。
// Phase4 默认开启短期 Memory，但保留开关，方便回退到 Phase3 的 one-turn 行为。
func DefaultConfig() Config {
	return Config{
		TurnTimeout:            15 * time.Second,
		LLMTimeout:             8 * time.Second,
		ObserveTimeout:         3 * time.Second,
		ActionTimeout:          3 * time.Second,
		MemoryEnabled:          boolPtr(defaultMemoryEnabled),
		RecentMemoryLimit:      5,
		MemoryContextSizeLimit: 4096,
		Prompt: PromptConfig{
			Language:        "Simplified Chinese",
			NPCStyle:        "自然、简短、符合当前游戏 NPC 的语气",
			MaxSpeakChars:   60,
			ToolInstruction: "Use exactly one available tool. Prefer speak for dialogue; use emote only for clear emotional reactions.",
		},
	}
}

// ConfigPathFromEnv 解析 Agent 配置文件路径。
// GAMEAGENT_AGENT_CONFIG 用于本地覆盖默认配置，未设置时读取 runtime/config/agent.json。
func ConfigPathFromEnv() string {
	if path := os.Getenv(configPathEnv); path != "" {
		return path
	}
	return defaultConfigPath
}

// LoadConfigFile 读取并解析 Agent 配置文件。
// 配置文件不存在时使用默认值，避免本地最小启动流程依赖额外文件。
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

	cfg := Config{
		MemoryEnabled:          raw.MemoryEnabled,
		TurnTimeout:            durationMS(raw.TurnTimeoutMS),
		LLMTimeout:             durationMS(raw.LLMTimeoutMS),
		ObserveTimeout:         durationMS(raw.ObserveTimeoutMS),
		ActionTimeout:          durationMS(raw.ActionTimeoutMS),
		RecentMemoryLimit:      raw.RecentMemoryLimit,
		MemoryContextSizeLimit: raw.MemoryContextSizeLimit,
		Prompt:                 raw.Prompt,
	}.WithDefaults()

	return cfg, nil
}

// MemoryEnabledValue 返回最终生效的 MemoryEnabled。
// 使用指针是为了区分“配置里没写”和“显式写 false”。
func (c Config) MemoryEnabledValue() bool {
	if c.MemoryEnabled == nil {
		return defaultMemoryEnabled
	}
	return *c.MemoryEnabled
}

// WithDefaults 为 PromptConfig 补齐缺省字段。
// 这样配置文件可以只覆盖关心的 prompt 片段。
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

// WithDefaults 为 Agent Config 补齐缺省字段。
// Phase4 的 Memory 配置也在这里归一化，避免 Loop 初始化时处理零值分支。
func (c Config) WithDefaults() Config {
	defaults := DefaultConfig()
	if c.MemoryEnabled == nil {
		c.MemoryEnabled = boolPtr(defaults.MemoryEnabledValue())
	}

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
	if c.RecentMemoryLimit <= 0 {
		c.RecentMemoryLimit = defaults.RecentMemoryLimit
	}
	if c.MemoryContextSizeLimit <= 0 {
		c.MemoryContextSizeLimit = defaults.MemoryContextSizeLimit
	}
	c.Prompt = c.Prompt.WithDefaults()
	return c
}

// durationMS 将配置文件中的毫秒值转换成 time.Duration。
func durationMS(v int64) time.Duration {
	if v <= 0 {
		return 0
	}
	return time.Duration(v) * time.Millisecond
}

// boolPtr 帮助构造可区分“未配置/显式配置”的 bool 指针。
func boolPtr(v bool) *bool {
	return &v
}
