package wmachine

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hersh/shared"
)

// GetComputationFunc returns the RawVarUpdateFunc, a skipSignal flag
// (false by default; set to true if you want to skip), and an error.
type GetComputationFunc func() (varUpdateFunc RawVarUpdateFunc, skipSignal bool, err error)
type GetComputationFuncResult struct {
	VarUpdateFunc RawVarUpdateFunc
	SkipSignal    bool
	Err           error
}

// VarUpdateFunc is a generic function that updates a variable's state.
// It receives the previous value of type T and returns the next value and an error.
type VarUpdateFunc[T any] func(prev T) (next T, err error)

// RawVarUpdateFunc is the internal non-generic version used by VarSig.
// It receives the previous RawHershValue and returns the next RawHershValue and an error.
type RawVarUpdateFunc func(prev shared.RawWatchValue) (next shared.RawWatchValue)

// VarSig represents a change in a watched variable's state.
type VarSig struct {
	ReceivedTime                  time.Time
	TargetVarName                 string
	GetComputeFuncErrOrGetChanErr error            // GetComputeFunc 실행 중 발생한 에러
	SourceType                    WatchType        // "Call" or "Flow" 구분
	VarUpdateFunc                 RawVarUpdateFunc // Function to compute the next state (internal raw version)
	IsStateIndependent            bool             // If true, only last signal matters; if false, apply sequentially
}
type WatchType string

const (
	WatchFlowType WatchType = "WatchFlowType"
	WatchCallType WatchType = "WatchCallType"
)

func (s *VarSig) Priority() shared.SignalPriority {
	return shared.PriorityVar
}

func (s *VarSig) CreatedAt() time.Time {
	return s.ReceivedTime
}

func (s *VarSig) String() string {
	typeStr := "dependent"
	if s.IsStateIndependent {
		typeStr = "independent"
	}
	return fmt.Sprintf("VarSig{var=%s, type=%s, time=%s}",
		s.TargetVarName, typeStr, s.ReceivedTime.Format(time.RFC3339))
}
