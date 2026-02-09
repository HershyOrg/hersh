package hersh

import "fmt"

// Memo caches a computed value for the duration of the Watcher session.
// On first call, it computes and caches the value.
// On subsequent calls, it returns the cached value.
//
// Memo is synchronous and does NOT trigger re-execution.
// It's useful for expensive initialization that should happen once.
//
// Example:
//
//	client := hersh.Memo(func() any {
//	    return expensive.NewClient()
//	}, "apiClient", ctx).(*Client)
func Memo(computeValue func() any, memoName string, ctx HershContext) any {
	w := getWatcherFromContext(ctx)
	if w == nil {
		panic("Memo called with invalid HershContext")
	}

	memoCache := w.manager.GetMemoCache()

	cached, exists := memoCache.Load(memoName)
	if exists {
		return cached
	}

	// Compute value
	value := computeValue()

	// Cache it (LoadOrStore handles race conditions)
	actual, loaded := memoCache.LoadOrStore(memoName, value)
	if loaded {
		// Another goroutine computed it first, use that value
		return actual
	}

	// Log the memoization (only if we stored it)
	if logger := w.GetLogger(); logger != nil {
		logger.LogEffect(fmt.Sprintf("Memo[%s] = %v", memoName, value))
	}

	return value
}

// ClearMemo removes a memoized value, forcing recomputation on next Memo call.
func ClearMemo(memoName string, ctx HershContext) {
	w := getWatcherFromContext(ctx)
	if w == nil {
		panic("ClearMemo called with invalid HershContext")
	}

	memoCache := w.manager.GetMemoCache()
	memoCache.Delete(memoName)

	if logger := w.GetLogger(); logger != nil {
		logger.LogEffect(fmt.Sprintf("Memo[%s] cleared", memoName))
	}
}
