package agent_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gameagent/runtime/internal/agent"
)

func TestLoadConfigFileLoadsPromptConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.json")
	data := []byte(`{
  "turn_timeout_ms": 1000,
  "llm_timeout_ms": 2000,
  "observe_timeout_ms": 3000,
  "action_timeout_ms": 4000,
  "prompt": {
    "language": "Simplified Chinese",
    "npc_style": "quiet mountain hermit",
    "max_speak_chars": 42,
    "tool_instruction": "Use exactly one available tool."
  }
}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := agent.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.TurnTimeout != time.Second {
		t.Fatalf("expected turn timeout 1s, got %s", cfg.TurnTimeout)
	}
	if cfg.Prompt.Language != "Simplified Chinese" {
		t.Fatalf("expected language from config, got %q", cfg.Prompt.Language)
	}
	if cfg.Prompt.NPCStyle != "quiet mountain hermit" {
		t.Fatalf("expected npc style from config, got %q", cfg.Prompt.NPCStyle)
	}
	if cfg.Prompt.MaxSpeakChars != 42 {
		t.Fatalf("expected max speak chars 42, got %d", cfg.Prompt.MaxSpeakChars)
	}
	if cfg.Prompt.ToolInstruction != "Use exactly one available tool." {
		t.Fatalf("expected tool instruction from config, got %q", cfg.Prompt.ToolInstruction)
	}
}

func TestConfigWithDefaultsFillsPromptConfig(t *testing.T) {
	cfg := (agent.Config{}).WithDefaults()

	if cfg.Prompt.Language == "" {
		t.Fatal("expected default prompt language")
	}
	if cfg.Prompt.NPCStyle == "" {
		t.Fatal("expected default npc style")
	}
	if cfg.Prompt.MaxSpeakChars <= 0 {
		t.Fatalf("expected positive default max speak chars, got %d", cfg.Prompt.MaxSpeakChars)
	}
	if cfg.Prompt.ToolInstruction == "" {
		t.Fatal("expected default tool instruction")
	}
}
