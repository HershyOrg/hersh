package hersh

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HershyOrg/hershy/hersh/manager"
)

// Watcher is the core reactive framework engine.
// It manages reactive state through Watch, executes managed functions,
// and provides fault tolerance through supervision.
type Watcher struct {
	config  WatcherConfig
	manager *manager.Manager

	// State
	isRunning atomic.Bool // watcher자체가 실행중인지의 값. (Run/ Stop)

	// Lifecycle
	rootCtx    context.Context
	rootCancel context.CancelFunc

	// API Server
	apiServer *WatcherAPIServer
}

// NewWatcher creates a new Watcher with the given configuration and environment variables.
// The Manager is initialized during Watcher construction.
// envVars are injected into HershContext and are immutable after initialization.
// If envVars is nil, an empty map is created.
//
// parentCtx (optional): Parent context for lifecycle management.
//   - If provided: Watcher automatically stops when context is cancelled
//   - If nil: Uses context.Background() (backward compatibility)
//   - Auto-stop has 5-minute timeout, then forces shutdown
func NewWatcher(config WatcherConfig, envVars map[string]string, parentCtx context.Context) *Watcher {
	if config.DefaultTimeout == 0 {
		config = DefaultWatcherConfig()
	}

	// Use parent context if provided, otherwise use Background
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	// Create independent context for Manager
	// This ensures Manager continues running even when parentCtx is cancelled
	// Only Stop() will cancel this context
	rootCtx, cancel := context.WithCancel(context.Background())

	// Initialize Manager with config (no managed function yet)
	mgr := manager.NewManager(config)

	// Inject environment variables into HershContext
	// The HershContext is already created by EffectHandler during Manager initialization
	hershCtx := mgr.GetEffectHandler().GetHershContext()
	if hctxImpl, ok := hershCtx.(interface{ SetEnvVars(map[string]string) }); ok {
		hctxImpl.SetEnvVars(envVars)
	}

	w := &Watcher{
		config:     config,
		manager:    mgr,
		rootCtx:    rootCtx,
		rootCancel: cancel,
	}

	// Set watcher reference in Manager's handler
	mgr.GetEffectHandler().SetWatcher(w)

	// 🎯 Auto-shutdown goroutine: monitors parent context
	// When parent context is cancelled, automatically stops the watcher
	go func() {
		<-parentCtx.Done()

		if !w.isRunning.Load() {
			return // Already stopped
		}

		fmt.Println("[Watcher] Parent context cancelled, stopping...")

		// Call Stop() with 5-minute timeout
		stopDone := make(chan error, 1)
		go func() {
			stopDone <- w.Stop()
		}()

		select {
		case err := <-stopDone:
			// Stop completed
			if err != nil {
				fmt.Printf("[Watcher] Stop error: %v\n", err)
			}
		case <-time.After(5 * time.Minute):
			// Stop() blocked for 5 minutes - force shutdown
			fmt.Println("[Watcher] Stop timeout (5 min), forcing shutdown...")
			w.forceShutdown()
		}
	}()

	return w
}

// Manage registers a function to be managed by the Watcher.
// Returns a CleanupBuilder for optional cleanup registration.
func (w *Watcher) Manage(fn manager.ManagedFunc, name string) *CleanupBuilder {
	if w.isRunning.Load() {
		panic("cannot call Manage after Watcher is already running")
	}

	if w.manager.HasManagedFunc() {
		panic("managed function already registered")
	}

	// Wrap the managed function
	wrappedFn := func(msg *Message, ctx HershContext) error {
		return fn(msg, ctx)
	}

	// Register managed function with the Manager
	// Cleaner will be set via CleanupBuilder
	w.manager.SetManagedFunc(wrappedFn, nil)

	return &CleanupBuilder{
		watcher:     w,
		managedFunc: wrappedFn,
	}
}

