package hersh

import (
	"time"

	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
)

// WatchTick provides a convenient way to create a tick-based watcher.
// It automatically uses the current time as the initial value.
func WatchTick(
	varName string,
	tick time.Duration,
	runCtx shared.ManageContext,
) shared.HershValue[shared.HershTick] {
	// Create initial tick with current time
	init := shared.HershTick{
		Time:       time.Now(),
		TickCount:  0,
		VarName:    varName,
		NotUpdated: true, // Mark as initial value
	}

	// Use WatchCall with tick generation function
	return WatchCall(
		init,
		func() (manager.VarUpdateFunc[shared.HershTick], bool, error) {
			return func(prev shared.HershTick) (shared.HershTick, error) {
				return shared.HershTick{
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