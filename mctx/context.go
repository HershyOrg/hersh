// Package ctx provides HershContext implementation for the hersh framework.
package mctx

import (
	"context"
	"sync"

	"github.com/HershyOrg/hersh/shared"
)

// Logger interface for context value logging and effect logging.
type Logger interface {
	LogContextValue(key string, oldValue, newValue any, operation string)
	LogEffect(msg string)
}

// ManageContext implements core.ManageContext interface.
// This is a concrete implementation that manages execution context,
// messages, watcher reference, and user-defined values.
type ManageContext struct {
	context.Context
	message         *shared.Message
	triggeredSignal *shared.TriggeredSignal
	watcher         any // Watcher reference (stored as any to avoid circular dependency with hersh package)
	valueStore      map[string]any
	envVarMap       map[string]string // Environment variables (immutable after initialization)
	valuesMutex     sync.RWMutex
	logger          Logger
}

// New creates a new HershContext with the given parameters.
func New(ctx context.Context, logger Logger) *ManageContext {
	return &ManageContext{
		Context:    ctx,
		message:    nil,
		watcher:    nil,
		valueStore: make(map[string]any),
		envVarMap:  make(map[string]string),
		logger:     logger,
	}
}

func (mc *ManageContext) Message() *shared.Message {
	return mc.message
}

func (mc *ManageContext) GetTriggeredSignal() *shared.TriggeredSignal {
	mc.valuesMutex.RLock()
	defer mc.valuesMutex.RUnlock()
	return mc.triggeredSignal
}

func (mc *ManageContext) GetValue(key string) any {
	mc.valuesMutex.RLock()
	defer mc.valuesMutex.RUnlock()
	return mc.valueStore[key]
}

func (mc *ManageContext) SetValue(key string, value any) {
	mc.valuesMutex.Lock()
	defer mc.valuesMutex.Unlock()

	oldValue := mc.valueStore[key]
	mc.valueStore[key] = value

	// Log the state change
	if mc.logger != nil {
		mc.logger.LogContextValue(key, oldValue, value, "initialized")
		if oldValue != nil {
			mc.logger.LogContextValue(key, oldValue, value, "updated")
		}
	}
}

func (mc *ManageContext) UpdateValue(key string, updateFn func(current any) any) any {
	mc.valuesMutex.Lock()
	defer mc.valuesMutex.Unlock()

	// Get current value
	currentValue := mc.valueStore[key]

	// Create a deep copy to pass to updateFn
	currentCopy := shared.DeepCopy(currentValue)

	// Call the update function with the copy
	newValue := updateFn(currentCopy)

	// Store the new value
	oldValue := mc.valueStore[key]
	mc.valueStore[key] = newValue

	// Log the state change
	if mc.logger != nil {
		if oldValue == nil {
			mc.logger.LogContextValue(key, nil, newValue, "initialized")
		} else {
			mc.logger.LogContextValue(key, oldValue, newValue, "updated")
		}
	}

	return newValue
}

// SetWatcher sets the watcher reference.
// This is called internally by the framework.
func (mc *ManageContext) SetWatcher(watcher any) {
	mc.watcher = watcher
}

// GetWatcher returns the watcher reference as any.
// Use this from manager package which doesn't know about hersh.Watcher type.
func (mc *ManageContext) GetWatcher() any {
	return mc.watcher
}

// SetMessage updates the current message.
// This is called internally by the framework during execution.
func (mc *ManageContext) SetMessage(msg *shared.Message) {
	mc.message = msg
}

// SetTriggeredSignal updates the triggered signal information.
// This is called internally by the framework during execution.
func (mc *ManageContext) SetTriggeredSignal(ts *shared.TriggeredSignal) {
	mc.valuesMutex.Lock()
	defer mc.valuesMutex.Unlock()
	mc.triggeredSignal = ts
}

// UpdateContext replaces the underlying context.
// This is used by EffectHandler when creating execution contexts with timeouts.
func (mc *ManageContext) UpdateContext(ctx context.Context) {
	mc.Context = ctx
}

// GetEnv returns the environment variable value for the given key.
// The second return value (ok) is true if the key exists, false otherwise.
// This method is safe for concurrent access as envVarMap is immutable after initialization.
func (mc *ManageContext) GetEnv(key string) (string, bool) {
	mc.valuesMutex.RLock()
	defer mc.valuesMutex.RUnlock()
	val, ok := mc.envVarMap[key]
	return val, ok
}

// SetEnvVars sets the environment variables for this context.
// This should only be called during initialization (by Watcher.NewWatcher).
// The envVars map is deep copied to ensure immutability.
func (mc *ManageContext) SetEnvVars(envVars map[string]string) {
	mc.valuesMutex.Lock()
	defer mc.valuesMutex.Unlock()

	// Deep copy for immutability
	mc.envVarMap = make(map[string]string)
	if envVars != nil {
		for k, v := range envVars {
			mc.envVarMap[k] = v
		}
	}
}

// GetLogger returns the logger instance.
// This is used by hersh.Log and hersh.PrintWithLog functions.
func (mc *ManageContext) GetLogger() Logger {
	return mc.logger
}
