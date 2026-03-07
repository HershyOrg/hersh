package hersh

import (
	"context"
	"fmt"
	"time"

	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
)

// getWatcherFromContext extracts the Watcher from ManageContext.
func getWatcherFromContext(ctx shared.ManageContext) *Watcher {
	w := ctx.GetWatcher()
	if w == nil {
		return nil
	}
	return w.(*Watcher)
}

// WatchCall monitors a value by periodically generating computation functions (generic version).
// Returns the current HershValue[T] or a zero-value HershValue[T] if not yet initialized.
//
// The getComputationFunc is called on each tick and returns:
// - A VarUpdateFunc[T] that computes the next state from the previous state
// - skipSignal: whether to skip sending a signal (default false = send signal)
// - An error if the computation function cannot be generated
//
// The returned VarUpdateFunc[T] receives:
// - prev: the previous value of type T (zero value on first call)
//
// The VarUpdateFunc[T] returns:
// - next: the new value of type T
// - error: any error that occurred during computation
func WatchCall[T any](
	getComputationFunc func() (manager.VarUpdateFunc[T], bool, error),
	varName string,
	tick time.Duration,
	runCtx shared.ManageContext,
) shared.HershValue[T] {
	w := getWatcherFromContext(runCtx)
	if w == nil {
		panic("WatchCall called with invalid ManageContext")
	}

	watchRegistry := w.manager.GetWatchRegistry()
	_, exists := watchRegistry.Load(varName)

	if !exists {
		// First call - register and start watching
		ctx, cancel := context.WithCancel(w.rootCtx)

		// Wrap user's generic function into raw function for internal use
		wrappedGetFunc := func() (manager.RawVarUpdateFunc, bool, error) {
			typedFunc, skip, err := getComputationFunc()
			if err != nil {
				return nil, skip, err
			}

			// Convert VarUpdateFunc[T] to rawVarUpdateFunc
			rawFunc := func(prev shared.RawHershValue) (shared.RawHershValue, error) {
				// Extract previous value with type assertion
				var prevT T
				if prev.Value != nil {
					var ok bool
					prevT, ok = prev.Value.(T)
					if !ok {
						var zero T
						panic(fmt.Sprintf(
							"WatchCall '%s': type mismatch on prev value - expected %T, got %T",
							varName, zero, prev.Value,
						))
					}
				}

				// Execute user's typed function
				nextT, err := typedFunc(prevT)

				// Return as RawHershValue
				return shared.RawHershValue{
					Value:   any(nextT),
					Error:   err,
					VarName: varName,
				}, nil
			}

			return rawFunc, skip, nil
		}

		tickHandle := &manager.TickHandle{
			VarName:            varName,
			GetComputationFunc: wrappedGetFunc,
			Tick:               tick,
			CancelFunc:         cancel,
		}

		if err := w.registerWatch(varName, tickHandle); err != nil {
			cancel() // Clean up context
			panic("WatchCall: " + err.Error())
		}

		// Start watching in background
		go tickWatchLoop(w, tickHandle, ctx)

		// Return zero-value HershValue[T] on first call (not yet initialized)
		var zero T
		return shared.HershValue[T]{Value: zero, VarName: varName}
	}

	// Get current RawHershValue from VarState
	if w.manager != nil {
		rawHV, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			// Not initialized yet - return zero value
			var zero T
			return shared.HershValue[T]{Value: zero, VarName: varName}
		}

		// Convert RawHershValue to HershValue[T] with type assertion
		var typedVal T
		if rawHV.Value != nil {
			var ok bool
			typedVal, ok = rawHV.Value.(T)
			if !ok {
				var zero T
				panic(fmt.Sprintf(
					"WatchCall '%s': type mismatch on read - expected %T, got %T",
					varName, zero, rawHV.Value,
				))
			}
		}

		return shared.HershValue[T]{
			Value:   typedVal,
			Error:   rawHV.Error,
			VarName: varName,
		}
	}

	var zero T
	return shared.HershValue[T]{Value: zero, VarName: varName}
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
			// Get computation function and signal flag
			varUpdateFunc, skipSignal, err := handle.GetComputationFunc()
			if err != nil {
				// Log error but continue watching
				if logger := w.manager.GetLogger(); logger != nil {
					logger.LogWatchError(handle.VarName, manager.ErrorPhaseGetComputeFunc, err)
				}
				continue
			}

			// Send VarSig unless user wants to skip
			if !skipSignal && w.manager != nil {
				w.manager.GetSignals().SendVarSig(&manager.VarSig{
					ReceivedTime:       time.Now(),
					TargetVarName:      handle.VarName,
					VarUpdateFunc:      varUpdateFunc,
					IsStateIndependent: false, // Tick is state-dependent (apply sequentially)
				})
			}
		}
	}
}

