package hutil

import (
	"testing"
	"time"
)

func TestTicker(t *testing.T) {
	tickerChan := Ticker(100 * time.Millisecond)

	// Should NOT receive initial value immediately
	select {
	case <-tickerChan:
		t.Fatal("Ticker should NOT send initial value")
	case <-time.After(50 * time.Millisecond):
		// Expected: no value yet
	}

	// Should receive first tick after interval
	select {
	case val := <-tickerChan:
		if _, ok := val.(time.Time); !ok {
			t.Fatalf("Expected time.Time, got %T", val)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("Ticker did not send value after interval")
	}
}

func TestTickerWithInit(t *testing.T) {
	tickerChan := TickerWithInit(100 * time.Millisecond)

	// Should receive initial value immediately
	select {
	case val := <-tickerChan:
		if _, ok := val.(time.Time); !ok {
			t.Fatalf("Expected time.Time, got %T", val)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("TickerWithInit did not send initial value")
	}

	// Should receive second value after interval
	select {
	case val := <-tickerChan:
		if _, ok := val.(time.Time); !ok {
			t.Fatalf("Expected time.Time, got %T", val)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("TickerWithInit did not send periodic value")
	}
}

func TestTickerNonBlocking(t *testing.T) {
	tickerChan := Ticker(20 * time.Millisecond)

	// Let ticker fill the buffer
	time.Sleep(250 * time.Millisecond)

	// Drain values with timeout
	count := 0
	timeout := time.After(100 * time.Millisecond)
drainLoop:
	for {
		select {
		case <-tickerChan:
			count++
		case <-timeout:
			break drainLoop
		}
	}

	if count == 0 {
		t.Fatal("Expected to receive some ticks")
	}

	// Should receive at least 1 tick, but not infinite (proving non-blocking drop)
	t.Logf("Received %d ticks (buffer size: %d)", count, defaultBufferSize)
}

func TestTickerValueNotNil(t *testing.T) {
	tickerChan := TickerWithInit(50 * time.Millisecond)

	// Receive 3 values
	for i := 0; i < 3; i++ {
		select {
		case val := <-tickerChan:
			if val == nil {
				t.Fatal("Ticker sent nil value")
			}
			tickTime, ok := val.(time.Time)
			if !ok {
				t.Fatalf("Expected time.Time, got %T", val)
			}
			if tickTime.IsZero() {
				t.Fatal("Ticker sent zero time.Time")
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("Timeout waiting for tick #%d", i+1)
		}
	}
}

func TestTickerMultipleConcurrent(t *testing.T) {
	ticker1 := TickerWithInit(50 * time.Millisecond)
	ticker2 := TickerWithInit(70 * time.Millisecond)

	// Both should work independently
	timeout := time.After(200 * time.Millisecond)

	received1, received2 := false, false

	for !received1 || !received2 {
		select {
		case val := <-ticker1:
			if _, ok := val.(time.Time); !ok {
				t.Fatalf("Ticker1: expected time.Time, got %T", val)
			}
			received1 = true
		case val := <-ticker2:
			if _, ok := val.(time.Time); !ok {
				t.Fatalf("Ticker2: expected time.Time, got %T", val)
			}
			received2 = true
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent tickers")
		}
	}
}
