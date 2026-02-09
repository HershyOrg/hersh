package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// BinanceStream handles WebSocket connection to Binance for real-time price data
type BinanceStream struct {
	// Price channels for WatchFlow (any type for hersh compatibility)
	btcPriceChan chan any
	ethPriceChan chan any

	// Current prices (atomic access)
	currentBTC atomic.Value // float64
	currentETH atomic.Value // float64

	// Statistics
	stats struct {
		messagesReceived atomic.Int64
		reconnects       atomic.Int64
		lastUpdate       atomic.Value // time.Time
		errors           atomic.Int64
	}

	// Connection state
	connected atomic.Bool
	wsConn    *websocket.Conn
	mu        sync.RWMutex

	// Control
	stopChan chan struct{}
	stopped  atomic.Bool
}

// BinanceTradeMsg represents a Binance trade message
type BinanceTradeMsg struct {
	Stream string `json:"stream"`
	Data   struct {
		Event     string `json:"e"` // Event type
		EventTime int64  `json:"E"` // Event time
		Symbol    string `json:"s"` // Symbol
		Price     string `json:"p"` // Price
		Quantity  string `json:"q"` // Quantity
		TradeTime int64  `json:"T"` // Trade time
	} `json:"data"`
}

// NewBinanceStream creates a new Binance WebSocket stream client
func NewBinanceStream() *BinanceStream {
	bs := &BinanceStream{
		btcPriceChan: make(chan any, 100),
		ethPriceChan: make(chan any, 100),
		stopChan:     make(chan struct{}),
	}

	bs.currentBTC.Store(0.0)
	bs.currentETH.Store(0.0)
	bs.stats.lastUpdate.Store(time.Now())

	return bs
}

// Connect establishes WebSocket connection to Binance
func (bs *BinanceStream) Connect() error {
	if bs.stopped.Load() {
		return fmt.Errorf("stream already stopped")
	}

	url := "wss://stream.binance.com:9443/stream?streams=btcusdt@trade/ethusdt@trade"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Binance: %w", err)
	}

	bs.mu.Lock()
	bs.wsConn = conn
	bs.mu.Unlock()

	bs.connected.Store(true)

	// Start message receiver goroutine
	go bs.receiveLoop()

	return nil
}

// receiveLoop continuously receives messages from WebSocket
func (bs *BinanceStream) receiveLoop() {
	defer func() {
		bs.connected.Store(false)

		// Close price channels when stream stops
		if bs.stopped.Load() {
			close(bs.btcPriceChan)
			close(bs.ethPriceChan)
		}
	}()

	for {
		select {
		case <-bs.stopChan:
			return
		default:
			// Continue receiving
		}

		bs.mu.RLock()
		conn := bs.wsConn
		bs.mu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Set read deadline to detect disconnections
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		var msg BinanceTradeMsg
		err := conn.ReadJSON(&msg)
		if err != nil {
			if bs.stopped.Load() {
				return
			}

			bs.stats.errors.Add(1)
			fmt.Printf("[Stream] Read error: %v\n", err)

			// Attempt reconnection
			bs.reconnect()
			continue
		}

		// Process message
		bs.processMessage(msg)
		bs.stats.messagesReceived.Add(1)
		bs.stats.lastUpdate.Store(time.Now())
	}
}

// processMessage parses and distributes price updates
func (bs *BinanceStream) processMessage(msg BinanceTradeMsg) {
	var price float64
	fmt.Sscanf(msg.Data.Price, "%f", &price)

	if price == 0 {
		return
	}

	switch msg.Stream {
	case "btcusdt@trade":
		bs.currentBTC.Store(price)
		// Non-blocking send (cast to any)
		select {
		case bs.btcPriceChan <- any(price):
		default:
			// Channel full, skip
		}

	case "ethusdt@trade":
		bs.currentETH.Store(price)
		// Non-blocking send (cast to any)
		select {
		case bs.ethPriceChan <- any(price):
		default:
			// Channel full, skip
		}
	}
}

// reconnect attempts to reconnect to Binance WebSocket
func (bs *BinanceStream) reconnect() {
	if bs.stopped.Load() {
		return
	}

	fmt.Println("[Stream] Reconnecting...")
	bs.stats.reconnects.Add(1)

	// Close old connection
	bs.mu.Lock()
	if bs.wsConn != nil {
		bs.wsConn.Close()
		bs.wsConn = nil
	}
	bs.mu.Unlock()

	bs.connected.Store(false)

	// Wait before reconnecting
	time.Sleep(2 * time.Second)

	// Try to reconnect
	err := bs.Connect()
	if err != nil {
		fmt.Printf("[Stream] Reconnection failed: %v\n", err)
	} else {
		fmt.Println("[Stream] Reconnected successfully")
	}
}

// GetBTCPriceChan returns the BTC price channel for WatchFlow
func (bs *BinanceStream) GetBTCPriceChan() <-chan any {
	return bs.btcPriceChan
}

// GetETHPriceChan returns the ETH price channel for WatchFlow
func (bs *BinanceStream) GetETHPriceChan() <-chan any {
	return bs.ethPriceChan
}

// GetCurrentBTC returns the current BTC price
func (bs *BinanceStream) GetCurrentBTC() float64 {
	if v := bs.currentBTC.Load(); v != nil {
		return v.(float64)
	}
	return 0
}

// GetCurrentETH returns the current ETH price
func (bs *BinanceStream) GetCurrentETH() float64 {
	if v := bs.currentETH.Load(); v != nil {
		return v.(float64)
	}
	return 0
}

// GetStats returns stream statistics
func (bs *BinanceStream) GetStats() StreamStats {
	lastUpdate := bs.stats.lastUpdate.Load().(time.Time)

	return StreamStats{
		MessagesReceived: bs.stats.messagesReceived.Load(),
		Reconnects:       bs.stats.reconnects.Load(),
		Errors:           bs.stats.errors.Load(),
		LastUpdate:       lastUpdate,
		Connected:        bs.connected.Load(),
	}
}

// StreamStats contains WebSocket stream statistics
type StreamStats struct {
	MessagesReceived int64
	Reconnects       int64
	Errors           int64
	LastUpdate       time.Time
	Connected        bool
}

// Close gracefully closes the WebSocket connection
func (bs *BinanceStream) Close() error {
	if !bs.stopped.CompareAndSwap(false, true) {
		return nil // Already stopped
	}

	fmt.Println("[Stream] Closing WebSocket connection...")

	// Signal stop
	close(bs.stopChan)

	// Close WebSocket connection
	bs.mu.Lock()
	if bs.wsConn != nil {
		bs.wsConn.Close()
		bs.wsConn = nil
	}
	bs.mu.Unlock()

	bs.connected.Store(false)

	fmt.Println("[Stream] WebSocket closed")
	return nil
}