// WatchFlow monitors a channel and emits VarSig when values arrive (generic version).
// This is for event-driven reactive programming.
//
// Returns the latest HershValue[T] from the channel or a zero-value HershValue[T] if none received.
func WatchFlow[T any](
	getChannelFunc func(ctx context.Context) (<-chan shared.FlowValue[T], error),
	varName string,
	runCtx shared.ManageContext,
) shared.HershValue[T] {
	w := getWatcherFromContext(runCtx)
	if w == nil {
		panic("WatchFlow called with invalid ManageContext")
	}

	watchRegistry := w.manager.GetWatchRegistry()
	_, exists := watchRegistry.Load(varName)

	if !exists {
		// First call - register and start watching
		// Create channel lifecycle context
		flowCtx, cancel := context.WithCancel(w.rootCtx)

		// Wrap user's generic channel into raw channel for internal use
		rawGetChanFunc := func(ctx context.Context) (<-chan shared.RawFlowValue, error) {
			typedChan, err := getChannelFunc(ctx)
			if err != nil {
				return nil, err
			}

			// Convert FlowValue[T] channel to RawFlowValue channel
			rawChan := make(chan shared.RawFlowValue, cap(typedChan))
			go func() {
				defer close(rawChan)
				for fv := range typedChan {
					// Convert FlowValue[T] to RawFlowValue
					rawChan <- shared.RawFlowValue{
						V:          any(fv.V),
						E:          fv.E,
						SkipSignal: fv.SkipSignal,
					}
				}
			}()

			return rawChan, nil
		}

		// Try to create channel
		sourceChan, err := rawGetChanFunc(flowCtx)
		if err != nil {
			cancel()
			// Log error (recovery responsibility is separated)
			w.GetLogger().LogWatchError(varName, manager.ErrorPhaseGetComputeFunc, err)

			// Register error RawHershValue with VarName
			errorHV := shared.RawHershValue{Value: nil, Error: err, VarName: varName}
			w.manager.GetState().VarState.Set(varName, errorHV)

			// Return error as HershValue[T]
			var zero T
			return shared.HershValue[T]{Value: zero, Error: err, VarName: varName}
		}

		flowHandle := &manager.FlowHandle{
			VarName:        varName,
			GetChannelFunc: rawGetChanFunc,
			CancelFunc:     cancel,
		}

		if err := w.registerWatch(varName, flowHandle); err != nil {
			cancel() // Clean up context
			panic("WatchFlow: " + err.Error())
		}

		// Start watching channel
		go flowWatchLoop(w, flowHandle, flowCtx, sourceChan)

		// Return zero-value HershValue[T]
		var zero T
		return shared.HershValue[T]{Value: zero, VarName: varName}
	}

	// Get current RawHershValue from VarState
	if w.manager != nil {
		rawHV, ok := w.manager.GetState().VarState.Get(varName)
		if !ok {
			var zero T
			return shared.HershValue[T]{Value: zero, VarName: varName}
		}

		// Convert RawHershValue to HershValue[T] with type assertion
		var typedVal T
		if rawHV.Value != nil {
			var ok bool
			typedVal, ok = rawHV.Value.(T)
			if !ok {
				var zero T
				panic(fmt.Sprintf(
					"WatchFlow '%s': type mismatch on read - expected %T, got %T",
					varName, zero, rawHV.Value,
				))
			}
		}

		return shared.HershValue[T]{
			Value:   typedVal,
			Error:   rawHV.Error,
			VarName: varName,
		}
	}

	var zero T
	return shared.HershValue[T]{Value: zero, VarName: varName}
}

// flowWatchLoop monitors a channel and sends VarSig on updates.
// Now propagates errors to user via RawHershValue instead of skipping them.
func flowWatchLoop(w *Watcher, handle *manager.FlowHandle, ctx context.Context, sourceChan <-chan shared.RawFlowValue) {
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

			// Send signal unless SkipSignal is true
			if !flowValue.SkipSignal {
				// Wrap value or error in a RawVarUpdateFunc that returns RawHershValue
				varUpdateFunc := func(prev shared.RawHershValue) (shared.RawHershValue, error) {
					if flowValue.E != nil {
						// Log error but still propagate to user
						w.GetLogger().LogWatchError(handle.VarName, manager.ErrorPhaseExecuteComputeFunc, flowValue.E)
						// Return RawHershValue with error
						return shared.RawHershValue{Value: nil, Error: flowValue.E}, nil
					}
					// Return RawHershValue with value
					return shared.RawHershValue{Value: flowValue.V, Error: nil}, nil
				}

				// Send VarSig
				if w.manager != nil {
					w.manager.GetSignals().SendVarSig(&manager.VarSig{
						ReceivedTime:       time.Now(),
						TargetVarName:      handle.VarName,
						VarUpdateFunc:      varUpdateFunc,
						IsStateIndependent: true, // Flow is state-independent (use last value only)
					})
				}
			}
		}
	}
}
