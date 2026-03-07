package main

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/manager"
)

// Advanced example demonstrating WatchCall reactive mechanism
func main1() {
	fmt.Println("=== Hersh WatchCall Demo ===")
	fmt.Println()

	config := hersh.DefaultWatcherConfig()
	watcher := hersh.NewWatcher(config, nil, nil)

	// Simulated external data source
	externalCounter := 0

	managedFunc := func(msg *hersh.Message, ctx hersh.ManageContext) error {
		fmt.Printf("\n[Managed Function Execution]\n")

		// WatchCall monitors external value and triggers re-execution on change (generic version)
		hv := hersh.WatchCall[int](
			0, // Initial counter value
			func() (manager.VarUpdateFunc[int], bool, error) {
				// Simulate polling external data source
				currentValue := externalCounter
				externalCounter++

				// VarUpdateFunc that updates to the new value
				updateFunc := func(prev int) (int, error) {
					fmt.Printf("  Polling: prev=%v, current=%v\n", prev, currentValue)
					return currentValue, nil
				}

				// Don't skip signal for this demo to show reactive updates
				skipSignal := false

				return updateFunc, skipSignal, nil
			},
			"externalCounter",
			300*time.Millisecond, // Poll every 300ms
			ctx,
		)

		// React to the watched value
		if hv.Value == 0 && hv.Error == nil {
			fmt.Println("  Status: Waiting for first value...")
		} else if hv.Error != nil {
			fmt.Printf("  ⚠️ Error: %v\n", hv.Error)
		} else {
			counter := hv.Value // Type-safe, no assertion needed
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

	watcher.Manage(managedFunc, "watchCallExample").Cleanup(func(ctx hersh.ManageContext) {
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
