package agent_test

import (
	"os"
	"path/filepath"
	"strings"
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
  "action_start_timeout_ms": 5000,
  "async_action_timeout_ms": 6000,
  "memory_enabled": true,
  "recent_memory_limit": 7,
  "memory_context_size_limit": 2048,
  "max_steps": 5,
  "max_tool_calls_per_step": 3,
  "max_tool_calls_per_turn": 9,
  "max_async_actions_per_turn": 2,
  "max_parallel_tool_calls": 2,
  "max_tool_result_output_bytes": 4096,
  "max_tool_result_output_depth": 3,
  "max_tool_result_output_fields": 16,
  "max_tool_result_output_array_items": 8,
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
	if cfg.ActionStartTimeout != 5*time.Second {
		t.Fatalf("expected action start timeout 5s, got %s", cfg.ActionStartTimeout)
	}
	if cfg.AsyncActionTimeout != 6*time.Second {
		t.Fatalf("expected async action timeout 6s, got %s", cfg.AsyncActionTimeout)
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
	if !cfg.MemoryEnabledValue() {
		t.Fatal("expected memory enabled from config")
	}
	if cfg.RecentMemoryLimit != 7 {
		t.Fatalf("expected recent memory limit 7, got %d", cfg.RecentMemoryLimit)
	}
	if cfg.MemoryContextSizeLimit != 2048 {
		t.Fatalf("expected memory context size limit 2048, got %d", cfg.MemoryContextSizeLimit)
	}
	if cfg.MaxSteps != 5 {
		t.Fatalf("expected max steps 5, got %d", cfg.MaxSteps)
	}
	if cfg.MaxToolCallsPerStep != 3 {
		t.Fatalf("expected max tool calls per step 3, got %d", cfg.MaxToolCallsPerStep)
	}
	if cfg.MaxToolCallsPerTurn != 9 {
		t.Fatalf("expected max tool calls per turn 9, got %d", cfg.MaxToolCallsPerTurn)
	}
	if cfg.MaxAsyncActionsPerTurn != 2 {
		t.Fatalf("expected max async actions per turn 2, got %d", cfg.MaxAsyncActionsPerTurn)
	}
	if cfg.MaxParallelToolCalls != 2 {
		t.Fatalf("expected max parallel tool calls 2, got %d", cfg.MaxParallelToolCalls)
	}
	if cfg.MaxToolResultOutputBytes != 4096 {
		t.Fatalf("expected max tool result output bytes 4096, got %d", cfg.MaxToolResultOutputBytes)
	}
	if cfg.MaxToolResultOutputDepth != 3 {
		t.Fatalf("expected max tool result output depth 3, got %d", cfg.MaxToolResultOutputDepth)
	}
	if cfg.MaxToolResultOutputFields != 16 {
		t.Fatalf("expected max tool result output fields 16, got %d", cfg.MaxToolResultOutputFields)
	}
	if cfg.MaxToolResultOutputArrayItems != 8 {
		t.Fatalf("expected max tool result output array items 8, got %d", cfg.MaxToolResultOutputArrayItems)
	}
}

func TestLoadConfigFileDefaultsMemoryEnabledWhenFieldOmitted(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.json")
	data := []byte(`{
  "turn_timeout_ms": 1000,
  "prompt": {
    "language": "Simplified Chinese"
  }
}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := agent.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if !cfg.MemoryEnabledValue() {
		t.Fatal("expected memory to be enabled by default when memory_enabled is omitted")
	}
}

func TestConfigWithDefaultsPreservesExplicitMemoryDisabled(t *testing.T) {
	cfg := (agent.Config{
		MemoryEnabled: boolPtr(false),
	}).WithDefaults()

	if cfg.MemoryEnabledValue() {
		t.Fatal("expected explicit memory disabled to survive WithDefaults")
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
	if !cfg.MemoryEnabledValue() {
		t.Fatal("expected memory to be enabled by default")
	}
	if cfg.RecentMemoryLimit <= 0 {
		t.Fatalf("expected positive default recent memory limit, got %d", cfg.RecentMemoryLimit)
	}
	if cfg.MemoryContextSizeLimit <= 0 {
		t.Fatalf("expected positive default memory context size limit, got %d", cfg.MemoryContextSizeLimit)
	}
}

func TestDefaultToolInstructionDoesNotNameGameSpecificTools(t *testing.T) {
	cfg := agent.DefaultConfig()
	instruction := cfg.Prompt.ToolInstruction

	for _, forbidden := range gameSpecificToolInstructionTerms() {
		if strings.Contains(instruction, forbidden) {
			t.Fatalf("default tool instruction should not mention game-specific term %q: %q", forbidden, instruction)
		}
	}
}

func TestDefaultToolInstructionKeepsGenericToolGuidance(t *testing.T) {
	cfg := agent.DefaultConfig()
	instruction := cfg.Prompt.ToolInstruction

	for _, want := range []string{"available tools", "descriptions", "input schemas", "settle"} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("expected default tool instruction to mention %q, got %q", want, instruction)
		}
	}
}

func TestBundledToolInstructionDoesNotNameGameSpecificTools(t *testing.T) {
	cfg, err := agent.LoadConfigFile(filepath.Join("..", "..", "config", "agent.json"))
	if err != nil {
		t.Fatalf("load bundled config: %v", err)
	}

	for _, forbidden := range gameSpecificToolInstructionTerms() {
		if strings.Contains(cfg.Prompt.ToolInstruction, forbidden) {
			t.Fatalf("bundled tool instruction should not mention game-specific term %q: %q", forbidden, cfg.Prompt.ToolInstruction)
		}
	}
}

func gameSpecificToolInstructionTerms() []string {
	return []string{
		"present_dialogue",
		"player_said_to_npc",
		"reply_options",
		"allow_free_text",
		"speak",
		"emote",
		"face_player",
	}
}

func TestConfigLoadsPhase5Budgets(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "agent.json")
	data := []byte(`{
  "max_steps": 5,
  "max_tool_calls_per_step": 4,
  "max_tool_calls_per_turn": 8,
  "max_parallel_tool_calls": 3,
  "max_tool_result_output_bytes": 1234,
  "max_tool_result_output_depth": 5,
  "max_tool_result_output_fields": 32,
  "max_tool_result_output_array_items": 12
}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := agent.LoadConfigFile(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.MaxSteps != 5 ||
		cfg.MaxToolCallsPerStep != 4 ||
		cfg.MaxToolCallsPerTurn != 8 ||
		cfg.MaxParallelToolCalls != 3 ||
		cfg.MaxToolResultOutputBytes != 1234 ||
		cfg.MaxToolResultOutputDepth != 5 ||
		cfg.MaxToolResultOutputFields != 32 ||
		cfg.MaxToolResultOutputArrayItems != 12 {
		t.Fatalf("phase5 budgets not loaded: %+v", cfg)
	}
}

func TestConfigDefaultsPhase5BudgetsWhenMissingZeroOrNegative(t *testing.T) {
	cfg := (agent.Config{
		MaxSteps:                      -1,
		MaxToolCallsPerStep:           0,
		MaxToolCallsPerTurn:           -2,
		MaxAsyncActionsPerTurn:        0,
		MaxParallelToolCalls:          0,
		MaxToolResultOutputBytes:      -1,
		MaxToolResultOutputDepth:      0,
		MaxToolResultOutputFields:     -1,
		MaxToolResultOutputArrayItems: 0,
	}).WithDefaults()

	if cfg.MaxSteps != 3 {
		t.Fatalf("MaxSteps = %d, want 3", cfg.MaxSteps)
	}
	if cfg.MaxToolCallsPerStep != 4 {
		t.Fatalf("MaxToolCallsPerStep = %d, want 4", cfg.MaxToolCallsPerStep)
	}
	if cfg.MaxToolCallsPerTurn != 6 {
		t.Fatalf("MaxToolCallsPerTurn = %d, want 6", cfg.MaxToolCallsPerTurn)
	}
	if cfg.MaxAsyncActionsPerTurn != 1 {
		t.Fatalf("MaxAsyncActionsPerTurn = %d, want 1", cfg.MaxAsyncActionsPerTurn)
	}
	if cfg.MaxParallelToolCalls != 4 {
		t.Fatalf("MaxParallelToolCalls = %d, want 4", cfg.MaxParallelToolCalls)
	}
	if cfg.MaxToolResultOutputBytes != 8192 {
		t.Fatalf("MaxToolResultOutputBytes = %d, want 8192", cfg.MaxToolResultOutputBytes)
	}
	if cfg.MaxToolResultOutputDepth != 4 {
		t.Fatalf("MaxToolResultOutputDepth = %d, want 4", cfg.MaxToolResultOutputDepth)
	}
	if cfg.MaxToolResultOutputFields != 64 {
		t.Fatalf("MaxToolResultOutputFields = %d, want 64", cfg.MaxToolResultOutputFields)
	}
	if cfg.MaxToolResultOutputArrayItems != 32 {
		t.Fatalf("MaxToolResultOutputArrayItems = %d, want 32", cfg.MaxToolResultOutputArrayItems)
	}
	if cfg.ActionStartTimeout != 3*time.Second {
		t.Fatalf("ActionStartTimeout = %s, want 3s", cfg.ActionStartTimeout)
	}
	if cfg.AsyncActionTimeout != 45*time.Second {
		t.Fatalf("AsyncActionTimeout = %s, want 45s", cfg.AsyncActionTimeout)
	}
	if cfg.TurnTimeout != 90*time.Second {
		t.Fatalf("TurnTimeout = %s, want 90s", cfg.TurnTimeout)
	}
}

func TestConfigPhase6DefaultTurnTimeoutCoversAsyncBudget(t *testing.T) {
	cfg := agent.DefaultConfig()
	worstCase := cfg.ObserveTimeout +
		time.Duration(cfg.MaxSteps)*cfg.LLMTimeout +
		cfg.ActionStartTimeout +
		cfg.AsyncActionTimeout +
		cfg.ObserveTimeout +
		cfg.LLMTimeout +
		cfg.ActionTimeout

	if cfg.TurnTimeout <= worstCase {
		t.Fatalf("TurnTimeout = %s, want greater than worst case %s", cfg.TurnTimeout, worstCase)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
