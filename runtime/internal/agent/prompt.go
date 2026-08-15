package agent

import (
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// BuildModelRequest 构建当前 Agent Turn 的模型输入。
func BuildModelRequest(
	event *protocolv1alpha1.GameEvent,
	observation *protocolv1alpha1.Observation,
	tools []model.ToolDefinition,
) model.Request {
	return model.Request{
		System: BuildSystemPrompt(),
		Messages: []model.Message{
			BuildTurnMessage(event, observation),
		},
		Tools: tools,
	}
}

func BuildSystemPrompt() string {
	return `You are controlling an NPC in a game.

Your job:
- Stay in character.
- Use exactly one available tool.
- For MVP, prefer speak unless there is no useful response.
- Speak in Simplified Chinese.
- The speak text must be no more than 60 Chinese characters.
- Keep dialogue short and natural.`
}

func BuildTurnMessage(
	event *protocolv1alpha1.GameEvent,
	observation *protocolv1alpha1.Observation,
) model.Message {
	return model.Message{
		Role: model.RoleUser,
		Content: fmt.Sprintf(`Current game event JSON:
%s

Current observation JSON:
%s

Return a tool call only.
`, protoToJSON(event), protoToJSON(observation)),
	}
}

// protoToJSON 把 protobuf 对象转成 JSON 字符串。
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
