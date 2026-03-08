// Package manager implements the Manager component of the hersh framework.
// Manager handles state management through Reducer and Effect System.
package manager

import (
	"fmt"
	"time"

	"github.com/HershyOrg/hersh/shared"
)

// VarUpdateFunc is a generic function that updates a variable's state.
// It receives the previous value of type T and returns the next value and an error.
type VarUpdateFunc[T any] func(prev T) (next T, err error)

// RawVarUpdateFunc is the internal non-generic version used by VarSig.
// It receives the previous RawHershValue and returns the next RawHershValue and an error.
type RawVarUpdateFunc func(prev shared.RawWatchValue) (next shared.RawWatchValue, err error)

// VarSig represents a change in a watched variable's state.
type VarSig struct {
	ReceivedTime       time.Time
	TargetVarName      string
	VarUpdateFunc      RawVarUpdateFunc // Function to compute the next state (internal raw version)
	IsStateIndependent bool             // If true, only last signal matters; if false, apply sequentially
}

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

// UserSig represents a change in the user message state.
type UserSig struct {
	ReceivedTime time.Time
	UserMessage  *shared.Message
}

func (s *UserSig) Priority() shared.SignalPriority {
	return shared.PriorityUser
}

func (s *UserSig) CreatedAt() time.Time {
	return s.ReceivedTime
}

func (s *UserSig) String() string {
	msgContent := ""
	if s.UserMessage != nil {
		msgContent = s.UserMessage.Content
	}
	return fmt.Sprintf("UserSig{msg=%s, time=%s}",
		msgContent, s.ReceivedTime.Format(time.RFC3339))
}

// ManagerInnerSig represents a change in the Managers's state.
type ManagerInnerSig struct {
	ReceivedTime time.Time
	TargetState  shared.ManagerInnerState
	Reason       string // Why this transition is happening
	NeedInit     bool   // Whether initialization is needed (for restart scenarios)
}

func (s *ManagerInnerSig) Priority() shared.SignalPriority {
	return shared.PriorityManagerInner
}

func (s *ManagerInnerSig) CreatedAt() time.Time {
	return s.ReceivedTime
}

func (s *ManagerInnerSig) String() string {
	return fmt.Sprintf("ManagerSig{target=%s, reason=%s, time=%s}",
		s.TargetState, s.Reason, s.ReceivedTime.Format(time.RFC3339))
}

// SignalChannels holds all signal channels for the Manager.
type SignalChannels struct {
	VarSigChan          chan *VarSig
	UserSigChan         chan *UserSig
	ManagerInnerSigChan chan *ManagerInnerSig
	NewSigAppended      chan struct{} // Notifies when any signal is added
}

// NewSignalChannels creates a new SignalChannels with buffered channels.
func NewSignalChannels(bufferSize int) *SignalChannels {
	return &SignalChannels{
		VarSigChan:          make(chan *VarSig, bufferSize),
		UserSigChan:         make(chan *UserSig, bufferSize),
		ManagerInnerSigChan: make(chan *ManagerInnerSig, bufferSize),
		NewSigAppended:      make(chan struct{}, bufferSize*3), // Can hold all possible signals
	}
}

// SendVarSig sends a VarSig and notifies of new signal.
func (sc *SignalChannels) SendVarSig(sig *VarSig) {
	sc.VarSigChan <- sig
	select {
	case sc.NewSigAppended <- struct{}{}:
	default:
		// Channel full, signal will still be processed
	}
}

// SendUserSig sends a UserSig and notifies of new signal.
func (sc *SignalChannels) SendUserSig(sig *UserSig) {
	sc.UserSigChan <- sig
	select {
	case sc.NewSigAppended <- struct{}{}:
	default:
	}
}

// SendManagerInnerSig sends a WatcherSig and notifies of new signal.
func (sc *SignalChannels) SendManagerInnerSig(sig *ManagerInnerSig) {
	sc.ManagerInnerSigChan <- sig
	select {
	case sc.NewSigAppended <- struct{}{}:
	default:
	}
}

// Close closes all signal channels.
func (sc *SignalChannels) Close() {
	close(sc.VarSigChan)
	close(sc.UserSigChan)
	close(sc.ManagerInnerSigChan)
	close(sc.NewSigAppended)
}
