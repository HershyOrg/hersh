package wmachine

import "time"

// GetRawCallHandleFunc는 RawCallHandleFunc를 WatchMachine이 저장하기 위해 변형한 형태임.
type GetRawCallHandleFunc func(callCtx CallContext) (RawCallHandle, error)

// GetRawFlowHandleFunc는 RawFlowHandleFunc를 WatchMachine이 저장하기 위해 변형한 형태임
type GetRawFlowHandleFunc func(flowCtx FlowContext) (RawFlowHandle, error)

// RawCallHandle은 WatchMachine이  CallHandle을 저장하기 위해 변형한 형태임.
type RawCallHandle struct {
	RawInit     any
	Tick        time.Time
	RawHookFunc RawHookFunc
	varName     string
}

// RawCallHandle은 WatchMachine이 FlowHandle을 저장하기 위해 변형한 형태임.
type RawFlowHandle struct {
	RawInit     any
	RawFlowChan chan any
	RawHookFunc RawHookFunc
	varName     string
}
type RawHookFunc func(prev any, hookCtx HookContext) (next any, skip bool, err error)
