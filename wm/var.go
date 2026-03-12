package wm

import (
	"time"

	"github.com/HershyOrg/hersh/shared"
)

// VarHistory는 WatchLoop가 보낸 WatchedNewVar 이벤트를 저장한 내역임.
// WatchMachine의 Subscriber들이 주로 []VarHistory를 가져감
type VarHistory struct {
	ReceivedTime time.Time
	//Hook의 Error는 Value에 반영되므로 굳이 Err필드 추가하지 않음.
	//Handle의 Error는 GetHandle시 제어됨.
	Value shared.RawWatchValue
}
