# Long-Running Trading Simulator Demo

A comprehensive demonstration of the Hersh framework's long-running stability with real-time cryptocurrency trading simulation.

## 🎯 Overview

This demo validates the Hersh framework's ability to run continuously for extended periods (4.5+ hours) while:
- Processing high-frequency WebSocket data streams from Binance
- Managing concurrent watches and goroutines
- Handling user commands during execution
- Maintaining responsive API server
- Managing memory and resources effectively

## 🏗️ Architecture

### Components

1. **BinanceStream** (`binance_stream.go`)
   - WebSocket client for real-time BTC/ETH price data
   - Auto-reconnection on disconnection
   - Statistics tracking (messages, reconnects, errors)
   - Dual price channels for WatchFlow integration

2. **TradingSimulator** (`trading_sim.go`)
   - Dry-run trading with $10,000 initial capital
   - Three trading strategies:
     - Moving Average Crossover (MA5 vs MA15)
     - Volatility Detection (±2% changes)
     - Portfolio Rebalancing (hourly)
   - Complete trade history and portfolio tracking

3. **StatsCollector** (`stats.go`)
   - 1-minute automatic statistics reporting
   - Detailed statistics on demand
   - Memory and system monitoring

4. **CommandHandler** (`commands.go`)
   - Interactive command processing
   - Portfolio management controls
   - Real-time statistics viewing

5. **Main Watcher** (`main.go`)
   - Hersh Watcher integration
   - WatchFlow for price updates
   - WatchCall for periodic tasks
   - User message handling

## 🚀 Quick Start

### Prerequisites

```bash
# Install dependencies
cd demo/long
go mod download
```

### Running the Demo

```bash
# Start the long-running demo
go run .
```

The demo will:
1. Connect to Binance WebSocket
2. Wait for initial price data
3. Start the Hersh Watcher
4. Begin trading simulation
5. Print statistics every 1 minute
6. Rebalance portfolio every 1 hour
7. Accept user commands via stdin

### Stopping the Demo

Press `Ctrl+C` to gracefully shutdown the demo.

## 📊 Available Commands

### Statistics
- `status` or `s` - Quick status summary
- `stats` or `st` - 1-minute stats report
- `detailed` or `d` - Comprehensive statistics
- `portfolio` or `p` - Detailed portfolio information

### Trading
- `trades` or `t` - Show last 10 trades
- `trades20` or `t20` - Show last 20 trades
- `trades50` or `t50` - Show last 50 trades
- `pause` - Pause trading strategy
- `resume` - Resume trading
- `rebalance` or `r` - Force portfolio rebalance

### Market
- `prices` or `price` - Current BTC/ETH prices

### Other
- `help` or `h` or `?` - Show help message
- `quit` or `exit` or `q` - Exit instructions

## 📈 Trading Strategy

### 1. Moving Average Crossover
- **Golden Cross**: MA5 crosses above MA15 → Buy $100
- **Death Cross**: MA5 crosses below MA15 → Sell 50% of holdings

### 2. Volatility Detection
- **Rapid Rise** (>2% in 10 ticks): Sell 30% → Take profit
- **Rapid Drop** (<-2% in 10 ticks): Buy $50 → Buy the dip

### 3. Portfolio Rebalancing (Hourly)
- Target allocation: 50% BTC, 50% ETH, 20% cash reserve
- Rebalance if difference > $50

## 🔍 What's Being Tested

### Framework Features
1. **WatchFlow** - High-frequency channel-based reactivity (2 watches: BTC, ETH)
2. **WatchCall** - Periodic task execution (2 watches: stats, rebalance)
3. **Memo Cache** - Caching components (BinanceStream)
4. **Context Storage** - State management (TradingSimulator, stats)
5. **Message Handling** - User command processing
6. **API Server** - Responsiveness during long operations

### Stability Features
1. **Log Circular Buffer** - Prevents unbounded log growth (50K limit)
2. **Watch Registry Limits** - Prevents goroutine explosion (1K limit)
3. **Memo Cache Limits** - Prevents memory accumulation (1K limit)
4. **Signal Channel Capacity** - High-throughput signal handling (50K buffer)
5. **Graceful Shutdown** - Clean termination with WaitGroup (1-min timeout)
6. **Panic Recovery** - Reducer panic handling with crash state transition

### High-Load Scenarios
1. **WebSocket Stream** - Continuous real-time data processing
2. **Concurrent Watches** - Multiple simultaneous reactive flows
3. **User Interaction** - Command processing during execution
4. **Memory Management** - Long-running memory stability
5. **Goroutine Management** - No goroutine leaks over time

