package hersh

import (
	"time"

	"github.com/HershyOrg/hersh/shared"
	"github.com/HershyOrg/hersh/wmachine"
)

// WatchTick provides a convenient way to create a tick-based watcher.
// It automatically uses the current time as the initial value.
func WatchTick(
	varName string,
	tick time.Duration,
	runCtx shared.ManageContext,
) shared.WatchValue[shared.TickValue] {
	// Create initial tick with current time
	init := shared.TickValue{
		Time:       time.Now(),
		TickCount:  0,
		VarName:    varName,
		NotUpdated: true, // Mark as initial value
	}

	// Use WatchCall with tick generation function
	return WatchCall(
		init,
		func() (wmachine.VarUpdateFunc[shared.TickValue], bool, error) {
			return func(prev shared.TickValue) (shared.TickValue, error) {
				return shared.TickValue{
					Time:       time.Now(),
					TickCount:  prev.TickCount + 1,
					VarName:    varName,
					NotUpdated: false, // Mark as updated
				}, nil
			}, false, nil
		},
		varName,
		tick,
		runCtx,
	)
}
