package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/shared"
)

// TestWatchFlow_MultipleInitialization tests that multiple WatchFlow variables work correctly from first execution
func TestWatchFlow_MultipleInitialization(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := hersh.NewWatcher(config, nil, nil)

	// Channels for each WatchFlow
	chanA := make(chan shared.FlowValue[string], 10)
	chanB := make(chan shared.FlowValue[string], 10)
	chanC := make(chan shared.FlowValue[string], 10)
	chanD := make(chan shared.FlowValue[string], 10)
	chanE := make(chan shared.FlowValue[string], 10)

	executionCount := 0
	allInitPrinted := false
	triggerLog := []string{}

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		executionCount++

		// Watch 5 different flow variables with initial values
		flowA := hersh.WatchFlow[string](
			"init_a",
			func(ctx context.Context) (<-chan shared.FlowValue[string], error) {
				return chanA, nil
			},
			"flowA",
			ctx,
		)

		flowB := hersh.WatchFlow[string](
			"init_b",
			func(ctx context.Context) (<-chan shared.FlowValue[string], error) {
				return chanB, nil
			},
			"flowB",
			ctx,
		)

		flowC := hersh.WatchFlow[string](
			"init_c",
			func(ctx context.Context) (<-chan shared.FlowValue[string], error) {
				return chanC, nil
			},
			"flowC",
			ctx,
		)

		flowD := hersh.WatchFlow[string](
			"init_d",
			func(ctx context.Context) (<-chan shared.FlowValue[string], error) {
				return chanD, nil
			},
			"flowD",
			ctx,
		)

		flowE := hersh.WatchFlow[string](
			"init_e",
			func(ctx context.Context) (<-chan shared.FlowValue[string], error) {
				return chanE, nil
			},
			"flowE",
			ctx,
		)

		// Check which flow triggered this execution
		trigger := ctx.GetTriggeredSignal()
		if trigger != nil {
			if trigger.HasVarTrigger("flowA") && flowA.IsUpdated() {
				triggerLog = append(triggerLog, fmt.Sprintf("flowA triggered: %s", flowA.Value))
				t.Logf("Execution %d: flowA triggered with value: %s", executionCount, flowA.Value)
			}
			if trigger.HasVarTrigger("flowB") && flowB.IsUpdated() {
				triggerLog = append(triggerLog, fmt.Sprintf("flowB triggered: %s", flowB.Value))
				t.Logf("Execution %d: flowB triggered with value: %s", executionCount, flowB.Value)
			}
			if trigger.HasVarTrigger("flowC") && flowC.IsUpdated() {
				triggerLog = append(triggerLog, fmt.Sprintf("flowC triggered: %s", flowC.Value))
				t.Logf("Execution %d: flowC triggered with value: %s", executionCount, flowC.Value)
			}
			if trigger.HasVarTrigger("flowD") && flowD.IsUpdated() {
				triggerLog = append(triggerLog, fmt.Sprintf("flowD triggered: %s", flowD.Value))
				t.Logf("Execution %d: flowD triggered with value: %s", executionCount, flowD.Value)
			}
			if trigger.HasVarTrigger("flowE") && flowE.IsUpdated() {
				triggerLog = append(triggerLog, fmt.Sprintf("flowE triggered: %s", flowE.Value))
				t.Logf("Execution %d: flowE triggered with value: %s", executionCount, flowE.Value)
			}
		}

		// Print "Now All Init!!" at the bottom of the function
		if !allInitPrinted {
			fmt.Println("Now All Init!!")
			allInitPrinted = true
		}

		// Stop after enough triggers collected
		if len(triggerLog) >= 10 {
			return shared.NewStopErr("test complete")
		}

		return nil
	}

	watcher.Manage(managedFunc, "multi_flow_init_test")

	// Start the watcher
	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Send signals at different times
	go func() {
		// a := 0.5s
		time.Sleep(500 * time.Millisecond)
		chanA <- shared.FlowValue[string]{V: "signal_a_1"}
		t.Log("Sent signal to flowA at 0.5s")

		// b := 1s
		time.Sleep(500 * time.Millisecond)
		chanB <- shared.FlowValue[string]{V: "signal_b_1"}
		t.Log("Sent signal to flowB at 1s")

		// c := 1.5s
		time.Sleep(500 * time.Millisecond)
		chanC <- shared.FlowValue[string]{V: "signal_c_1"}
		t.Log("Sent signal to flowC at 1.5s")

		// d := 2s
		time.Sleep(500 * time.Millisecond)
		chanD <- shared.FlowValue[string]{V: "signal_d_1"}
		t.Log("Sent signal to flowD at 2s")

		// e := 2.5s
		time.Sleep(500 * time.Millisecond)
		chanE <- shared.FlowValue[string]{V: "signal_e_1"}
		t.Log("Sent signal to flowE at 2.5s")

		// Send another signal to A at 3s (total 3.5s from start)
		time.Sleep(500 * time.Millisecond)
		chanA <- shared.FlowValue[string]{V: "signal_a_2"}
		t.Log("Sent second signal to flowA at 3s")
	}()

	// Wait for test to complete
	time.Sleep(4 * time.Second)

	// Stop the watcher
	watcher.Stop()

	// Verify results
	t.Logf("Total executions: %d", executionCount)
	t.Logf("Trigger log: %v", triggerLog)

	// 1. Should work from first execution (allInitPrinted should be true)
	if !allInitPrinted {
		t.Error("Expected 'Now All Init!!' to be printed from first execution")
	}

	// 2. Should have multiple executions triggered by flow signals
	if executionCount < 6 {
		t.Errorf("Expected at least 6 executions (initial + 5 signals + 1 more), got %d", executionCount)
	}

	// 3. Should have triggers from flowA at least twice
	flowATriggers := 0
	for _, log := range triggerLog {
		if len(log) > 5 && log[:5] == "flowA" {
			flowATriggers++
		}
	}
	if flowATriggers < 2 {
		t.Errorf("Expected flowA to trigger at least twice, got %d times", flowATriggers)
	}

	// 4. Should have triggers from all flows
	hasA, hasB, hasC, hasD, hasE := false, false, false, false, false
	for _, log := range triggerLog {
		if len(log) > 5 {
			switch log[:5] {
			case "flowA":
				hasA = true
			case "flowB":
				hasB = true
			case "flowC":
				hasC = true
			case "flowD":
				hasD = true
			case "flowE":
				hasE = true
			}
		}
	}

	if !hasA || !hasB || !hasC || !hasD || !hasE {
		t.Errorf("Expected triggers from all flows. A:%v B:%v C:%v D:%v E:%v",
			hasA, hasB, hasC, hasD, hasE)
	}

	t.Log("✅ Test complete - all WatchFlow variables initialized and triggered correctly")
}