// Start begins the Watcher's execution.
// It starts all Manager components, enters InitRun state, and waits for Ready state.
func (w *Watcher) Start() error {
	if !w.isRunning.CompareAndSwap(false, true) {
		return fmt.Errorf("watcher already running")
	}

	if !w.manager.HasManagedFunc() {
		w.isRunning.Store(false) // Reset on error
		return fmt.Errorf("no managed function registered")
	}

	// Start Manager components
	w.manager.Start(w.rootCtx)

	// Start API server (non-blocking)
	apiServer, err := w.StartAPIServer()
	if err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}
	w.apiServer = apiServer

	// Send InitRun signal to start initialization
	w.manager.GetSignals().SendWatcherSig(&manager.WatcherSig{
		SignalTime:  time.Now(),
		TargetState: StateInitRun,
		Reason:      "watcher start",
	})

	// Wait for initialization to complete using channel (deterministic, no timeouts)
	// The Manager will transition to Ready state when all variables are initialized
	readyAfterInitChan := w.manager.GetState().WaitReadyAfterInit()
	<-readyAfterInitChan

	// Check final state - it should be Ready, but could be Crashed/Killed if init failed
	finalState := w.manager.GetState().GetManagerInnerState()
	if finalState != StateReady {
		// Initialization failed
		return fmt.Errorf("initialization failed: watcher entered %s state", finalState)
	}

	return nil
}

// Stop gracefully stops the Watcher.
func (w *Watcher) Stop() error {
	// Check if Manager is already in a terminal state
	// This handles cases where StopError/KillError automatically stopped the Manager
	currentState := w.manager.GetState().GetManagerInnerState()
	if currentState == StateStopped || currentState == StateKilled || currentState == StateCrashed {
		return fmt.Errorf("watcher already stopped (state: %s)", currentState)
	}

	if !w.isRunning.Load() {
		return fmt.Errorf("watcher not running")
	}

	// Send Stop signal
	w.manager.GetSignals().SendWatcherSig(&manager.WatcherSig{
		SignalTime:  time.Now(),
		TargetState: StateStopped,
		Reason:      "user requested stop",
	})

	// Wait for actual cleanup completion using channels (deterministic, no timeouts)
	// 1. Wait for cleanup to actually complete
	cleanupDone := w.manager.GetEffectHandler().GetCleanupDone()
	<-cleanupDone

	// 2. Wait for Manager to reach Stopped state
	stoppedChan := w.manager.GetState().WaitStoppedAfterCleanup()
	<-stoppedChan

	// 3. Shutdown API server
	if w.apiServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := w.apiServer.Shutdown(shutdownCtx); err != nil {
			// Check if timeout occurred
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("[Watcher] API server shutdown timeout (5s), forcing close...")
				// Force close
				if closeErr := w.apiServer.Close(); closeErr != nil {
					fmt.Printf("[Watcher] API server force close error: %v\n", closeErr)
				} else {
					fmt.Println("[Watcher] API server force closed successfully")
				}
			} else {
				fmt.Printf("[Watcher] API server shutdown error: %v\n", err)
			}
		} else {
			fmt.Println("[Watcher] API server stopped gracefully")
		}
	}

	// 4. Finalize Watcher shutdown
	w.stopAllWatches()
	w.rootCancel()
	w.isRunning.Store(false)

	return nil
}

// forceShutdown forcefully terminates the Watcher (last resort).
// Called when Stop() times out after 5 minutes.
// This bypasses normal cleanup and immediately shuts down all components.
func (w *Watcher) forceShutdown() {
	fmt.Println("[Watcher] Force shutdown initiated...")

	// Cancel all contexts immediately
	w.rootCancel()

	// Force stop all watch goroutines
	w.stopAllWatches()

	// Force close API server without graceful shutdown
	if w.apiServer != nil {
		if err := w.apiServer.Close(); err != nil {
			fmt.Printf("[Watcher] Force close API server error: %v\n", err)
		}
	}

	// Mark as stopped
	w.isRunning.Store(false)

	fmt.Println("[Watcher] Force shutdown complete")
}

