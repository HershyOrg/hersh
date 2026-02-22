package test

import (
	"context"
	"testing"
	"time"

	"github.com/HershyOrg/hersh"
	"github.com/HershyOrg/hersh/hutil"
	"github.com/HershyOrg/hersh/manager"
	"github.com/HershyOrg/hersh/shared"
)

// Test 1: WatchCall trigger detection
func TestTriggeredSignal_WatchCall(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	counter := 0
	triggeredVars := []string{}

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		// WatchCall: 100ms마다 카운터 증가
		counterHV := hersh.WatchCall(func() (manager.VarUpdateFunc, error) {
			return func(prev shared.HershValue) (shared.HershValue, bool, error) {
				counter++
				return shared.HershValue{Value: counter}, true, nil
			}, nil
		}, "counter", 100*time.Millisecond, runCtx)

		// 트리거 정보 확인
		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil && trigger.HasVarTrigger("counter") {
			triggeredVars = append(triggeredVars, "counter")

			if !counterHV.IsError() && counterHV.Value != nil {
				t.Logf("✅ Counter triggered: value=%d", counterHV.Value.(int))
			}

			// 3번 트리거되면 종료
			if len(triggeredVars) >= 3 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "WatchCallTest")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(1 * time.Second)
	watcher.Stop()

	// 검증: counter 트리거가 최소 3번 감지되어야 함
	if len(triggeredVars) < 3 {
		t.Errorf("Expected at least 3 counter triggers, got %d", len(triggeredVars))
	}
}

// Test 2: WatchTick trigger detection
func TestTriggeredSignal_WatchTick(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	tickTriggered := 0

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		// WatchTick: 200ms 간격
		tick := hutil.WatchTick("ticker", 200*time.Millisecond, runCtx)

		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil && trigger.HasVarTrigger("ticker") {
			tickTriggered++

			if !tick.IsZero() {
				t.Logf("✅ Ticker triggered: tick#%d at %s",
					tick.TickCount, tick.Time.Format("15:04:05"))
			}

			if tickTriggered >= 3 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "WatchTickTest")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(1 * time.Second)
	watcher.Stop()

	if tickTriggered < 3 {
		t.Errorf("Expected at least 3 tick triggers, got %d", tickTriggered)
	}
}

// Test 3: WatchFlow trigger detection
func TestTriggeredSignal_WatchFlow(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	// 테스트용 채널
	priceChan := make(chan shared.FlowValue, 10)
	flowTriggered := 0

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		// WatchFlow: 채널에서 가격 스트림 감시
		priceHV := hersh.WatchFlow(func(ctx context.Context) (<-chan shared.FlowValue, error) {
			return priceChan, nil
		}, "price", runCtx)

		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil && trigger.HasVarTrigger("price") {
			flowTriggered++

			if !priceHV.IsError() && priceHV.Value != nil {
				t.Logf("✅ Price triggered: value=%.2f", priceHV.Value.(float64))
			}

			if flowTriggered >= 3 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "WatchFlowTest")

	// 채널에 값 주입
	go func() {
		time.Sleep(500 * time.Millisecond)
		priceChan <- shared.FlowValue{V: 100.5, E: nil}
		time.Sleep(200 * time.Millisecond)
		priceChan <- shared.FlowValue{V: 101.2, E: nil}
		time.Sleep(200 * time.Millisecond)
		priceChan <- shared.FlowValue{V: 99.8, E: nil}
	}()

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(2 * time.Second)

	watcher.Stop()

	if flowTriggered < 2 {
		t.Errorf("Expected at least 2 flow triggers, got %d", flowTriggered)
	}
}

// Test 4: UserSig trigger detection
func TestTriggeredSignal_UserMessage(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	userTriggered := 0
	receivedMessages := []string{}

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		trigger := runCtx.GetTriggeredSignal()

		if trigger != nil && trigger.IsUserSig {
			userTriggered++

			if msg != nil {
				receivedMessages = append(receivedMessages, msg.String())
				t.Logf("✅ User message triggered: '%s'", msg.String())
			}

			if userTriggered >= 3 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "UserMessageTest")

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// 메시지 전송
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("Hello")
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("World")
	time.Sleep(100 * time.Millisecond)
	watcher.SendMessage("Test")

	time.Sleep(500 * time.Millisecond)
	watcher.Stop()

	if userTriggered < 3 {
		t.Errorf("Expected 3 user triggers, got %d", userTriggered)
	}

	if len(receivedMessages) < 3 {
		t.Errorf("Expected 3 messages, got %d", len(receivedMessages))
	}
}

