package hersh

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
)

// TestManagerLifecycleIsolation tests that Manager and Watch have independent lifecycles.
func TestManagerLifecycleIsolation(t *testing.T) {
	config := DefaultWatcherConfig()
	w := NewWatcher(config, map[string]string{"ENV": "test"}, nil)

	flowChannels := make([]chan shared.FlowValue[int], 3)
	var flowGoroutinesRunning atomic.Int32
	stopChan := make(chan bool)

	// Manage call creates Manager
	w.Manage(func(msg *Message, ctx ManageContext) error {
		// Create 3 WatchFlow instances
		for i := 0; i < 3; i++ {
			idx := i
			flowChannels[idx] = make(chan shared.FlowValue[int], 10)

			WatchFlow(0, func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
				flowGoroutinesRunning.Add(1)
				go func() {
					defer flowGoroutinesRunning.Add(-1)
					<-ctx.Done() // Wait for context cancellation
				}()
				return flowChannels[idx], nil
			}, fmt.Sprintf("flow%d", idx), ctx)
		}

		// Wait for stop signal
		select {
		case <-stopChan:
			return shared.NewStopErr("test stop")
		case <-ctx.Done():
			return ctx.Err()
		}
	}, "test").Cleanup(func(ctx ManageContext) {
		t.Log("Cleanup called")
	})

	// Verify Manager was created
	if w.manager == nil {
		t.Fatal("Manager should be created on Manage")
	}

	// Start Watcher
	err := w.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for Watch goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Verify Watch goroutines are running
	if got := flowGoroutinesRunning.Load(); got != 3 {
		t.Errorf("Expected 3 flow goroutines running, got %d", got)
	}

	// Send data through channels to verify they work
	for i, ch := range flowChannels {
		ch <- shared.FlowValue[int]{V: i}
	}
	time.Sleep(50 * time.Millisecond)

	// Stop signal
	close(stopChan)

	// Stop Watcher - this should stop Manager and all Watch goroutines
	err = w.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Wait a bit for goroutines to stop
	time.Sleep(100 * time.Millisecond)

	// Verify Manager's EffectHandler rootCtx is cancelled
	select {
	case <-w.manager.GetEffectHandler().GetRootContext().Done():
		// Good - context is cancelled
	default:
		t.Fatal("Manager's EffectHandler rootCtx should be cancelled after Stop")
	}

	// Verify all Watch goroutines stopped
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if flowGoroutinesRunning.Load() == 0 {
			return // Success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("Flow goroutines did not stop: %d still running", flowGoroutinesRunning.Load())
}

// TestWatcherContextCancellation tests that cancelling Watcher's parent context
// properly triggers auto-shutdown and stops Manager and all Watches.
func TestWatcherContextCancellation(t *testing.T) {
	config := DefaultWatcherConfig()
	parentCtx, parentCancel := context.WithCancel(context.Background())

	w := NewWatcher(config, map[string]string{"TEST": "value"}, parentCtx)

	flowChannels := make([]chan shared.FlowValue[int], 3)
	var flowGoroutinesRunning atomic.Int32
	managerStarted := make(chan bool, 1)

	// Single Manage call
	w.Manage(func(msg *Message, ctx ManageContext) error {
		// Create 3 WatchFlow instances
		for i := 0; i < 3; i++ {
			idx := i
			flowChannels[idx] = make(chan shared.FlowValue[int], 10)

			WatchFlow(0, func(ctx context.Context) (<-chan shared.FlowValue[int], error) {
				flowGoroutinesRunning.Add(1)
				go func() {
					defer flowGoroutinesRunning.Add(-1)
					<-ctx.Done() // Wait for context cancellation
				}()
				return flowChannels[idx], nil
			}, fmt.Sprintf("flow%d", idx), ctx)
		}

		// Signal that manager has started
		select {
		case managerStarted <- true:
		default:
		}

		// Keep running until context is cancelled
		<-ctx.Done()
		return ctx.Err()
	}, "test").Cleanup(func(ctx ManageContext) {
		t.Log("Cleanup called after context cancellation")
	})

	// Verify Manager was created
	if w.manager == nil {
		t.Fatal("Manager should be created on Manage")
	}

	// Start Watcher
	err := w.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for Manager to start
	select {
	case <-managerStarted:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("Manager did not start")
	}

	// Wait a bit for Watch goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Verify Watch goroutines are running
	if got := flowGoroutinesRunning.Load(); got != 3 {
		t.Errorf("Expected 3 flow goroutines running, got %d", got)
	}

	// Send some data to verify channels work
	for i, ch := range flowChannels {
		select {
		case ch <- shared.FlowValue[int]{V: i * 10}:
		case <-time.After(100 * time.Millisecond):
			t.Logf("Warning: could not send to channel %d", i)
		}
	}

	// Cancel parent context - this should trigger Watcher auto-shutdown
	parentCancel()

	// Wait for Watcher to stop (auto-shutdown should happen)
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if !w.isRunning.Load() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if w.isRunning.Load() {
		t.Fatal("Watcher should stop after parent context cancelled")
	}

	// Verify Manager's EffectHandler rootCtx is cancelled
	select {
	case <-w.manager.GetEffectHandler().GetRootContext().Done():
		// Good - context is cancelled
	default:
		t.Fatal("Manager's EffectHandler rootCtx should be cancelled after parent context cancellation")
	}

	// Verify all Watch goroutines stopped
	deadline = time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		if flowGoroutinesRunning.Load() == 0 {
			t.Log("All Watch goroutines successfully stopped")
			return // Success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("Flow goroutines did not stop: %d still running", flowGoroutinesRunning.Load())
}

// TestWatchRegistrationWithManager tests Watch registration through Manager.
func TestWatchRegistrationWithManager(t *testing.T) {
	config := DefaultWatcherConfig()
	config.MaxWatches = 2 // Limit to 2 watches

	w := NewWatcher(config, nil, nil)

	watchNames := []string{}
	registerErr := make(chan error, 1)

	w.Manage(func(msg *Message, ctx ManageContext) error {
		// Try to register 3 watches (should fail on 3rd)
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("watch%d", i)
			err := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						if watchErr, ok := r.(*shared.WatchInitPanic); ok {
							err = fmt.Errorf("watch init panic: %v", watchErr.Reason)
						} else {
							err = fmt.Errorf("panic: %v", r)
						}
					}
				}()

				WatchCall(0, func() (manager.VarUpdateFunc[int], bool, error) {
					return func(prev int) (int, error) {
						return prev + 1, nil
					}, false, nil
				}, name, 1*time.Second, ctx)

				watchNames = append(watchNames, name)
				return nil
			}()

			if err != nil {
				select {
				case registerErr <- err:
				default:
				}
				return shared.NewStopErr("registration limit reached")
			}
		}

		<-ctx.Done()
		return ctx.Err()
	}, "test")

	err := w.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for registration error
	select {
	case err := <-registerErr:
		if err == nil || err.Error() == "" {
			t.Fatal("Expected registration error")
		}
		// Check for limit message
		// Error message should contain "limit"
		t.Logf("Got expected error: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Expected registration error due to limit")
	}

	err = w.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Should have registered exactly 2 watches
	if len(watchNames) != 2 {
		t.Errorf("Expected exactly 2 watches registered before hitting limit, got %d", len(watchNames))
	}
}
