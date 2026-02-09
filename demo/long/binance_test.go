package main

import (
	"fmt"
	"testing"
	"time"
)

func TestBinanceWebSocketConnection(t *testing.T) {
	fmt.Println("Testing Binance WebSocket connection...")

	// Create stream
	stream := NewBinanceStream()

	// Connect
	err := stream.Connect()
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer stream.Close()

	fmt.Println("✅ Connected successfully")

	// Wait for initial prices
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	pricesReceived := false
	for !pricesReceived {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for prices")
		case <-ticker.C:
			btc := stream.GetCurrentBTC()
			eth := stream.GetCurrentETH()
			if btc > 0 && eth > 0 {
				fmt.Printf("✅ Prices received: BTC=$%.2f, ETH=$%.2f\n", btc, eth)
				pricesReceived = true
			}
		}
	}

	// Test channel data reception
	fmt.Println("\nTesting channel data...")
	btcChan := stream.GetBTCPriceChan()
	ethChan := stream.GetETHPriceChan()

	btcReceived := false
	ethReceived := false

	timeout = time.After(10 * time.Second)
	for !btcReceived || !ethReceived {
		select {
		case <-timeout:
			if !btcReceived {
				t.Error("No BTC price on channel")
			}
			if !ethReceived {
				t.Error("No ETH price on channel")
			}
			t.FailNow()

		case price := <-btcChan:
			if !btcReceived {
				fmt.Printf("✅ BTC channel received: $%.2f\n", price.(float64))
				btcReceived = true
			}

		case price := <-ethChan:
			if !ethReceived {
				fmt.Printf("✅ ETH channel received: $%.2f\n", price.(float64))
				ethReceived = true
			}
		}
	}

	// Test stats
	stats := stream.GetStats()
	fmt.Printf("\n📊 Stats:\n")
	fmt.Printf("   Messages: %d\n", stats.MessagesReceived)
	fmt.Printf("   Reconnects: %d\n", stats.Reconnects)
	fmt.Printf("   Errors: %d\n", stats.Errors)
	fmt.Printf("   Connected: %v\n", stats.Connected)

	if stats.MessagesReceived == 0 {
		t.Error("No messages received")
	}
	if !stats.Connected {
		t.Error("Not connected")
	}

	fmt.Println("\n✅ All tests passed!")
}

func TestBinanceChannelData(t *testing.T) {
	fmt.Println("Testing Binance channel data flow...")

	stream := NewBinanceStream()
	if err := stream.Connect(); err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer stream.Close()

	// Wait for initial connection
	time.Sleep(2 * time.Second)

	btcChan := stream.GetBTCPriceChan()
	ethChan := stream.GetETHPriceChan()

	// Collect 10 prices from each
	btcPrices := make([]float64, 0, 10)
	ethPrices := make([]float64, 0, 10)

	timeout := time.After(30 * time.Second)
	for len(btcPrices) < 10 || len(ethPrices) < 10 {
		select {
		case <-timeout:
			t.Fatalf("Timeout: BTC=%d, ETH=%d prices received", len(btcPrices), len(ethPrices))

		case price := <-btcChan:
			btcPrices = append(btcPrices, price.(float64))
			if len(btcPrices) <= 3 {
				fmt.Printf("BTC #%d: $%.2f\n", len(btcPrices), price.(float64))
			}

		case price := <-ethChan:
			ethPrices = append(ethPrices, price.(float64))
			if len(ethPrices) <= 3 {
				fmt.Printf("ETH #%d: $%.2f\n", len(ethPrices), price.(float64))
			}
		}
	}

	fmt.Printf("\n✅ Received %d BTC and %d ETH prices\n", len(btcPrices), len(ethPrices))

	// Check for variation
	btcMin, btcMax := btcPrices[0], btcPrices[0]
	for _, p := range btcPrices {
		if p < btcMin {
			btcMin = p
		}
		if p > btcMax {
			btcMax = p
		}
	}

	fmt.Printf("BTC range: $%.2f - $%.2f (variation: $%.2f)\n", btcMin, btcMax, btcMax-btcMin)

	if btcMin == btcMax {
		t.Error("BTC prices are not changing")
	}

	fmt.Println("✅ Channel data test passed!")
}
