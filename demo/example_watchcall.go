package main

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hershy/hersh"
	"github.com/HershyOrg/hershy/hersh/manager"
)

// Advanced example demonstrating WatchCall reactive mechanism
func main1() {
	fmt.Println("=== Hersh WatchCall Demo ===")
	fmt.Println()

	config := hersh.DefaultWatcherConfig()
	watcher := hersh.NewWatcher(config, nil, nil)

	// Simulated external data source
	externalCounter := 0

	managedFunc := func(msg *hersh.Message, ctx hersh.HershContext) error {
		fmt.Printf("\n[Managed Function Execution]\n")

		// WatchCall monitors external value and triggers re-execution on change
		watchedValue := hersh.WatchCall(
			func() (manager.VarUpdateFunc, error) {
				return func(prev any) (any, bool, error) {
					// Simulate polling external data source
					currentValue := externalCounter
					externalCounter++

					fmt.Printf("  Polling: prev=%v, current=%v\n", prev, currentValue)

					// Detect change
					if prev == nil {
						return currentValue, true, nil // First call - always changed
					}

					prevInt := prev.(int)
					changed := prevInt != currentValue
					return currentValue, changed, nil
				}, nil
			},
			"externalCounter",
			300*time.Millisecond, // Poll every 300ms
			ctx,
		)

		// React to the watched value
		if watchedValue == nil {
			fmt.Println("  Status: Waiting for first value...")
		} else {
			counter := watchedValue.(int)
			fmt.Printf("  Watched Value: %d\n", counter)

			// Business logic based on watched value
			if counter%3 == 0 && counter > 0 {
				fmt.Printf("  🎯 Milestone reached at %d!\n", counter)
			}

			// Stop condition
			if counter >= 5 {
				fmt.Println("  ✅ Target reached, stopping...")
				return hersh.NewStopErr("reached target count")
			}
		}

		return nil
	}

	watcher.Manage(managedFunc, "watchCallExample").Cleanup(func(ctx hersh.HershContext) {
		fmt.Println("\n[Cleanup] Shutting down watcher")
	})

	fmt.Println("Starting watcher...")
	err := watcher.Start()
	if err != nil {
		panic(err)
	}

	// Wait for reactive executions triggered by WatchCall
	time.Sleep(3 * time.Second)

	fmt.Printf("\nFinal state: %s\n", watcher.GetState())

	// Print summary
	fmt.Println("\n=== Execution Summary ===")
	watcher.GetLogger().PrintSummary()

	err = watcher.Stop()
	if err != nil {
		fmt.Printf("Error stopping: %v\n", err)
	}

	fmt.Println("\n=== Demo Complete ===")
}
