package hersh

import (
	"context"
	"time"

	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
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
// Returns the current HershValue or an empty HershValue if not yet initialized.
//
// The getComputationFunc is called on each tick and returns:
// - A VarUpdateFunc that computes the next state from the previous state
// - An error if the computation function cannot be generated
//
// The returned VarUpdateFunc receives:
// - prev: the previous HershValue (empty HershValue on first call)
//
// The VarUpdateFunc returns:
// - next: the new HershValue
// - changed: whether the value changed
// - error: any error that occurred during computation
func WatchCall(
	getComputationFunc func() (manager.VarUpdateFunc, error),
	varName string,
	tick time.Duration,
	runCtx HershContext,
) shared.HershValue {
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

		// Return empty HershValue on first call (not yet initialized)
		return shared.HershValue{VarName: varName}
	}

	// Get current HershValue from VarState
	if w.manager != nil {
		hv, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			// Not initialized yet
			return shared.HershValue{VarName: varName}
		}
		// Set VarName before returning
		hv.VarName = varName
		return hv
	}

	return shared.HershValue{VarName: varName}
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
// Returns the latest HershValue from the channel or an empty HershValue if none received.
func WatchFlow(
	getChannelFunc func(ctx context.Context) (<-chan shared.FlowValue, error),
	varName string,
	runCtx HershContext,
) shared.HershValue {
	w := getWatcherFromContext(runCtx)
	if w == nil {
		panic("WatchFlow called with invalid HershContext")
	}

	watchRegistry := w.manager.GetWatchRegistry()
	_, exists := watchRegistry.Load(varName)

	if !exists {
		// First call - register and start watching
		// Create channel lifecycle context
		flowCtx, cancel := context.WithCancel(w.rootCtx)

		// Try to create channel
		sourceChan, err := getChannelFunc(flowCtx)
		if err != nil {
			cancel()
			// Log error (recovery responsibility is separated)
			w.GetLogger().LogWatchError(varName, manager.ErrorPhaseGetComputeFunc, err)

			// Register error HershValue with VarName
			errorHV := shared.HershValue{Value: nil, Error: err, VarName: varName}
			w.manager.GetState().VarState.Set(varName, errorHV)
			return errorHV
		}

		flowHandle := &manager.FlowHandle{
			VarName:        varName,
			GetChannelFunc: getChannelFunc,
			CancelFunc:     cancel,
		}

		if err := w.registerWatch(varName, flowHandle); err != nil {
			cancel() // Clean up context
			panic("WatchFlow: " + err.Error())
		}

		// Start watching channel
		go flowWatchLoop(w, flowHandle, flowCtx, sourceChan)

		return shared.HershValue{VarName: varName}
	}

	// Get current HershValue from VarState
	if w.manager != nil {
		hv, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			return shared.HershValue{VarName: varName}
		}
		// Set VarName before returning
		hv.VarName = varName
		return hv
	}

	return shared.HershValue{VarName: varName}
}

// flowWatchLoop monitors a channel and sends VarSig on updates.
// Now propagates errors to user via HershValue instead of skipping them.
func flowWatchLoop(w *Watcher, handle *manager.FlowHandle, ctx context.Context, sourceChan <-chan shared.FlowValue) {
	for {
		select {
		case <-ctx.Done():
			msg := "FlowWatch stopped: " + handle.VarName
			w.GetLogger().LogEffect(msg)
			return

		case flowValue, ok := <-sourceChan:
			if !ok {
				// Channel closed
				msg := "Channel closed: " + handle.VarName
				w.GetLogger().LogEffect(msg)
				return
			}

			// Wrap value or error in a VarUpdateFunc that returns HershValue
			varUpdateFunc := func(prev shared.HershValue) (shared.HershValue, bool, error) {
				if flowValue.E != nil {
					// Log error but still propagate to user
					w.GetLogger().LogWatchError(handle.VarName, manager.ErrorPhaseExecuteComputeFunc, flowValue.E)
					// Return HershValue with error
					return shared.HershValue{Value: nil, Error: flowValue.E}, true, nil
				}
				// Return HershValue with value
				return shared.HershValue{Value: flowValue.V, Error: nil}, true, nil
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
