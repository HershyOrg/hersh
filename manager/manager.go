package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/HershyOrg/hershy/hersh/shared"
)

// WatchHandle is an interface for different types of watch mechanisms.
type WatchHandle interface {
	GetVarName() string
	GetCancelFunc() context.CancelFunc
}

// TickHandle represents a tick-based watch variable.
type TickHandle struct {
	VarName            string
	GetComputationFunc func() (VarUpdateFunc, error) // Returns a function to compute next state
	Tick               time.Duration
	CancelFunc         context.CancelFunc
}

func (h *TickHandle) GetVarName() string                { return h.VarName }
func (h *TickHandle) GetCancelFunc() context.CancelFunc { return h.CancelFunc }

// FlowHandle represents a channel-based watch variable.
type FlowHandle struct {
	VarName    string
	SourceChan <-chan any
	CancelFunc context.CancelFunc
}

func (h *FlowHandle) GetVarName() string                { return h.VarName }
func (h *FlowHandle) GetCancelFunc() context.CancelFunc { return h.CancelFunc }

// Manager encapsulates all Manager components.
// It orchestrates the Reducer-Effect pattern for reactive execution.
type Manager struct {
	config *shared.WatcherConfig

	logger *Logger

	state   *ManagerState
	signals *SignalChannels

	reducer   *Reducer
	commander *EffectCommander
	handler   *EffectHandler

	memoCache     sync.Map // map[string]any
	watchRegistry sync.Map // map[string]WatchHandle
}

// NewManager creates a new WatcherManager with core components initialized.
// The Manager creates and owns its logger internally.
// ManagedFunc should be set later via SetManagedFunc().
func NewManager(config shared.WatcherConfig) *Manager {
	// Initialize logger with config limit
	logger := NewLogger(config.MaxLogEntries)

	// Initialize Manager components (start in Ready, will transition to InitRun on Start)
	state := NewManagerState(shared.StateReady)
	signals := NewSignalChannels(config.SignalChanCapacity)

	// Create reducer (no longer needs ActionChannel)
	reducer := NewReducer(state, signals, logger)

	// Create commander (now synchronous, no channels)
	commander := NewEffectCommander()

	// Create handler (no longer needs effectCh)
	handler := NewEffectHandler(
		nil, // ManagedFunc to be set later
		nil, // Cleaner to be set later
		state,
		signals,
		logger,
		config,
	)

	return &Manager{
		config:        &config,
		logger:        logger,
		state:         state,
		signals:       signals,
		reducer:       reducer,
		commander:     commander,
		handler:       handler,
		memoCache:     sync.Map{},
		watchRegistry: sync.Map{},
	}
}

// Start starts the Reducer loop (synchronous architecture).
// Only Reducer runs in a goroutine now - it calls Commander and Handler synchronously.
func (wm *Manager) Start(rootCtx context.Context) {
	// Pass commander and handler to reducer so it can call them synchronously
	go wm.reducer.RunWithEffects(rootCtx, wm.commander, wm.handler)
}

// GetState returns the current ManagerState.
func (wm *Manager) GetState() *ManagerState {
	return wm.state
}

// GetSignals returns the SignalChannels.
func (wm *Manager) GetSignals() *SignalChannels {
	return wm.signals
}

// GetEffectHandler returns the EffectHandler.
func (wm *Manager) GetEffectHandler() *EffectHandler {
	return wm.handler
}

// GetLogger returns the Manager's logger.
func (wm *Manager) GetLogger() *Logger {
	return wm.logger
}

// SetManagedFunc sets the managed function and cleaner.
// This should be called after Manager initialization, typically by Watcher.Manage().
func (wm *Manager) SetManagedFunc(managedFunc ManagedFunc, cleaner Cleaner) {
	wm.handler.SetManagedFunc(managedFunc)
	wm.handler.SetCleaner(cleaner)
}

// HasManagedFunc returns whether a managed function has been registered.
func (wm *Manager) HasManagedFunc() bool {
	return wm.handler.HasManagedFunc()
}

// GetMemoCache returns a pointer to the memo cache.
func (wm *Manager) GetMemoCache() *sync.Map {
	return &wm.memoCache
}

// GetWatchRegistry returns a pointer to the watch registry.
func (wm *Manager) GetWatchRegistry() *sync.Map {
	return &wm.watchRegistry
}

// GetConfig returns the WatcherConfig.
func (wm *Manager) GetConfig() *shared.WatcherConfig {
	return wm.config
}

// SetMemo stores a value in the memo cache with size limit enforcement.
// Returns error if cache limit is reached.
func (wm *Manager) SetMemo(key string, value any) error {
	// Check if updating existing entry (allowed)
	if _, exists := wm.memoCache.Load(key); !exists {
		// New entry - check size limit
		count := 0
		wm.memoCache.Range(func(_, _ any) bool {
			count++
			return true
		})

		if count >= wm.config.MaxMemoEntries {
			return fmt.Errorf("memo cache limit reached: %d/%d (cannot cache '%s')",
				count, wm.config.MaxMemoEntries, key)
		}
	}

	wm.memoCache.Store(key, value)
	return nil
}

// GetMemo retrieves a value from the memo cache.
func (wm *Manager) GetMemo(key string) (any, bool) {
	return wm.memoCache.Load(key)
}
