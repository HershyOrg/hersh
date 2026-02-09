package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/HershyOrg/hershy/hersh"
)

func TestWatchFlowBasic(t *testing.T) {
	fmt.Println("Testing WatchFlow with simple channel...")

	// Create test channel
	testChan := make(chan any, 10)

	// Create Watcher
	config := hersh.DefaultWatcherConfig()
	watcher := hersh.NewWatcher(config, nil, nil)

	executionCount := 0
	// Send initial value BEFORE starting Watcher
	// This ensures WatchFlow will receive data during InitRun
	fmt.Println("Sending initial value to channel...")
	testChan <- 0

	// Register managed function
	watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
		executionCount++
		fmt.Printf("[Reducer #%d] Called\n", executionCount)

		// WatchFlow
		val := hersh.WatchFlow(testChan, "test_value", ctx)
		if val != nil {
			fmt.Printf("[Reducer #%d] Received value: %v\n", executionCount, val)
		} else {
			fmt.Printf("[Reducer #%d] WatchFlow returned nil (first time)\n", executionCount)
		}

		return nil
	}, "TestFunc")

	// Start Watcher (will block until Ready)
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	fmt.Println("Watcher started (Ready state)")
	time.Sleep(200 * time.Millisecond)

	// Send test values
	fmt.Println("\nSending values to channel...")
	for i := 1; i <= 5; i++ {
		fmt.Printf("Sending value %d...\n", i)
		testChan <- i
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	// Stop Watcher
	watcher.Stop()

	fmt.Printf("\nTotal executions: %d\n", executionCount)

	if executionCount < 2 {
		t.Errorf("Expected at least 2 executions (InitRun + data), got %d", executionCount)
	}

	fmt.Println("✅ Test completed!")
}