## 📋 Expected Output

### Startup
```
════════════════════════════════════════════════════════════════════════════════
🚀 Long-Running Trading Simulator v1.0.0
════════════════════════════════════════════════════════════════════════════════
⏱️  Target Duration: 4h30m0s
📊 Stats Interval: 1m0s
💼 Initial Capital: $10000.00
════════════════════════════════════════════════════════════════════════════════

🔧 Initializing components...
   ✅ BinanceStream created
   ✅ TradingSimulator created
   ✅ StatsCollector created
   ✅ CommandHandler created

🌐 Connecting to Binance WebSocket...
   ✅ Connected to wss://stream.binance.com:9443

⏳ Waiting for initial price data...
   ✅ Initial prices received: BTC=$98765.43, ETH=$3456.78

🔍 Creating Hersh Watcher...
   ✅ Watcher created with long-running config
   ✅ Components stored in context

▶️  Starting main trading loop...
   Type 'help' for available commands
   Press Ctrl+C to stop
```

### 1-Minute Stats (Automatic)
```
════════════════════════════════════════════════════════════════════════════════
⏰ Stats Report - 2026-02-02 15:23:45
════════════════════════════════════════════════════════════════════════════════
📊 Uptime: 1m 23s

🌐 WebSocket Status:
   Connected: true
   Messages Received: 452
   Reconnects: 0
   Errors: 0
   Last Update: 1s ago

💰 Current Prices:
   BTC: $98765.43
   ETH: $3456.78

💼 Portfolio:
   Total Value: $10123.45 (1.23%)
   Cash: $9234.56
   BTC: 0.005432 ($536.21)
   ETH: 0.098765 ($341.23)

📈 Trading:
   Total Trades: 3
   Trading: true

🖥️  System:
   Memory: 12.3 MB
   Goroutines: 15
════════════════════════════════════════════════════════════════════════════════
```

### Trade Execution
```
💹 Strategy executed 1 trades:
   15:24:32 BUY BTC: 0.001012 @ $98765.43 (golden_cross)
   Portfolio Value: $10145.67 (1.46%)
```

### Hourly Rebalance
```
⏰ Hourly rebalance triggered...
   Executed 2 rebalancing trades
      SELL BTC: 0.000234 @ $98876.54
      BUY ETH: 0.012345 @ $3467.89
```

## 🎯 Target Duration

The demo is designed to run for **4 hours 30 minutes** (4.5 hours) continuously. After reaching the target duration, a message will be displayed, but the demo will continue running until manually stopped.

This duration is chosen to validate:
- Memory stability over extended periods
- Goroutine lifecycle management
- Resource cleanup effectiveness
- WebSocket connection resilience
- Framework recovery mechanisms

## 🔧 Configuration

### Watcher Config
- Default Timeout: 10 minutes
- Min Consecutive Failures: 5
- Max Consecutive Failures: 10
- Base Retry Delay: 10 seconds
- Max Retry Delay: 5 minutes

### Resource Limits
- Max Log Entries: 50,000
- Max Watches: 1,000
- Max Memo Entries: 1,000
- Signal Channel Capacity: 50,000

### Trading Config
- Initial Capital: $10,000 USD
- Stats Interval: 1 minute
- Rebalance Interval: 1 hour
- MA Periods: 5 and 15
- Volatility Threshold: ±2%
- Price History: 100 points

## 🐛 Troubleshooting

### WebSocket Connection Issues
If the WebSocket fails to connect:
1. Check internet connectivity
2. Verify Binance API is accessible
3. Check for firewall/proxy issues
4. Retry will happen automatically on reconnect

### High Memory Usage
If memory usage is high:
1. Check the stats with `detailed` command
2. Verify log circular buffer is working (50K limit)
3. Check watch count (should not exceed 1K)
4. Monitor goroutine count

### Trading Not Executing
If no trades are happening:
1. Check if trading is paused: `status`
2. Verify price data is coming in: `prices`
3. Check strategy conditions (MA crossover, volatility)
4. Use `resume` to unpause if needed

## 📝 Notes

- This is a **dry-run simulation** - no real trading occurs
- Price data is real-time from Binance public WebSocket
- All trades are simulated based on current market prices
- Portfolio calculations include realistic slippage assumptions
- The demo can run indefinitely beyond the 4.5-hour target

## 🙏 Acknowledgments

- Binance for providing public WebSocket API
- Gorilla WebSocket for Go WebSocket implementation
- Hersh framework for reactive execution engine
