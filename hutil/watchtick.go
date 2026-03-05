package hutil

import (
	"context"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/shared"
)

const defaultBufferSize = 10

// WatchTick monitors time-based intervals and returns a HershTick with timestamp and count.
//
// This is a convenience wrapper around WatchFlow that simplifies time-based operations.
// It automatically handles:
//   - Ticker channel creation with initial tick
//   - FlowValue conversion
//   - Tick count tracking (starts from 1)
//   - Deduplication via WatchFlow's state-independent behavior
//
// Returns HershTick with Time and TickCount, or zero value if not yet initialized.
//
// Usage:
//
//	tick := hutil.WatchTick("stats_ticker", 1*time.Minute, ctx)
//	if !tick.IsZero() {
//	    fmt.Printf("Tick #%d at %s\n", tick.TickCount, tick.Time.Format("15:04:05"))
//	    // Handle tick event
//	}
//
// The function sends an initial tick immediately (TickCount=1), then at regular intervals.
// Each subsequent tick increments the TickCount.
func WatchTick(varName string, tickInterval time.Duration, runCtx shared.HershContext) shared.HershTick {
	// Create a FlowValue channel function that wraps the ticker with count tracking
	getChannelFunc := func(flowCtx context.Context) (<-chan shared.FlowValue, error) {
		flowChan := make(chan shared.FlowValue, defaultBufferSize)

		go func() {
			defer close(flowChan)

			tickCount := 0

			// Send initial tick immediately
			tickCount++
			select {
			case flowChan <- shared.FlowValue{
				V: shared.HershTick{Time: time.Now(), TickCount: tickCount, VarName: varName},
				E: nil,
			}:
			case <-flowCtx.Done():
				return
			}

			// Create ticker for subsequent ticks
			ticker := time.NewTicker(tickInterval)
			defer ticker.Stop()

			for {
				select {
				case <-flowCtx.Done():
					return
				case t := <-ticker.C:
					tickCount++
					select {
					case flowChan <- shared.FlowValue{
						V: shared.HershTick{Time: t, TickCount: tickCount, VarName: varName},
						E: nil,
					}:
					case <-flowCtx.Done():
						return
					}
				}
			}
		}()

		return flowChan, nil
	}

	// Use WatchFlow with the ticker channel function
	hv := hersh.WatchFlow(getChannelFunc, varName, runCtx)

	// Return HershTick (zero value if not initialized or error)
	if hv.IsError() || hv.Value == nil {
		return shared.HershTick{}
	}

	tick := hv.Value.(shared.HershTick)
	tick.VarName = varName
	return tick
}
