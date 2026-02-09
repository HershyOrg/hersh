package hersh

import (
	"context"
	"time"

	"github.com/HershyOrg/hershy/hersh/manager"
)

// getWatcherFromContext extracts the Watcher from HershContext.
func getWatcherFromContext(ctx HershContext) *Watcher {
	w := ctx.GetWatcher()
	if w == nil {
		return nil
	}
	return w.(*Watcher)
}

// WatchCall monitors a value by periodically generating computation functions.
// Returns the current value or nil if not yet initialized.
//
// The getComputationFunc is called on each tick and returns:
// - A VarUpdateFunc that computes the next state from the previous state
// - An error if the computation function cannot be generated
//
// The returned VarUpdateFunc receives:
// - prev: the previous value (nil on first call)
//
// The VarUpdateFunc returns:
// - next: the new value
// - changed: whether the value changed
// - error: any error that occurred during computation
func WatchCall(
	getComputationFunc func() (manager.VarUpdateFunc, error),
	varName string,
	tick time.Duration,
	runCtx HershContext,
) any {
	w := getWatcherFromContext(runCtx)
	if w == nil {
		panic("WatchCall called with invalid HershContext")
	}

	watchRegistry := w.manager.GetWatchRegistry()
	_, exists := watchRegistry.Load(varName)

	if !exists {
		// First call - register and start watching
		ctx, cancel := context.WithCancel(w.rootCtx)

		tickHandle := &manager.TickHandle{
			VarName:            varName,
			GetComputationFunc: getComputationFunc,
			Tick:               tick,
			CancelFunc:         cancel,
		}

		if err := w.registerWatch(varName, tickHandle); err != nil {
			cancel() // Clean up context
			panic("WatchCall: " + err.Error())
		}

		// Start watching in background
		go tickWatchLoop(w, tickHandle, ctx)

		// Return nil on first call (not yet initialized)
		return nil
	}

	// Get current value from VarState
	if w.manager != nil {
		val, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			// Not initialized yet
			return nil
		}
		return val
	}

	return nil
}

// tickWatchLoop runs the tick-based Watch monitoring loop.
func tickWatchLoop(w *Watcher, handle *manager.TickHandle, rootCtx context.Context) {
	ticker := time.NewTicker(handle.Tick)
	defer ticker.Stop()

	for {
		select {
		case <-rootCtx.Done():
			return

		case <-ticker.C:
			// Get computation function
			varUpdateFunc, err := handle.GetComputationFunc()
			if err != nil {
				// Log error but continue watching
				if logger := w.manager.GetLogger(); logger != nil {
					logger.LogWatchError(handle.VarName, manager.ErrorPhaseGetComputeFunc, err)
				}
				continue
			}

			// Send VarSig with the computation function
			if w.manager != nil {
				w.manager.GetSignals().SendVarSig(&manager.VarSig{
					ComputedTime:       time.Now(),
					TargetVarName:      handle.VarName,
					VarUpdateFunc:      varUpdateFunc,
					IsStateIndependent: false, // Tick is state-dependent (apply sequentially)
				})
			}
		}
	}
}

// WatchFlow monitors a channel and emits VarSig when values arrive.
// This is for event-driven reactive programming.
//
// Returns the latest value from the channel or nil if none received.
func WatchFlow(
	sourceChan <-chan any,
	varName string,
	runCtx HershContext,
) any {
	w := getWatcherFromContext(runCtx)
	if w == nil {
		panic("WatchFlow called with invalid HershContext")
	}

	watchRegistry := w.manager.GetWatchRegistry()
	_, exists := watchRegistry.Load(varName)

	if !exists {
		// First call - register and start watching
		ctx, cancel := context.WithCancel(w.rootCtx)

		flowHandle := &manager.FlowHandle{
			VarName:    varName,
			SourceChan: sourceChan,
			CancelFunc: cancel,
		}

		if err := w.registerWatch(varName, flowHandle); err != nil {
			cancel() // Clean up context
			panic("WatchFlow: " + err.Error())
		}

		// Start watching channel
		go flowWatchLoop(w, flowHandle, ctx)

		return nil
	}

	// Get current value from VarState
	if w.manager != nil {
		val, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			return nil
		}
		return val
	}

	return nil
}

// flowWatchLoop monitors a channel and sends VarSig on updates.
func flowWatchLoop(w *Watcher, handle *manager.FlowHandle, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			msg := time.Now().String() + ": flow " + handle.VarName + " chan cancelled"
			w.GetLogger().LogEffect(msg)
			return

		case value, ok := <-handle.SourceChan:
			if !ok {
				// Channel closed
				return
			}

			// Wrap value in a VarUpdateFunc (ignores prev, returns value)
			varUpdateFunc := func(prev any) (any, bool, error) {
				return value, true, nil
			}

			// Send VarSig
			if w.manager != nil {
				w.manager.GetSignals().SendVarSig(&manager.VarSig{
					ComputedTime:       time.Now(),
					TargetVarName:      handle.VarName,
					VarUpdateFunc:      varUpdateFunc,
					IsStateIndependent: true, // Flow is state-independent (use last value only)
				})
			}
		}
	}
}

// Watch is a convenience wrapper that creates a channel-based watch.
// Deprecated: Use WatchCall or WatchFlow directly.
func Watch(varName string, initialValue any, runCtx HershContext) any {
	// For backward compatibility, just return WatchCall with a simple function
	return WatchCall(
		func() (manager.VarUpdateFunc, error) {
			return func(prev any) (any, bool, error) {
				if prev == nil {
					return initialValue, true, nil
				}
				return prev, false, nil
			}, nil
		},
		varName,
		1*time.Second,
		runCtx,
	)
}
