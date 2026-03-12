package wm

// MarkerInterface는 Marker가 해야 할 일을 정의한 인터페이스임.
// Marker는 WatchMachine이 Subscriber에게 VarHistory를 전달해 줄 때 사용하는 구조체임.
// Marker는 병렬 안전해야 함.
type MarkerInterface interface {
	//ReadHistory는 Subscriber가 "가장 최신으로 읽은 VarHistory 이후의 []VarHistory"를 제공함
	ReadHistory(Subscriber Subscriber) ([]VarHistory, error)
	//mark는 Subscriber가 "가장 최신으로 읽은 VarHistory"의 인덱스를 마킹함
	mark(subscriber Subscriber) error
}
