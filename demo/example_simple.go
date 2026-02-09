package main

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hershy/hersh"
)

// Simple counter example demonstrating hersh reactive framework
func main() {
	fmt.Printf("=== Hersh Reactive Framework Demo ===\n")

	config := hersh.DefaultWatcherConfig()
	config.DefaultTimeout = 5 * time.Second

	watcher := hersh.NewWatcher(config, nil, nil)

	// Managed function with reactive state
	counter := 0
	managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
		counter++
		fmt.Printf("[Execution %d]\n", counter)

		// Use Memo for expensive computation (cached)
		expensiveResult := hersh.Memo(func() any {
			fmt.Println("  Computing expensive value...")
			time.Sleep(100 * time.Millisecond)
			return "cached_result"
		}, "expensiveComputation", ctx)
		fmt.Printf("  Memo result: %v\n", expensiveResult)

		// Use context value for shared state
		totalRuns := ctx.GetValue("totalRuns")
		if totalRuns == nil {
			totalRuns = 0
		}
		newTotal := totalRuns.(int) + 1
		ctx.SetValue("totalRuns", newTotal)
		fmt.Printf("  Total runs across executions: %d\n", newTotal)

		// Handle user messages
		if msg != nil {
			fmt.Printf("  Received message: '%s'\n", msg.Content)
			if msg.Content == "stop" {
				fmt.Println("  Stopping watcher gracefully...")
				return hersh.NewStopErr("user requested stop")
			}
		}

		fmt.Println()
		return nil
	}

	// Register managed function with cleanup
	watcher.Manage(managedFunc, "counterExample").Cleanup(func(ctx hersh.HershContext) {
		fmt.Println("\n[Cleanup] Watcher is shutting down")
		fmt.Printf("[Cleanup] Final state - Counter: %d\n", counter)
	})

	// Start watcher
	fmt.Println("Starting watcher...")
	err := watcher.Start()
	if err != nil {
		panic(err)
	}

	// Wait for initialization
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("Watcher state: %s\n\n", watcher.GetState())

	// Send messages to trigger executions
	fmt.Println("Sending message 1...")
	watcher.SendMessage("Hello from main!")
	time.Sleep(300 * time.Millisecond)

	fmt.Println("Sending message 2...")
	watcher.SendMessage("Another message")
	time.Sleep(300 * time.Millisecond)

	fmt.Println("Sending stop message...")
	watcher.SendMessage("stop")
	time.Sleep(300 * time.Millisecond)

	// Print logger summary
	fmt.Println("\n=== Execution Summary ===")
	watcher.GetLogger().PrintSummary()

	// Stop watcher
	err = watcher.Stop()
	if err != nil {
		fmt.Printf("Error stopping watcher: %v\n", err)
	}

	fmt.Println("\n=== Demo Complete ===")
}