// Test 5: Mixed triggers integration test
func TestTriggeredSignal_Mixed(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 5 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	priceChan := make(chan shared.FlowValue, 10)
	triggerLog := []string{} // "user", "price", "counter", "ticker" 등

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		// 1. WatchFlow: 가격
		priceHV := hersh.WatchFlow(func(ctx context.Context) (<-chan shared.FlowValue, error) {
			return priceChan, nil
		}, "price", runCtx)

		// 2. WatchTick: 타이머
		tick := hutil.WatchTick("ticker", 300*time.Millisecond, runCtx)

		// 3. WatchCall: 카운터
		counterHV := hersh.WatchCall(func() (manager.VarUpdateFunc, error) {
			return func(prev shared.HershValue) (shared.HershValue, bool, error) {
				val := 0
				if prev.Value != nil {
					val = prev.Value.(int)
				}
				return shared.HershValue{Value: val + 1}, true, nil
			}, nil
		}, "counter", 250*time.Millisecond, runCtx)

		// 트리거 감지
		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil {
			if trigger.IsUserSig {
				triggerLog = append(triggerLog, "user")
				t.Logf("✅ User triggered: '%s'", msg.String())
			}

			if trigger.HasVarTrigger("price") && !priceHV.IsError() && priceHV.Value != nil {
				triggerLog = append(triggerLog, "price")
				t.Logf("✅ Price triggered: %.2f", priceHV.Value)
			}

			if trigger.HasVarTrigger("counter") && !counterHV.IsError() && counterHV.Value != nil {
				triggerLog = append(triggerLog, "counter")
				t.Logf("✅ Counter triggered: %d", counterHV.Value)
			}

			if trigger.HasVarTrigger("ticker") && !tick.IsZero() {
				triggerLog = append(triggerLog, "ticker")
				t.Logf("✅ Ticker triggered: tick#%d", tick.TickCount)
			}

			// 총 10개 이상 트리거되면 종료
			if len(triggerLog) >= 10 {
				return shared.NewStopErr("test complete")
			}
		}

		return nil
	}, "MixedTest")

	// 이벤트 주입
	go func() {
		time.Sleep(200 * time.Millisecond)
		priceChan <- shared.FlowValue{V: 100.5, E: nil}
		time.Sleep(300 * time.Millisecond)
		watcher.SendMessage("hello")
		time.Sleep(400 * time.Millisecond)
		priceChan <- shared.FlowValue{V: 101.2, E: nil}
		time.Sleep(300 * time.Millisecond)
		watcher.SendMessage("world")
	}()

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	time.Sleep(3 * time.Second)
	watcher.Stop()

	// 검증: 다양한 트리거가 감지되었는지
	hasUser := false
	hasPrice := false
	hasCounter := false
	hasTicker := false

	for _, log := range triggerLog {
		switch log {
		case "user":
			hasUser = true
		case "price":
			hasPrice = true
		case "counter":
			hasCounter = true
		case "ticker":
			hasTicker = true
		}
	}

	if !hasUser {
		t.Error("Expected user trigger")
	}
	if !hasPrice {
		t.Error("Expected price trigger")
	}
	if !hasCounter {
		t.Error("Expected counter trigger")
	}
	if !hasTicker {
		t.Error("Expected ticker trigger")
	}

	t.Logf("📊 Trigger log (%d entries): %v", len(triggerLog), triggerLog)
}

// Test 6: Batch VarSig trigger detection
func TestTriggeredSignal_BatchVarSigs(t *testing.T) {
	config := shared.DefaultWatcherConfig()
	config.ServerPort = 0
	config.DefaultTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	watcher := hersh.NewWatcher(config, nil, ctx)

	ch1 := make(chan shared.FlowValue, 10)
	ch2 := make(chan shared.FlowValue, 10)
	ch3 := make(chan shared.FlowValue, 10)

	batchDetected := false

	watcher.Manage(func(msg *shared.Message, runCtx shared.HershContext) error {
		time.Sleep(100 * time.Microsecond)
		// 3개 WatchFlow 동시 등록
		hersh.WatchFlow(func(ctx context.Context) (<-chan shared.FlowValue, error) {
			return ch1, nil
		}, "var1", runCtx)
		hersh.WatchFlow(func(ctx context.Context) (<-chan shared.FlowValue, error) {
			return ch2, nil
		}, "var2", runCtx)
		hersh.WatchFlow(func(ctx context.Context) (<-chan shared.FlowValue, error) {
			return ch3, nil
		}, "var3", runCtx)

		trigger := runCtx.GetTriggeredSignal()
		if trigger != nil && len(trigger.VarSigNames) >= 2 {
			batchDetected = true
			t.Logf("✅ Batch trigger detected: %v", trigger.VarSigNames)
			return shared.NewStopErr("batch detected")
		}

		return nil
	}, "BatchTest")

	ch1 <- shared.FlowValue{V: 1, E: nil}
	ch2 <- shared.FlowValue{V: 2, E: nil}
	ch3 <- shared.FlowValue{V: 3, E: nil}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	// 동시에 여러 채널에 값 주입 (배치 처리 유도)
	time.Sleep(200 * time.Millisecond)
	ch1 <- shared.FlowValue{V: 1, E: nil}
	ch2 <- shared.FlowValue{V: 2, E: nil}
	ch3 <- shared.FlowValue{V: 3, E: nil}
	time.Sleep(500 * time.Millisecond)
	ch1 <- shared.FlowValue{V: 1, E: nil}
	ch2 <- shared.FlowValue{V: 2, E: nil}
	ch3 <- shared.FlowValue{V: 3, E: nil}
	watcher.Stop()

	if !batchDetected {
		t.Error("Expected batch VarSig detection")
	}
}
