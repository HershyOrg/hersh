package hutil

import "time"

const defaultBufferSize = 10

// Ticker creates a channel that sends time.Time at regular intervals.
// Does NOT send initial value immediately.
//
// The returned channel is buffered (size 10) and uses non-blocking sends.
// If the buffer is full, new ticks are dropped.
//
// Usage:
//
//	tickerChan := hutil.Ticker(1 * time.Minute)
//	tick := hersh.WatchFlow(tickerChan, "my_ticker", ctx)
func Ticker(interval time.Duration) <-chan any {
	ch := make(chan any, defaultBufferSize)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for t := range ticker.C {
			select {
			case ch <- t:
			default:
				// Non-blocking: drop if channel full
			}
		}
	}()

	return ch
}

// TickerWithInit creates a channel that sends time.Time immediately,
// then at regular intervals.
//
// This is useful when you need an initial signal for initialization,
// followed by periodic updates.
//
// Usage:
//
//	tickerChan := hutil.TickerWithInit(1 * time.Minute)
//	tick := hersh.WatchFlow(tickerChan, "stats_ticker", ctx)
func TickerWithInit(interval time.Duration) <-chan any {
	ch := make(chan any, defaultBufferSize)

	go func() {
		// Send initial value immediately
		select {
		case ch <- time.Now():
		default:
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for t := range ticker.C {
			select {
			case ch <- t:
			default:
				// Non-blocking: drop if channel full
			}
		}
	}()

	return ch
}
