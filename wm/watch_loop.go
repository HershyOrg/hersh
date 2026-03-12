package wm

// WatchLoopInterface는 WatchLoop가 해야 할 일에 대한 디자인이다.
type WatchLoopInterface interface {
	//Start시 WatchLoop는 Handle에 따라 값 받은 후,
	//Hook을 적용해 V, skip, error를 얻는다.
	// skip==true시 History에 저장하지 않음과 동시에 NewSigAppend로 구독자에게 보고하지도 않는다.
	// skip하지 않는다면 NewSigAppend로 구독자에게 보고하고,
	// V,error는 한데 합쳐 RawWatchValue로 VarHitsory에 저장한다.
	Start(ctx RunContext) error
}
