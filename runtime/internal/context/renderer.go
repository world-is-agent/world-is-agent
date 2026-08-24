package context

import (
	"fmt"
	"strings"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type RendererConfig struct {
	MemoryContextSizeLimit int
}

type Renderer struct {
	config RendererConfig
}

// NewRenderer 创建 AgentContext Renderer。
// Renderer 负责把结构化上下文变成 Provider 可以消费的模型请求。
func NewRenderer(config RendererConfig) Renderer {
	return Renderer{config: config}
}

// Render 负责把 AgentContext 转成 Provider Request。
// Renderer 在这里固定 Current Observation 优先于 Recent Memory 的上下文语义。
func (r Renderer) Render(agentContext AgentContext) (model.Request, error) {
	if agentContext.Event == nil {
		return model.Request{}, fmt.Errorf("%w: event is required", ErrInvalidInput)
	}
	if agentContext.Observation == nil {
		return model.Request{}, fmt.Errorf("%w: observation is required", ErrInvalidInput)
	}

	return model.Request{
		System: agentContext.RuntimePolicy,
		Messages: []model.Message{
			{
				Role:    model.RoleUser,
				Content: r.renderUserMessage(agentContext),
			},
		},
		Tools: append([]model.ToolDefinition(nil), agentContext.Tools...),
	}, nil
}

// renderUserMessage 渲染本轮模型输入的 user message。
// 它把 Recent Memory、Current Event 和 Current Observation 放进同一个可读上下文块。
func (r Renderer) renderUserMessage(agentContext AgentContext) string {
	return fmt.Sprintf(`[Recent Memory]
%s

[Agent Descriptor]
%s

[Current Event]
%s

[Current Observation]
%s

[Instruction]
Current Observation is the current truth.
Recent Memory is historical context.
If Recent Memory conflicts with Current Observation, follow Current Observation.
If Recent Memory is from today and current game time has not clearly advanced much, treat it as nearby conversation context, not proof that the player left and returned.

Return a tool call only.
`,
		r.renderMemories(agentContext.RecentMemories, currentGameTime(agentContext)),
		renderAgentDescriptor(agentContext.AgentDescriptor),
		protoToJSON(agentContext.Event),
		protoToJSON(agentContext.Observation),
	)
}

func renderAgentDescriptor(descriptor AgentDescriptor) string {
	definitionID := strings.TrimSpace(descriptor.DefinitionID)
	if definitionID == "" {
		definitionID = "(unspecified)"
	}
	return fmt.Sprintf("entity_id: %s\ndefinition_id: %s", descriptor.EntityID, definitionID)
}

// renderMemories 渲染 Recent Memory section。
// 没有可用 Memory 时显式输出 (none)，让模型知道不是遗漏上下文。
func (r Renderer) renderMemories(records []memory.Record, currentTime *memory.GameTimeSnapshot) string {
	records = trimMemories(records, r.config.MemoryContextSizeLimit, currentTime)
	if len(records) == 0 {
		return "(none)"
	}

	lines := make([]string, 0, len(records))
	for _, record := range records {
		lines = append(lines, renderMemory(record, currentTime))
	}
	return strings.Join(lines, "\n")
}

// trimMemories 按 soft budget 裁剪 Recent Memory。
// Phase4 优先保留最新 Memory；如果最新一条本身超限，仍保留它。
func trimMemories(records []memory.Record, limit int, currentTime *memory.GameTimeSnapshot) []memory.Record {
	if len(records) == 0 || limit <= 0 {
		return records
	}

	start := len(records) - 1
	rendered := renderMemory(records[start], currentTime)
	for start > 0 {
		next := renderMemory(records[start-1], currentTime)
		if len([]byte(next+"\n"+rendered)) > limit {
			break
		}
		start--
		rendered = next + "\n" + rendered
	}

	out := make([]memory.Record, len(records[start:]))
	copy(out, records[start:])
	return out
}

// renderMemory 将单条 MemoryRecord 投影为模型可读的短摘要。
// 存储字段用于 Runtime 追踪；模型只需要“何时 + 可见动作”的连续性信号。
func renderMemory(record memory.Record, currentTime *memory.GameTimeSnapshot) string {
	return fmt.Sprintf("- %s: %s", gameTimeRelation(record.GameTime, currentTime), visibleActionSummary(record.Outcome))
}

func visibleActionSummary(outcome memory.TurnOutcome) string {
	switch strings.ToLower(strings.TrimSpace(outcome.ToolName)) {
	case "speak":
		if text := stringArgument(outcome.ToolArguments, "text"); text != "" {
			return fmt.Sprintf("said %q", text)
		}
		return "spoke"
	case "emote":
		if emote := stringArgument(outcome.ToolArguments, "emote"); emote != "" {
			return fmt.Sprintf("used emote %q", emote)
		}
		return "used emote"
	default:
		tool := strings.TrimSpace(outcome.ToolName)
		if tool == "" {
			return "completed a visible action"
		}
		return fmt.Sprintf("used tool %q", tool)
	}
}

func stringArgument(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func currentGameTime(agentContext AgentContext) *memory.GameTimeSnapshot {
	if snapshot := gameTimeSnapshot(agentContext.Event.GetGameTime()); snapshot != nil {
		return snapshot
	}
	return gameTimeSnapshot(agentContext.Observation.GetGameTime())
}

func gameTimeSnapshot(gameTime *protocolv1alpha2.GameTime) *memory.GameTimeSnapshot {
	if gameTime == nil {
		return nil
	}
	return &memory.GameTimeSnapshot{
		Year:   gameTime.GetYear(),
		Season: gameTime.GetSeason(),
		Day:    gameTime.GetDay(),
		Hour:   gameTime.GetHour(),
		Minute: gameTime.GetMinute(),
		Tick:   gameTime.GetTick(),
	}
}

func gameTimeRelation(memoryTime, currentTime *memory.GameTimeSnapshot) string {
	if memoryTime == nil || currentTime == nil {
		return "previous interaction"
	}
	if sameGameDay(memoryTime, currentTime) {
		return fmt.Sprintf("today %02d:%02d", memoryTime.Hour, memoryTime.Minute)
	}
	return fmt.Sprintf("previous day %s", formatGameTime(memoryTime))
}

func sameGameDay(left, right *memory.GameTimeSnapshot) bool {
	return left.Year == right.Year &&
		left.Season == right.Season &&
		left.Day == right.Day
}

func formatGameTime(gameTime *memory.GameTimeSnapshot) string {
	return fmt.Sprintf("Y%d S%d D%d %02d:%02d", gameTime.Year, gameTime.Season, gameTime.Day, gameTime.Hour, gameTime.Minute)
}

// protoToJSON 把 protobuf message 转成缩进 JSON。
// 渲染失败时返回空对象，避免上下文渲染阶段因为展示问题中断 Turn。
func protoToJSON(message proto.Message) string {
	if message == nil {
		return "{}"
	}

	data, err := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}.Marshal(message)
	if err != nil {
		return "{}"
	}
	return string(data)
}
