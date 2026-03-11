package wmachine

import (
	"time"

	"github.com/HershyOrg/hersh/shared"
)

// WatchCallFunc는 사용자가 Tick으로 값을 받으며, 필요시 Hook을 적용하기를 선언해주는 함수임.
// WatchCallFunc는 managedCtx의 Manager에 varName에 따른 WatchMachine이 없을 시,
// GetCallHandle을 통해 WatchMachine을 생성함.
// 만약 이미 WatchMachine이 있다면, Manager(=Subscriber)의 VarState에서 값을 받아옴.
// WatchMachine은 백그라운드에서 WatchLoop를 돌리며 VarSig를 Manager(=Subscriber)로 보냄
type WatchCallFunc[T any] func(varName string, getCallHandle GetCallHandleFunc[T],
	managerCtx shared.ManageContext) (V shared.WatchValue[T])

// WatchFlowFunc는 사용자가 Chan으로 값을 받으며, 필요시 Hook을 적용하기를 선언해주는 함수임.
// WatchCallFunc는 managedCtx의 Manager에 varName에 따른 WatchMachine이 없을 시,
// GetFlowHandle을 통해 WatchMachine을 생성함.
// 만약 이미 WatchMachine이 있다면, Manager의 VarState에서 값을 받아옴.
// WatchMachine은 백그라운드에서 WatchLoop를 돌리며 VarSig를 Manager(=Subscriber)로 보냄
type WatchFlowFunc[T any] func(varName string, getFlowHandle GetFlowHandleFunc[T],
	managerCtx shared.ManageContext) (V shared.WatchValue[T])

type GetCallHandleFunc[T any] func(callCtx CallContext) (CallHandle[T], error)

type CallHandle[T any] struct {
	Init     T
	Tick     time.Time
	HookFunc HookFunc[T]
	//* varName은 WatchCall이 varName받은 후 삽입.
	varName string
}

type HookFunc[T any] func(prev T, hookCtx HookContext) (next T, skip bool, err error)

type GetFlowHandleFunc[T any] func(flowCtx FlowContext) (FlowHandle[T], error)
type FlowHandle[T any] struct {
	Init     T
	FlowChan chan T
	HookFunc HookFunc[T]
	//* varName은 WatchFlow가 varName받은 후 삽입
	varName string
}
