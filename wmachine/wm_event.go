package wmachine

import (
	"time"

	"github.com/HershyOrg/hersh/shared"
)

// WmEvent는 WatchMachine상에서 발생하는 이벤트임.
type WmEvent interface {
	WatchMachineEvent()
}

// WatchedNewVar는 WatchLoop가 관측한 새 Var임.
type WatchedNewVar struct {
	WatchedTime time.Time
	Value       shared.RawWatchValue
}

func (w *WatchedNewVar) WatchMachineEvent() {}
