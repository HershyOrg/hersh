package main

import (
	"fmt"
	"strings"
)

// CommandHandler handles user commands
type CommandHandler struct {
	bs    *BinanceStream
	ts    *TradingSimulator
	stats *StatsCollector
}

// NewCommandHandler creates a new command handler
func NewCommandHandler(bs *BinanceStream, ts *TradingSimulator, stats *StatsCollector) *CommandHandler {
	return &CommandHandler{
		bs:    bs,
		ts:    ts,
		stats: stats,
	}
}

// HandleCommand processes user commands
func (ch *CommandHandler) HandleCommand(cmd string) {
	cmd = strings.TrimSpace(strings.ToLower(cmd))

	switch cmd {
	case "help", "h", "?":
		ch.printHelp()

	case "status", "s":
		ch.stats.PrintStatus(ch.bs, ch.ts)

	case "stats", "st":
		ch.stats.PrintStats(ch.bs, ch.ts)

	case "detailed", "detail", "d":
		ch.stats.PrintDetailedStats(ch.bs, ch.ts)

	case "portfolio", "p":
		ch.stats.PrintPortfolio(ch.ts)

	case "trades", "t":
		ch.stats.PrintRecentTrades(ch.ts, 10)

	case "trades20", "t20":
		ch.stats.PrintRecentTrades(ch.ts, 20)

	case "trades50", "t50":
		ch.stats.PrintRecentTrades(ch.ts, 50)

	case "pause":
		ch.pauseTrading()

	case "resume":
		ch.resumeTrading()

	case "rebalance", "r":
		ch.rebalance()

	case "prices", "price":
		ch.printPrices()

	case "quit", "exit", "q":
		fmt.Println("\n⚠️  Use Ctrl+C to stop the demo gracefully")

	default:
		fmt.Printf("\n❌ Unknown command: '%s'\n", cmd)
		fmt.Println("💡 Type 'help' to see available commands")
	}
}

// printHelp prints available commands
func (ch *CommandHandler) printHelp() {
	fmt.Println("\n" + strings.Repeat("═", 80))
	fmt.Println("📖 AVAILABLE COMMANDS")
	fmt.Println(strings.Repeat("═", 80))

	fmt.Println("\n📊 Statistics:")
	fmt.Println("   status, s          Show quick status summary")
	fmt.Println("   stats, st          Show 1-minute stats report")
	fmt.Println("   detailed, d        Show comprehensive detailed statistics")
	fmt.Println("   portfolio, p       Show detailed portfolio information")

	fmt.Println("\n📈 Trading:")
	fmt.Println("   trades, t          Show last 10 trades")
	fmt.Println("   trades20, t20      Show last 20 trades")
	fmt.Println("   trades50, t50      Show last 50 trades")
	fmt.Println("   pause              Pause trading (stop strategy execution)")
	fmt.Println("   resume             Resume trading")
	fmt.Println("   rebalance, r       Force portfolio rebalance")

	fmt.Println("\n💰 Market:")
	fmt.Println("   prices, price      Show current BTC/ETH prices")

	fmt.Println("\n❓ Other:")
	fmt.Println("   help, h, ?         Show this help message")
	fmt.Println("   quit, exit, q      Exit instructions (use Ctrl+C)")

	fmt.Println(strings.Repeat("═", 80))
}

// pauseTrading pauses trading strategy
func (ch *CommandHandler) pauseTrading() {
	if ch.ts.IsPaused() {
		fmt.Println("\n⚠️  Trading is already paused")
		return
	}

	ch.ts.Pause()
	fmt.Println("\n⏸️  Trading PAUSED")
	fmt.Println("   Strategy execution stopped")
	fmt.Println("   Price monitoring continues")
	fmt.Println("   Type 'resume' to restart trading")
}

// resumeTrading resumes trading strategy
func (ch *CommandHandler) resumeTrading() {
	if !ch.ts.IsPaused() {
		fmt.Println("\n⚠️  Trading is already active")
		return
	}

	ch.ts.Resume()
	fmt.Println("\n▶️  Trading RESUMED")
	fmt.Println("   Strategy execution restarted")
}

// rebalance forces portfolio rebalance
func (ch *CommandHandler) rebalance() {
	fmt.Println("\n🔄 Rebalancing portfolio...")

	trades := ch.ts.Rebalance()

	if len(trades) == 0 {
		fmt.Println("   No rebalancing needed (positions already balanced)")
		return
	}

	fmt.Printf("   Executed %d rebalancing trades:\n", len(trades))
	for _, t := range trades {
		fmt.Printf("      %s %s: %.6f @ $%.2f = $%.2f\n",
			t.Action, t.Symbol, t.Amount, t.Price, t.USDValue)
	}

	portfolio := ch.ts.GetPortfolio()
	fmt.Printf("   New Portfolio Value: $%.2f\n", portfolio.CurrentValue)
}

// printPrices prints current market prices
func (ch *CommandHandler) printPrices() {
	btcPrice := ch.bs.GetCurrentBTC()
	ethPrice := ch.bs.GetCurrentETH()
	streamStats := ch.bs.GetStats()

	fmt.Println("\n" + strings.Repeat("-", 50))
	fmt.Println("💰 Current Market Prices")
	fmt.Println(strings.Repeat("-", 50))

	if btcPrice == 0 || ethPrice == 0 {
		fmt.Println("   ⚠️  Prices not available yet")
		fmt.Printf("   WebSocket Connected: %v\n", streamStats.Connected)
		fmt.Println(strings.Repeat("-", 50))
		return
	}

	fmt.Printf("   🟠 BTC/USDT: $%.2f\n", btcPrice)
	fmt.Printf("   🔵 ETH/USDT: $%.2f\n", ethPrice)
	fmt.Printf("   📡 WebSocket: %v\n", streamStats.Connected)
	fmt.Printf("   📨 Messages: %d\n", streamStats.MessagesReceived)

	fmt.Println(strings.Repeat("-", 50))
}
