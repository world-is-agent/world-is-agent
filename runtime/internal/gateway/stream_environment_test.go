package gateway

import (
	"context"
	"strings"
	"testing"

	protocolv1alpha1 "gameagent/protocol/gen/go/gameagent/protocol/v1alpha1"
)

func TestStreamEnvironmentSubmitActionRejectsNilRequest(t *testing.T) {
	env := newStreamEnvironment(nil)

	_, err := env.SubmitAction(context.Background(), nil)

	if err == nil {
		t.Fatal("expected error for nil action request")
	}
	if !strings.Contains(err.Error(), "action request is nil") {
		t.Fatalf("expected nil action request error, got %q", err.Error())
	}
}

func TestStreamEnvironmentSubmitActionRejectsEmptyActionID(t *testing.T) {
	env := newStreamEnvironment(nil)

	_, err := env.SubmitAction(context.Background(), &protocolv1alpha1.ActionRequest{})

	if err == nil {
		t.Fatal("expected error for empty action id")
	}
	if !strings.Contains(err.Error(), "action id is empty") {
		t.Fatalf("expected empty action id error, got %q", err.Error())
	}
}
