package wmachine

// WmReducerInterface는 WatchMachine의 Reducer가 해야 할 일에 대한 디자인임.
type WmReducerInterface interface {
	//WmReducerInterface의 Reduce함수는
	//"WatchLoop"의 State, Effect를 다룸
	Reduce(currentState WlState, event WmEvent) (nextState WlState, effects []WlEffect)
}

// WlState는 WatchLoop의 상태임
type WlState interface {
	WlState()
}

// PrescribeFunc는 문제 발생 시 "처방전"을 지시함.
// 리커버리 판단에 쓰임.
// 미약한 경우 WatchLoop를 잠시 자게 하고,
// 에러 연속열이 길 시, WatchLoop를 다시 Start하며
// 최종 실패 시 WatchLoop를 Crash로 전이시킴.
type PrescribeFunc func(varHistory VarHistory) (state WlState, effects []WlEffect)