// SendMessage sends a user message to the managed function.
func (w *Watcher) SendMessage(content string) error {
	if !w.isRunning.Load() {
		return fmt.Errorf("watcher not running")
	}

	msg := &Message{
		Content:    content,
		IsConsumed: false,
		ReceivedAt: time.Now(),
	}

	w.manager.GetSignals().SendUserSig(&manager.UserSig{
		ReceivedTime: time.Now(),
		UserMessage:  msg,
	})

	return nil
}

// GetState returns the current WatcherState.
func (w *Watcher) GetState() ManagerInnerState {
	return w.manager.GetState().GetManagerInnerState()
}

// GetLogger returns the Watcher's logger for inspection.
func (w *Watcher) GetLogger() *manager.Logger {
	return w.manager.GetLogger()
}

// registerWatch registers a Watch variable with limit enforcement.
// This is called by Watch/WatchCall/WatchFlow functions.
// Returns error if watch limit is reached.
func (w *Watcher) registerWatch(varName string, handle manager.WatchHandle) error {
	watchRegistry := w.manager.GetWatchRegistry()

	// Check if updating existing watch (allowed)
	if _, exists := watchRegistry.Load(varName); !exists {
		// New watch - check size limit
		count := 0
		watchRegistry.Range(func(_, _ any) bool {
			count++
			return true
		})

		if count >= w.config.MaxWatches {
			return fmt.Errorf("watch registry limit reached: %d/%d (cannot register '%s')",
				count, w.config.MaxWatches, varName)
		}
	}

	watchRegistry.Store(varName, handle)

	// Register with EffectHandler for initialization tracking
	w.manager.GetEffectHandler().RegisterVar(varName)
	return nil
}

// stopAllWatches stops all Watch goroutines with 1 minute timeout.
func (w *Watcher) stopAllWatches() {
	var wg sync.WaitGroup
	watchRegistry := w.manager.GetWatchRegistry()

	watchRegistry.Range(func(key, value any) bool {
		handle := value.(manager.WatchHandle)
		wg.Add(1)

		go func(h manager.WatchHandle) {
			defer wg.Done()
			if cancelFunc := h.GetCancelFunc(); cancelFunc != nil {
				cancelFunc()
			}
		}(handle)

		return true // continue iteration
	})

	// Wait for all watches to stop with 1 minute timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("[Watcher] All watches stopped successfully")
	case <-time.After(1 * time.Minute):
		fmt.Println("[Watcher] Warning: Some watches did not stop within 1min timeout")
	}
}

// CleanupBuilder provides a fluent interface for registering cleanup.
type CleanupBuilder struct {
	watcher     *Watcher
	managedFunc manager.ManagedFunc
}

// Cleanup registers a cleanup function to be called on Stop/Kill/Crash.
func (cb *CleanupBuilder) Cleanup(cleanupFn func(ctx HershContext)) *Watcher {
	cleaner := &cleanupAdapter{
		cleanupFn: cleanupFn,
	}

	// Update cleaner in the Manager
	cb.watcher.manager.SetManagedFunc(cb.managedFunc, cleaner)

	return cb.watcher
}

// cleanupAdapter adapts the user's cleanup function to the Cleaner interface.
type cleanupAdapter struct {
	cleanupFn func(ctx HershContext)
}

func (ca *cleanupAdapter) ClearRun(ctx HershContext) error {
	// Simply call the cleanup function with HershContext
	ca.cleanupFn(ctx)
	return nil
}

// hershContextImpl implements HershContext (simple fallback version).
type hershContextImpl struct {
	context.Context
	watcherID string
	message   *Message
}

func (hc *hershContextImpl) WatcherID() string {
	return hc.watcherID
}

func (hc *hershContextImpl) Message() *Message {
	return hc.message
}

func (hc *hershContextImpl) GetValue(key string) any {
	// Fallback implementation returns nil
	return nil
}

func (hc *hershContextImpl) SetValue(key string, value any) {
	// Fallback implementation does nothing
}

func (hc *hershContextImpl) UpdateValue(key string, updateFn func(current any) any) any {
	// Fallback implementation - call updateFn with nil and return result
	return updateFn(nil)
}
