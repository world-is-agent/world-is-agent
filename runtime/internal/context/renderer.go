package context

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/memory"
	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type RendererConfig struct {
	MemoryContextSizeLimit        int
	MaxToolResultOutputBytes      int
	MaxToolResultOutputDepth      int
	MaxToolResultOutputFields     int
	MaxToolResultOutputArrayItems int
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

	messages := []model.Message{
		{
			Role:    model.RoleUser,
			Content: r.renderUserMessage(agentContext),
		},
	}
	messages = append(messages, r.renderTranscript(agentContext.Transcript)...)

	return model.Request{
		System:   agentContext.RuntimePolicy,
		Messages: messages,
		Tools:    append([]model.ToolDefinition(nil), agentContext.Tools...),
		Controls: []model.ControlDefinition{
			{
				Kind:        model.ControlSettle,
				Description: "Finish the current turn without an environment action.",
			},
		},
	}, nil
}

func (r Renderer) renderTranscript(transcript []model.Message) []model.Message {
	if len(transcript) == 0 {
		return nil
	}

	messages := make([]model.Message, 0, len(transcript))
	for _, message := range transcript {
		rendered := model.Message{
			Role:        message.Role,
			Content:     strings.TrimSpace(message.Content),
			ToolCalls:   copyToolCalls(message.ToolCalls),
			ToolResults: r.normalizeToolResults(message.ToolResults),
		}

		switch {
		case len(rendered.ToolCalls) > 0:
			rendered.Content = renderToolCalls(rendered.ToolCalls)
		case len(rendered.ToolResults) > 0:
			rendered.Content = renderToolResults(rendered.ToolResults)
		}
		messages = append(messages, rendered)
	}
	return messages
}

func renderToolCalls(calls []model.ToolCall) string {
	items := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		items = append(items, map[string]any{
			"tool_call_id": call.ID,
			"name":         call.Name,
			"arguments":    orderedMap(call.Arguments),
		})
	}
	return mustMarshalJSONString(items)
}

func renderToolResults(results []model.ToolResult) string {
	items := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item := map[string]any{
			"tool_call_id": result.ToolCallID,
			"name":         result.Name,
			"status":       result.Status,
			"code":         result.Code,
		}
		if result.Message != "" {
			item["message"] = result.Message
		}
		if len(result.Output) > 0 {
			item["output"] = orderedMap(result.Output)
		}
		items = append(items, item)
	}
	return mustMarshalJSONString(items)
}

func (r Renderer) normalizeToolResults(results []model.ToolResult) []model.ToolResult {
	if len(results) == 0 {
		return nil
	}

	normalized := make([]model.ToolResult, len(results))
	for i, result := range results {
		normalized[i] = result
		normalized[i].Message = sanitizeToolResultMessage(result.Message)
		normalized[i].Output = r.projectToolResultOutput(result.Output)
	}
	return normalized
}

func sanitizeToolResultMessage(message string) string {
	message = strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
	if message == "" {
		return ""
	}
	if index := strings.Index(message, "\n"); index >= 0 {
		message = strings.TrimSpace(message[:index])
	}
	if index := strings.Index(message, "{"); index >= 0 {
		message = strings.TrimSpace(message[:index])
	}
	if len([]rune(message)) <= 120 {
		return message
	}
	runes := []rune(message)
	return strings.TrimSpace(string(runes[:120]))
}

func (r Renderer) projectToolResultOutput(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}

	bounds := r.toolResultOutputBounds()
	projected, ok := projectOutputValue(output, 1, bounds).(map[string]any)
	if !ok || len(projected) == 0 {
		return nil
	}

	data, err := json.Marshal(orderedMap(projected))
	if err == nil && bounds.maxBytes > 0 && len(data) > bounds.maxBytes {
		return map[string]any{
			"_truncated": "tool result output exceeded byte limit",
		}
	}
	return projected
}

type toolResultOutputBounds struct {
	maxBytes      int
	maxDepth      int
	maxFields     int
	maxArrayItems int
}

func (r Renderer) toolResultOutputBounds() toolResultOutputBounds {
	return toolResultOutputBounds{
		maxBytes:      positiveOrDefault(r.config.MaxToolResultOutputBytes, 8192),
		maxDepth:      positiveOrDefault(r.config.MaxToolResultOutputDepth, 4),
		maxFields:     positiveOrDefault(r.config.MaxToolResultOutputFields, 64),
		maxArrayItems: positiveOrDefault(r.config.MaxToolResultOutputArrayItems, 32),
	}
}

