// Package core contains shared types used across the hersh framework.
package shared

import (
	"context"
	"time"
)

// ManagerInnerState represents the internal state of the Manager.
// States form a finite state machine with specific transition rules.
// This is the core state that drives the reactive execution engine.
type ManagerInnerState uint8

const (
	// StateReady indicates the Manager is ready to execute on next signal
	StateReady ManagerInnerState = iota
	// StateRunning indicates the Manager is currently executing
	StateRunning
	// StateInitRun indicates the Manager is initializing (first run)
	StateInitRun
	// StateStopped indicates the Manager stopped normally and can be restarted
	StateStopped
	// StateKilled indicates the Manager was killed and is permanently stopped
	StateKilled
	// StateCrashed indicates the Manager crashed irrecoverably
	StateCrashed
	// StateWaitRecover indicates the Manager is waiting for recovery decision
	StateWaitRecover
)

func (s ManagerInnerState) String() string {
	switch s {
	case StateReady:
		return "Ready"
	case StateRunning:
		return "Running"
	case StateInitRun:
		return "InitRun"
	case StateStopped:
		return "Stopped"
	case StateKilled:
		return "Killed"
	case StateCrashed:
		return "Crashed"
	case StateWaitRecover:
		return "WaitRecover"
	default:
		return "Unknown"
	}
}

// SignalPriority defines the priority order for signal processing.
// Lower numeric values indicate higher priority.
type SignalPriority uint8

const (
	// PriorityManagerInner is the highest priority (Watcher state control)
	PriorityManagerInner SignalPriority = 0
	// PriorityUser is medium priority (user messages)
	PriorityUser SignalPriority = 1
	// PriorityVar is the lowest priority (watched variable changes)
	PriorityVar SignalPriority = 2
)

// Signal is the interface that all signal types must implement.
// Signals trigger state transitions in the Watcher.
type Signal interface {
	Priority() SignalPriority
	CreatedAt() time.Time
	String() string
}

// Message represents a user-sent message to a managed function.
type Message struct {
	Content    string
	IsConsumed bool
	ReceivedAt time.Time
}

// String returns the message content.
func (m *Message) String() string {
	if m == nil {
		return ""
	}
	return m.Content
}

// HershContext provides runtime context for managed functions.
// It includes cancellation, deadlines, and access to Watcher features.
// This is the base interface used by manager package.
type HershContext interface {
	context.Context

	// WatcherID returns the unique identifier of the current Watcher
	WatcherID() string

	// Message returns the current user message (nil if none)
	Message() *Message

	// GetValue retrieves a value stored in the context by key
	// Returns nil if the key does not exist
	// WARNING: Returns the actual stored value (not a copy)
	// Mutating returned pointers will affect the stored state
	GetValue(key string) any

	// SetValue stores a value in the context by key
	// This allows managed functions to maintain state across executions
	// The framework automatically tracks changes for monitoring
	SetValue(key string, value any)

	// UpdateValue provides a safe way to update context values
	// The updateFn receives a copy of the current value and returns the new value
	// This ensures immutability and proper change tracking
	// Returns the new value after update
	UpdateValue(key string, updateFn func(current any) any) any

	// GetWatcher returns the watcher reference
	// Returns any to avoid circular dependency with hersh package
	GetWatcher() any

	// GetEnv returns the environment variable value for the given key
	// The second return value (ok) is true if the key exists, false otherwise
	// Environment variables are immutable after Watcher initialization
	GetEnv(key string) (string, bool)
}

// WatcherConfig holds configuration for creating a new Watcher.
type WatcherConfig struct {
	// ScheduleInfo contains scheduling information for the Watcher
	ScheduleInfo string

	// UserInfo contains user identification and metadata
	UserInfo string

	// ServerPort is the port number for the Watcher server
	ServerPort int

	// DefaultTimeout is the default timeout for managed function execution
	DefaultTimeout time.Duration

	// RecoveryPolicy defines how the Watcher handles failures
	RecoveryPolicy RecoveryPolicy

	// Resource limit settings for long-running stability
	MaxLogEntries      int // Maximum log entries before circular buffer truncation (default: 50,000)
	MaxWatches         int // Maximum number of concurrent watches (default: 1,000)
	MaxMemoEntries     int // Maximum number of memo cache entries (default: 1,000)
	SignalChanCapacity int // Signal channel buffer capacity (default: 50,000)
}

// RecoveryPolicy defines fault tolerance behavior.
type RecoveryPolicy struct {
	// MinConsecutiveFailures before entering recovery mode (default: 3)
	// Failures below this threshold return to Ready immediately
	MinConsecutiveFailures int

	// MaxConsecutiveFailures before crashing (default: 6)
	MaxConsecutiveFailures int

	// BaseRetryDelay is the initial delay before retry in recovery mode (default: 5s)
	// Used when failures >= MinConsecutiveFailures (exponential backoff)
	BaseRetryDelay time.Duration

	// MaxRetryDelay caps the maximum retry delay (default: 5m)
	MaxRetryDelay time.Duration

	// LightweightRetryDelays defines backoff delays for errors below MinConsecutiveFailures.
	// Example: [15s, 30s, 60s] means:
	//   - 1st failure: wait 15s → Ready
	//   - 2nd failure: wait 30s → Ready
	//   - 3rd failure: wait 60s → Ready
	//   - 4th+ failure: WaitRecover (if >= MinConsecutiveFailures)
	// If nil or empty, no delay (legacy behavior for backward compatibility).
	LightweightRetryDelays []time.Duration
}

// DefaultRecoveryPolicy returns sensible defaults.
func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		MinConsecutiveFailures: 3,
		MaxConsecutiveFailures: 6,
		BaseRetryDelay:         5 * time.Second,
		MaxRetryDelay:          5 * time.Minute,
		LightweightRetryDelays: []time.Duration{
			15 * time.Second, // 1st failure: 15s
			30 * time.Second, // 2nd failure: 30s
			60 * time.Second, // 3rd failure: 60s
		},
	}
}

// DefaultWatcherConfig returns default configuration.
func DefaultWatcherConfig() WatcherConfig {
	return WatcherConfig{
		ServerPort:         8080,
		DefaultTimeout:     1 * time.Minute,
		RecoveryPolicy:     DefaultRecoveryPolicy(),
		MaxLogEntries:      50_000,
		MaxWatches:         1_000,
		MaxMemoEntries:     1_000,
		SignalChanCapacity: 50_000,
	}
}
