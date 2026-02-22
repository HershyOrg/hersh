package main

import (
	"fmt"
	"strings"

	"github.com/HershyOrg/hersh"
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
func (ch *CommandHandler) HandleCommand(cmd string, ctx hersh.HershContext) {
	cmd = strings.TrimSpace(strings.ToLower(cmd))

	switch cmd {
	case "help", "h", "?":
		ch.printHelp(ctx)

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
		ch.pauseTrading(ctx)

	case "resume":
		ch.resumeTrading(ctx)

	case "rebalance", "r":
		ch.rebalance(ctx)

	case "prices", "price":
		ch.printPrices(ctx)

	case "quit", "exit", "q":
		hersh.PrintWithLog("\n⚠️  Use Ctrl+C to stop the demo gracefully", ctx)

	default:
		hersh.PrintWithLog(fmt.Sprintf("\n❌ Unknown command: '%s'", cmd), ctx)
		hersh.PrintWithLog("💡 Type 'help' to see available commands", ctx)
	}
}

// printHelp prints available commands
func (ch *CommandHandler) printHelp(ctx hersh.HershContext) {
	hersh.PrintWithLog("\n"+strings.Repeat("═", 80), ctx)
	hersh.PrintWithLog("📖 AVAILABLE COMMANDS", ctx)
	hersh.PrintWithLog(strings.Repeat("═", 80), ctx)

	hersh.PrintWithLog("\n📊 Statistics:", ctx)
	hersh.PrintWithLog("   status, s          Show quick status summary", ctx)
	hersh.PrintWithLog("   stats, st          Show 1-minute stats report", ctx)
	hersh.PrintWithLog("   detailed, d        Show comprehensive detailed statistics", ctx)
	hersh.PrintWithLog("   portfolio, p       Show detailed portfolio information", ctx)

	hersh.PrintWithLog("\n📈 Trading:", ctx)
	hersh.PrintWithLog("   trades, t          Show last 10 trades", ctx)
	hersh.PrintWithLog("   trades20, t20      Show last 20 trades", ctx)
	hersh.PrintWithLog("   trades50, t50      Show last 50 trades", ctx)
	hersh.PrintWithLog("   pause              Pause trading (stop strategy execution)", ctx)
	hersh.PrintWithLog("   resume             Resume trading", ctx)
	hersh.PrintWithLog("   rebalance, r       Force portfolio rebalance", ctx)

	hersh.PrintWithLog("\n💰 Market:", ctx)
	hersh.PrintWithLog("   prices, price      Show current BTC/ETH prices", ctx)

	hersh.PrintWithLog("\n❓ Other:", ctx)
	hersh.PrintWithLog("   help, h, ?         Show this help message", ctx)
	hersh.PrintWithLog("   quit, exit, q      Exit instructions (use Ctrl+C)", ctx)

	hersh.PrintWithLog(strings.Repeat("═", 80), ctx)
}

// pauseTrading pauses trading strategy
func (ch *CommandHandler) pauseTrading(ctx hersh.HershContext) {
	if ch.ts.IsPaused() {
		hersh.PrintWithLog("\n⚠️  Trading is already paused", ctx)
		return
	}

	ch.ts.Pause()
	hersh.PrintWithLog("\n⏸️  Trading PAUSED", ctx)
	hersh.PrintWithLog("   Strategy execution stopped", ctx)
	hersh.PrintWithLog("   Price monitoring continues", ctx)
	hersh.PrintWithLog("   Type 'resume' to restart trading", ctx)
}

// resumeTrading resumes trading strategy
func (ch *CommandHandler) resumeTrading(ctx hersh.HershContext) {
	if !ch.ts.IsPaused() {
		hersh.PrintWithLog("\n⚠️  Trading is already active", ctx)
		return
	}

	ch.ts.Resume()
	hersh.PrintWithLog("\n▶️  Trading RESUMED", ctx)
	hersh.PrintWithLog("   Strategy execution restarted", ctx)
}

// rebalance forces portfolio rebalance
func (ch *CommandHandler) rebalance(ctx hersh.HershContext) {
	hersh.PrintWithLog("\n🔄 Rebalancing portfolio...", ctx)

	trades := ch.ts.Rebalance()

	if len(trades) == 0 {
		hersh.PrintWithLog("   No rebalancing needed (positions already balanced)", ctx)
		return
	}

	hersh.PrintWithLog(fmt.Sprintf("   Executed %d rebalancing trades:", len(trades)), ctx)
	for _, t := range trades {
		hersh.PrintWithLog(fmt.Sprintf("      %s %s: %.6f @ $%.2f = $%.2f",
			t.Action, t.Symbol, t.Amount, t.Price, t.USDValue), ctx)
	}

	portfolio := ch.ts.GetPortfolio()
	hersh.PrintWithLog(fmt.Sprintf("   New Portfolio Value: $%.2f", portfolio.CurrentValue), ctx)
}

// printPrices prints current market prices
func (ch *CommandHandler) printPrices(ctx hersh.HershContext) {
	btcPrice := ch.bs.GetCurrentBTC()
	ethPrice := ch.bs.GetCurrentETH()
	streamStats := ch.bs.GetStats()

	hersh.PrintWithLog("\n"+strings.Repeat("-", 50), ctx)
	hersh.PrintWithLog("💰 Current Market Prices", ctx)
	hersh.PrintWithLog(strings.Repeat("-", 50), ctx)

	if btcPrice == 0 || ethPrice == 0 {
		hersh.PrintWithLog("   ⚠️  Prices not available yet", ctx)
		hersh.PrintWithLog(fmt.Sprintf("   WebSocket Connected: %v", streamStats.Connected), ctx)
		hersh.PrintWithLog(strings.Repeat("-", 50), ctx)
		return
	}

	hersh.PrintWithLog(fmt.Sprintf("   🟠 BTC/USDT: $%.2f", btcPrice), ctx)
	hersh.PrintWithLog(fmt.Sprintf("   🔵 ETH/USDT: $%.2f", ethPrice), ctx)
	hersh.PrintWithLog(fmt.Sprintf("   📡 WebSocket: %v", streamStats.Connected), ctx)
	hersh.PrintWithLog(fmt.Sprintf("   📨 Messages: %d", streamStats.MessagesReceived), ctx)

	hersh.PrintWithLog(strings.Repeat("-", 50), ctx)
}