// TestWatchFlow_ImmediateExecution verifies that managed function runs immediately on Start
func TestWatchFlow_ImmediateExecution(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	watcher := hersh.NewWatcher(config, nil, nil)

	firstExecutionTime := time.Time{}
	startTime := time.Time{}

	managedFunc := func(msg *shared.Message, ctx shared.ManageContext) error {
		if firstExecutionTime.IsZero() {
			firstExecutionTime = time.Now()
			t.Log("First execution triggered")
		}

		// Single WatchFlow with initial value
		chanA := make(chan shared.FlowValue[int], 1)
		flow := hersh.WatchFlow[int](
			42, // Initial value
			func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
				return chanA, nil
			},
			"immediateFlow",
			ctx,
		)

		// Check if initial value is available immediately
		if !flow.IsUpdated() {
			t.Logf("Initial execution: value=%v, NotUpdated=%v", flow.Value, flow.NotUpdated)
		}

		// Print confirmation
		fmt.Printf("Managed function executing with flow value: %v\n", flow.Value)

		// Stop after first execution
		return shared.NewStopErr("immediate test complete")
	}

	watcher.Manage(managedFunc, "immediate_exec_test")

	startTime = time.Now()
	err := watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait briefly
	time.Sleep(100 * time.Millisecond)
	watcher.Stop()

	// Verify immediate execution
	if firstExecutionTime.IsZero() {
		t.Fatal("Managed function was not executed")
	}

	timeDiff := firstExecutionTime.Sub(startTime)
	if timeDiff > 100*time.Millisecond {
		t.Errorf("First execution took too long: %v (expected < 100ms)", timeDiff)
	}

	t.Logf("✅ Managed function executed immediately after Start (delay: %v)", timeDiff)
}