func positiveOrDefault(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func projectOutputValue(value any, depth int, bounds toolResultOutputBounds) any {
	switch typed := value.(type) {
	case map[string]any:
		if bounds.maxDepth > 0 && depth > bounds.maxDepth {
			return "_truncated: max depth exceeded"
		}
		return projectOutputMap(typed, depth, bounds)
	case []any:
		if bounds.maxDepth > 0 && depth > bounds.maxDepth {
			return "_truncated: max depth exceeded"
		}
		return projectOutputArray(typed, depth, bounds)
	case string, bool, nil:
		return typed
	case int:
		return typed
	case int32:
		return typed
	case int64:
		return typed
	case float32:
		return typed
	case float64:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func projectOutputMap(values map[string]any, depth int, bounds toolResultOutputBounds) map[string]any {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	limit := len(keys)
	if bounds.maxFields > 0 && limit > bounds.maxFields {
		limit = bounds.maxFields
	}

	out := make(map[string]any, limit+1)
	for _, key := range keys[:limit] {
		out[key] = projectOutputValue(values[key], depth+1, bounds)
	}
	if limit < len(keys) {
		out["_truncated_fields"] = len(keys) - limit
	}
	return out
}

func projectOutputArray(values []any, depth int, bounds toolResultOutputBounds) []any {
	limit := len(values)
	if bounds.maxArrayItems > 0 && limit > bounds.maxArrayItems {
		limit = bounds.maxArrayItems
	}

	out := make([]any, 0, limit+1)
	for _, value := range values[:limit] {
		out = append(out, projectOutputValue(value, depth+1, bounds))
	}
	if limit < len(values) {
		out = append(out, fmt.Sprintf("_truncated_items:%d", len(values)-limit))
	}
	return out
}

func orderedMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = values[key]
	}
	return out
}

func mustMarshalJSONString(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(data)
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

Return tool calls only when an environment action is needed. If no action is needed, settle the current turn.
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
	records = selectTimelineMemories(records, currentTime)
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

func selectTimelineMemories(records []memory.Record, currentTime *memory.GameTimeSnapshot) []memory.Record {
	if len(records) == 0 {
		return records
	}

	selected := make([]memory.Record, 0, len(records))
	for _, record := range records {
		if isFutureMemory(record.GameTime, currentTime) {
			continue
		}
		selected = append(selected, record)
	}

	stabilizeEqualGameTimeSequences(selected)
	return selected
}

func stabilizeEqualGameTimeSequences(records []memory.Record) {
	for start := 0; start < len(records); {
		if !hasComparableGameTimeSequence(records[start]) {
			start++
			continue
		}

		end := start + 1
		for end < len(records) && sameGameInstant(records[start].GameTime, records[end].GameTime) && records[end].SourceEventSequence != 0 {
			end++
		}

		if end-start > 1 {
			sort.SliceStable(records[start:end], func(i, j int) bool {
				return records[start+i].SourceEventSequence < records[start+j].SourceEventSequence
			})
		}
		start = end
	}
}

func hasComparableGameTimeSequence(record memory.Record) bool {
	return record.GameTime != nil && record.SourceEventSequence != 0
}

// trimMemories 按 soft budget 裁剪 Recent Memory。
// 优先保留最新 Memory；如果最新一条本身超限，仍保留它。
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
// 存储字段用于 Runtime 追踪；模型只需要“何时 + 输入事实 + 可见动作”的连续性信号。
func renderMemory(record memory.Record, currentTime *memory.GameTimeSnapshot) string {
	outcomes := record.Outcomes

	summaries := make([]string, 0, len(record.SourceContextFacts)+len(outcomes))
	for _, fact := range record.SourceContextFacts {
		if summary := visibleContextFactSummary(fact); summary != "" {
			summaries = append(summaries, summary)
		}
	}
	for _, outcome := range outcomes {
		summaries = append(summaries, visibleActionSummary(outcome))
	}
	if len(summaries) == 0 {
		summaries = append(summaries, "completed turn")
	}
	return fmt.Sprintf("- %s: %s", gameTimeRelation(record.GameTime, currentTime), strings.Join(summaries, "; "))
}

func visibleContextFactSummary(fact memory.SourceContextFact) string {
	kind := strings.ToLower(strings.TrimSpace(fact.Kind))
	text := strings.TrimSpace(fact.Text)
	label := strings.TrimSpace(fact.Label)
	actor := strings.TrimSpace(fact.ActorEntityID)
	if actor == "" {
		actor = "actor"
	}

	switch kind {
	case "utterance":
		if text != "" {
			return fmt.Sprintf("%s said %q", actor, text)
		}
		return ""
	default:
		if text != "" {
			if kind == "" {
				return fmt.Sprintf("%s context %q", actor, text)
			}
			return fmt.Sprintf("%s %s %q", actor, kind, text)
		}
		if label != "" {
			if kind == "" {
				return fmt.Sprintf("%s context %q", actor, label)
			}
			return fmt.Sprintf("%s %s %q", actor, kind, label)
		}
		return ""
	}
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
	case "present_dialogue":
		if text := stringArgument(outcome.ToolArguments, "text"); text != "" {
			return fmt.Sprintf("presented dialogue %q", text)
		}
		return "presented dialogue"
	case "face_player":
		return "faced player"
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

func sameGameInstant(left, right *memory.GameTimeSnapshot) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Year == right.Year &&
		left.Season == right.Season &&
		left.Day == right.Day &&
		left.Hour == right.Hour &&
		left.Minute == right.Minute &&
		left.Tick == right.Tick
}

func isFutureMemory(memoryTime, currentTime *memory.GameTimeSnapshot) bool {
	if memoryTime == nil || currentTime == nil {
		return false
	}
	return compareGameTime(memoryTime, currentTime) > 0
}

func compareGameTime(left, right *memory.GameTimeSnapshot) int {
	if left.Year != right.Year {
		return compareInt32(left.Year, right.Year)
	}
	if left.Season != right.Season {
		return compareInt32(left.Season, right.Season)
	}
	if left.Day != right.Day {
		return compareInt32(left.Day, right.Day)
	}
	if left.Hour != right.Hour {
		return compareInt32(left.Hour, right.Hour)
	}
	if left.Minute != right.Minute {
		return compareInt32(left.Minute, right.Minute)
	}
	return compareInt64(left.Tick, right.Tick)
}

func compareInt32(left, right int32) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
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
