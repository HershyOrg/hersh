package manager

import (
	"testing"

	"github.com/HershyOrg/hershy/hersh/shared"
)

func TestVarState_BasicOperations(t *testing.T) {
	vs := NewVarState()

	// Test Set and Get
	vs.Set("var1", 42)
	val, ok := vs.Get("var1")
	if !ok {
		t.Fatal("expected var1 to exist")
	}
	if val != 42 {
		t.Errorf("expected 42, got %v", val)
	}

	// Test non-existent variable
	_, ok = vs.Get("var2")
	if ok {
		t.Error("expected var2 to not exist")
	}
}

func TestVarState_BatchSet(t *testing.T) {
	vs := NewVarState()

	updates := map[string]any{
		"var1": 1,
		"var2": "two",
		"var3": 3.0,
	}

	vs.BatchSet(updates)

	for name, expected := range updates {
		val, ok := vs.Get(name)
		if !ok {
			t.Errorf("expected %s to exist", name)
		}
		if val != expected {
			t.Errorf("expected %v, got %v", expected, val)
		}
	}
}

func TestVarState_AllInitialized(t *testing.T) {
	vs := NewVarState()

	expectedVars := []string{"var1", "var2", "var3"}

	// Not initialized yet
	if vs.AllInitialized(expectedVars) {
		t.Error("expected AllInitialized to return false")
	}

	// Initialize all
	vs.Set("var1", 1)
	vs.Set("var2", 2)
	vs.Set("var3", 3)

	if !vs.AllInitialized(expectedVars) {
		t.Error("expected AllInitialized to return true")
	}

	// Set one to nil
	vs.Set("var2", nil)
	if vs.AllInitialized(expectedVars) {
		t.Error("expected AllInitialized to return false when var is nil")
	}
}

func TestVarState_Clear(t *testing.T) {
	vs := NewVarState()

	vs.Set("var1", 1)
	vs.Set("var2", 2)

	vs.Clear()

	snapshot := vs.GetAll()
	if len(snapshot) != 0 {
		t.Errorf("expected empty state after Clear, got %d variables", len(snapshot))
	}
}

func TestUserState_Operations(t *testing.T) {
	us := NewUserState()

	// Initially no message
	if msg := us.GetMessage(); msg != nil {
		t.Error("expected nil message initially")
	}

	// Set message
	msg := &shared.Message{
		Content:    "test message",
		IsConsumed: false,
	}
	us.SetMessage(msg)

	// Get message
	retrieved := us.GetMessage()
	if retrieved == nil {
		t.Fatal("expected message to exist")
	}
	if retrieved.Content != "test message" {
		t.Errorf("expected 'test message', got %s", retrieved.Content)
	}

	// Consume message
	consumed := us.ConsumeMessage()
	if consumed == nil {
		t.Fatal("expected consumed message")
	}
	if !consumed.IsConsumed {
		t.Error("expected message to be marked as consumed")
	}
	if consumed.Content != "test message" {
		t.Errorf("expected 'test message', got %s", consumed.Content)
	}

	// Message should be nil after consumption
	if msg := us.GetMessage(); msg != nil {
		t.Error("expected nil message after consumption")
	}

	// Consuming again should return nil
	if consumed := us.ConsumeMessage(); consumed != nil {
		t.Error("expected nil when consuming non-existent message")
	}
}

func TestManagerState_ManagerInnerState(t *testing.T) {
	ms := NewManagerState(shared.StateInitRun)

	if state := ms.GetManagerInnerState(); state != shared.StateInitRun {
		t.Errorf("expected StateInitRun, got %s", state)
	}

	ms.SetManagerInnerState(shared.StateReady)

	if state := ms.GetManagerInnerState(); state != shared.StateReady {
		t.Errorf("expected StateReady, got %s", state)
	}
}

func TestManagerState_Snapshot(t *testing.T) {
	ms := NewManagerState(shared.StateReady)

	// Set up state
	ms.VarState.Set("var1", 100)
	ms.UserState.SetMessage(&shared.Message{Content: "hello"})

	snapshot := ms.Snapshot()

	// Verify snapshot
	if snapshot.ManagerInnerState != shared.StateReady {
		t.Errorf("expected StateReady, got %s", snapshot.ManagerInnerState)
	}

	if val, ok := snapshot.VarState["var1"]; !ok || val != 100 {
		t.Errorf("expected var1=100, got %v", val)
	}

	if snapshot.UserMessage == nil || snapshot.UserMessage.Content != "hello" {
		t.Error("expected message 'hello' in snapshot")
	}
}
