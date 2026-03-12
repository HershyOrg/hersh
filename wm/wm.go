package wm

import "time"

// WatchMachine은 Watch와 관련한 기능을 한데 모은 구조체임.
// Manager는 WatchMachine을 Subscribe함으로써 새 변수 값을 감지-추적 가능함.
// 기존의 DELETED_Watch함수, DELETED_VarSig 및 watchRegistry나 각종 Handle을 대체함.
type WatchMachine struct {
	//varName을 통해 WatchMachine을 식별함
	VarName string

	//WatchXXX설정 보관
	WatchType             WatchType
	GetRawCallHandleOrNil GetRawCallHandleFunc
	GetRawFlowHandleOrNil GetRawFlowHandleFunc

	//reduce-effect엔진
	reducer       WmReducerInterface
	effectHandler WmEffectHandlerInferface
	//loop의 상태-조작을 reducer-effect가 담당-지시함.
	loop WatchLoopInterface

	//ctxConfig로 WatchMachine의 생명주기-타임아웃 결정
	ctxConfig WatchContextConfig

	//Subscribers는 WatchMachine을 구독한 Manager들임.
	//즉, WatchXXX를 varName으로 호출한 것들.
	Subscribers []Subscriber
	//marker를 이용해 WatchMachine은 Subscribers가
	//Subscriber별로 어디까지 reducer의 VarHistory를 읽었는지 추적함.
	marker MarkerInterface

	//PublisherOrNil는 Multi-Manager가 구현되었을 시,
	//해당 WatchMachine을 Export한 Manager를 나타냄
	//현재는 신경쓰지 않으며, 당장은 nil로 둚.
	//Publisher는 있더라도 오직 하나임. SingleWriter-MultiReader모델.
	PublisherOrNil Publisher

	//GcChecker를 통해 WatchMachine의 구독자들을 체크하고,
	//구독자들이 다 멈췄다면, 쓸모없어신 자신도 멈춤.
	GcChecker GcCheckerInterface
	//PublishChecker를 통해 자신을 Export한 Manager가 죽었는지 체크함.
	//Loop가 받는 정보만으론 Export한 Manager 상태를 체크할 수 없기 때문.
	//현재 Export기능이 없으므로,지금은 PublishChecker를 고려하지 않음.
	PublishChecker PublishCheckerInterface
}

type WatchContextConfig struct {
	RunContextTimeout  time.Time
	RootContextTimeout time.Time
}
type WatchType string

const (
	WatchFlowType WatchType = "WatchFlowType"
	WatchCallType WatchType = "WatchCallType"
)
