package wmachine

import "time"

type WmEffectHandlerInferface interface{}
type WlEffect interface {
	WatchMachineEffect()
}

// StartWatchLoop는 WatchLoop를 "초기화 후 시작"하는 Effect임
// 기존의 RootContext를 제거 후, 새 Context와 새 Handle과 함께 Loop자체를 초기화함.
type StartLoop struct{}

// SleepWatchLoop는 WatchLoop가 "잠들게"하는 Effect임
type SleepLoop struct {
	duration time.Time
}
