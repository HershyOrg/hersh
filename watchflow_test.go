package hersh

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/HershyOrg/hersh/shared"
)

func TestWatchFlowBasic(t *testing.T) {
	fmt.Println("Testing WatchFlow with simple channel...")

	// Create Watcher
	config := DefaultWatcherConfig()
	watcher := NewWatcher(config, nil, nil)

	executionCount := 0

	// Create channel function (new WatchFlow signature)
	getChannelFunc := func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
		testChan := make(chan shared.FlowValue[int], 10)

		go func() {
			defer close(testChan)

			// Send initial value
			testChan <- shared.FlowValue[int]{V: 0, E: nil}

			// Send more values
			for i := 1; i <= 5; i++ {
				select {
				case <-ctx.Done():
					return
				case <-time.After(200 * time.Millisecond):
					fmt.Printf("Sending value %d...\n", i)
					testChan <- shared.FlowValue[int]{V: i, E: nil}
				}
			}
		}()

		return testChan, nil
	}

	// Register managed function
	watcher.Manage(func(msg *Message, ctx HershContext) error {
		executionCount++
		fmt.Printf("[Reducer #%d] Called\n", executionCount)

		// WatchFlow
		val := WatchFlow[int](0, getChannelFunc, "test_value", ctx)
		if !val.IsError() {
			fmt.Printf("[Reducer #%d] Received value: %v\n", executionCount, val.Value)
		} else {
			fmt.Printf("[Reducer #%d] WatchFlow returned error or zero value\n", executionCount)
		}
		return nil
	}, "TestFunc")

	// Start Watcher (will block until Ready)
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	fmt.Println("Watcher started (Ready state)")

	// Wait for processing (values are sent by goroutine in getChannelFunc)
	time.Sleep(2 * time.Second)

	// Stop Watcher
	watcher.Stop()

	fmt.Printf("\nTotal executions: %d\n", executionCount)

	if executionCount < 2 {
		t.Errorf("Expected at least 2 executions (InitRun + data), got %d", executionCount)
	}

	fmt.Println("✅ Test completed!")
}
