package tool

import (
	"fmt"
	protocolv1alpha2 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha2"
	"gameagent/runtime/internal/idgen"
	"gameagent/runtime/internal/model"
)

// BuildActionRequest 把模型 ToolCall 转成 Adapter 能执行的 ActionRequest。
//
// 这里不直接访问 gRPC stream，也不接触具体游戏 API；它只维护
// Runtime 内部 tool 语义到 protocol action 语义的转换边界。
func BuildActionRequest(worldID string, entityID string, call model.ToolCall) (*protocolv1alpha2.ActionRequest, error) {
	if worldID == "" {
		return nil, fmt.Errorf("world is empty")
	}

	if entityID == "" {
		return nil, fmt.Errorf("entity is empty")
	}

	if call.Arguments == nil {
		return nil, fmt.Errorf("tool arguments are missing")
	}

	return &protocolv1alpha2.ActionRequest{
		ActionId:   newActionID(),
		EntityId:   entityID,
		Capability: call.Name,
		Arguments:  call.Arguments,
		WorldId:    worldID,
	}, nil
}

// newActionID 创建游戏动作 ID。
//
// action_id 标识一次 Adapter 需要执行的业务动作，不等同于 RuntimeMessage.message_id。
func newActionID() string {
	return idgen.New("act")
}
