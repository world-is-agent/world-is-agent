package tool

import (
	"fmt"
	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
	"gameagent/runtime/internal/model"
	"time"
)

func BuildActionRequest(entityID string, call model.ToolCall) (*protocolv1alpha1.ActionRequest, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entity is empty")
	}

	if call.Arguments == nil {
		return nil, fmt.Errorf("tool arguments are missing")
	}

	return &protocolv1alpha1.ActionRequest{
		ActionId:   newActionID(),
		EntityId:   entityID,
		Capability: call.Name,
		Arguments:  call.Arguments,
	}, nil
}

func newActionID() string {
	return fmt.Sprintf("act_%d", time.Now().UnixNano())
}
