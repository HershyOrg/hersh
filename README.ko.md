# Hersh

**관리 실행 및 모니터링을 위한 Go 리액티브 프레임워크**

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.21-blue)](https://go.dev/)

Hersh는 관리 실행(Managed Execution) 모델을 통해 결정론적 상태 관리를 제공하는 경량 리액티브 프레임워크입니다.

## 주요 기능

- 🎯 **Managed Execution**: 메시지 또는 리액티브 변경에 의해 트리거되는 단일 함수
- 🔄 **WatchCall**: 폴링 기반 리액티브 변수 (tick 기반 변경 감지)
- 📡 **WatchFlow**: 채널 기반 리액티브 변수 (이벤트 스트림)
- 💾 **Memo**: 고비용 계산을 위한 세션 범위 캐싱
- 📦 **HershContext**: 원자적 업데이트를 지원하는 영속 키-값 저장소
- 🌐 **WatcherAPI**: 외부 제어 및 모니터링용 HTTP 서버
- 🛡️ **Fault Tolerance**: 지수 백오프 기반 내장 복구 정책
- 📊 **Execution Logging**: 상태 전이, 오류, 컨텍스트 변경 추적

## 빠른 시작

### 설치

```bash
go get github.com/HershyOrg/hersh@v0.2.0
```

### 전체 예제

아래 예제는 핵심 기능을 한 곳에서 모두 보여줍니다.

```go
package main

import (
    "fmt"
    "time"
    "github.com/HershyOrg/hersh"
    "github.com/HershyOrg/hersh/manager"
)

func main() {
    // 1. 환경 변수를 포함해 Watcher 생성
    config := hersh.DefaultWatcherConfig()
    watcher := hersh.NewWatcher(config, map[string]string{
        "API_KEY": "secret",
    }, nil)

    // 외부 데이터 소스
    externalCounter := 0
    eventChan := make(chan any, 10)

    // 모든 기능을 사용하는 관리 함수
    watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
        // 2. WatchCall: 폴링 기반 리액티브 (500ms마다 확인)
        counter := hersh.WatchCall(
            func() (manager.VarUpdateFunc, error) {
                // 함수 본문에서는 네트워크 호출 등 가능
                time.Sleep(20*time.Millisecond)
                // return 함수 내부에는 가능한 외부 효과 없는 계산만 사용
                return func(prev any) (any, bool, error) {
                    current := externalCounter
                    externalCounter++
                    if prev == nil {
                        return current, true, nil
                    }
                    return current, prev.(int) != current, nil
                }, nil
            },
            "counter", 500*time.Millisecond, ctx,
        )

        // 3. WatchFlow: 채널 기반 리액티브
        event := hersh.WatchFlow(eventChan, "events", ctx)

        // 4. Memo: 계산 결과 캐싱 (세션당 1회 실행)
        apiClient := hersh.Memo(func() any {
            fmt.Println("Initializing API client...")
            return &struct{ name string }{name: "client"}
        }, "apiClient", ctx)

        // 5. HershContext: 원자적 업데이트가 가능한 영속 상태
        ctx.UpdateValue("totalRuns", func(current any) any {
            if current == nil {
                return 1
            }
            return current.(int) + 1
        })
        totalRuns := ctx.GetValue("totalRuns")

        // 6. 환경 변수 (불변)
        apiKey, _ := ctx.GetEnv("API_KEY")

        fmt.Printf("Execution: counter=%v, event=%v, client=%v, runs=%v, key=%s\n",
            counter, event, apiClient, totalRuns, apiKey)

        // 7. 메시지 처리와 오류 제어
        if msg != nil && msg.Content == "stop" {
            return hersh.NewStopErr("user requested stop")
        }

        return nil
    }, "app").Cleanup(func(ctx hersh.HershContext) {
        fmt.Println("Cleanup executed")
    })

    // Watcher 시작 (Ready 상태까지 블로킹)
    watcher.Start()

    // 8. 실행 트리거 메시지 전송
    watcher.SendMessage("hello")
    time.Sleep(100 * time.Millisecond)

    // WatchFlow 트리거 이벤트 전송
    eventChan <- "event1"
    time.Sleep(100 * time.Millisecond)

    // 9. Logger: 실행 이력 확인
    watcher.GetLogger().PrintSummary()

    // 10. 정상 종료
    watcher.SendMessage("stop")
    time.Sleep(100 * time.Millisecond)
    watcher.Stop()
}
```

**출력**:

``` txt
Initializing API client...
Execution: counter=0, event=<nil>, client=&{client}, runs=1, key=secret
Execution: counter=1, event=<nil>, client=&{client}, runs=2, key=secret
Execution: counter=1, event=event1, client=&{client}, runs=3, key=secret

=== Logger Summary ===
Reduce Log Entries: 12
Effect Log Entries: 0
Effect Results: 8
Watch Error Log Entries: 0
Context Value Changes: 5

Cleanup executed
```

## 핵심 개념

### Managed Function

Hersh는 아래 시점에 실행되는 **단일 관리 함수**를 사용합니다.

1. **시작 시**: 초기 실행 (`InitRun` 상태)
2. **SendMessage 시**: `SendMessage(content)` 또는 API `/message` 호출 시
3. **WatchCall 시**: 폴링한 값이 변경될 때 (`tick` 간격마다 확인)
4. **WatchFlow 시**: 채널에 새 값이 들어올 때

상태 흐름: `NotRun → InitRun → Ready → Running → Ready → ...`

### 리액티브 변수

#### WatchCall (폴링)

- 고정 간격(`tick`)으로 외부 값을 폴링
- `prev`를 입력으로 `(next, changed, error)`를 계산하는 `VarUpdateFunc` 반환
- `changed = true`일 때만 관리 함수를 재실행
- 첫 값이 준비되기 전까지 `nil` 반환

#### WatchFlow (채널)

- 채널에서 새로운 값을 모니터링
- 계산 없이 값을 직접 반영
- 채널 이벤트마다 관리 함수를 재실행
- 첫 값 수신 전까지 `nil` 반환

### Memo vs HershContext

| 기능 | Memo | HershContext |
|---------|------|--------------|
| **목적** | 고비용 계산 캐시 | 영속 상태 저장 |
| **수명** | 세션(`ClearMemo` 또는 재시작 전까지) | 세션(모든 실행에 걸쳐 유지) |
| **재실행 트리거** | 재실행을 트리거하지 않음 | 재실행을 트리거하지 않음 |
| **스레드 안전성** | ✅ `LoadOrStore` 시맨틱 | ✅ Mutex 보호 |
| **사용 사례** | DB 연결, HTTP 클라이언트 | 카운터, 통계, 플래그 |

### 오류 처리

특수 오류를 반환해 Watcher 라이프사이클을 제어합니다.

```go
// 정상 중지 (cleanup 실행, 복구 불가)
return hersh.NewStopErr("user stop")

// 강제 종료 (cleanup 없음, 즉시 종료)
return hersh.NewKillErr("critical error")

// 크래시 + 복구 시도 (cleanup 실행, 설정에 따라 복구 가능)
return hersh.NewCrashErr("recoverable error")

// 일반 오류 (로그만 남기고 계속 실행)
return fmt.Errorf("non-fatal error")
```

### 상태 라이프사이클

```
NotRun → InitRun → Ready ⇄ Running
                    ↓
                Stopped/Killed (영구)
                    ↓
                Crashed → WaitRecover → Ready (또는 영구 Crashed)
```

종단 상태: `Stopped`, `Killed`, `Crashed` (최대 재시도 초과 후)

## 전체 API 레퍼런스

### Watcher 메서드

| 메서드 | 설명 |
|--------|-------------|
| `NewWatcher(config, envVars, parentCtx)` | 설정 및 환경 변수로 Watcher 생성 |
| `Manage(fn, name)` | 관리 함수 등록, `CleanupBuilder` 반환 |
| `.Cleanup(cleanupFn)` | 정리 함수 등록 (`Stop/Kill/Crash` 시 호출) |
| `Start()` | Watcher 시작 (`Ready` 또는 오류까지 블로킹) |
| `Stop()` | cleanup 포함 정상 종료 (완료까지 블로킹) |
| `SendMessage(content)` | 관리 함수 실행 트리거 메시지 전송 |
| `GetState()` | 현재 상태 조회 (`Ready`, `Running`, `Stopped` 등) |
| `GetLogger()` | 실행 로그 객체 접근 |
| `StartAPIServer()` | HTTP API 서버 시작 (기본 포트 8080) |

### Logger 메서드

| 메서드 | 반환값 | 설명 |
|--------|---------|-------------|
| `PrintSummary()` | - | 실행 요약을 stdout에 출력 |
| `GetReduceLog()` | `[]ReduceLogEntry` | 상태 전이 로그(처리된 시그널) |
| `GetEffectLog()` | `[]EffectLogEntry` | 이펙트 실행 로그(사용자 메시지) |
| `GetWatchErrorLog()` | `[]WatchErrorLogEntry` | Watch 변수 오류 로그(계산 실패) |
| `GetContextLog()` | `[]ContextValueLogEntry` | 컨텍스트 값 변경 로그 (`SetValue`, `UpdateValue`) |
| `GetStateTransitionFaultLog()` | `[]StateTransitionFaultLogEntry` | 잘못된 상태 전이 로그(오류) |
| `GetRecentResults(count)` | `[]*EffectResult` | 최근 N개 이펙트 실행 결과 |

**예시**:
```go
logger := watcher.GetLogger()
logger.PrintSummary()

// 특정 로그 조회
contextChanges := logger.GetContextLog()
watchErrors := logger.GetWatchErrorLog()
```

### 리액티브 함수

| 함수 | 설명 |
|----------|-------------|
| `WatchCall(getComputationFunc, varName, tick, ctx)` | 폴링 기반 리액티브 (현재 값 또는 `nil` 반환) |
| `WatchFlow(sourceChan, varName, ctx)` | 채널 기반 리액티브 (최신 값 또는 `nil` 반환) |
| `Memo(computeValue, memoName, ctx)` | 세션 범위 캐시 (계산된 값 반환) |
| `ClearMemo(memoName, ctx)` | 캐시된 값 제거 (다음 호출 시 재계산) |

**WatchCall 시그니처**:
```go
func WatchCall(
    getComputationFunc func() (manager.VarUpdateFunc, error),
    varName string,
    tick time.Duration,
    ctx HershContext,
) any
```

**VarUpdateFunc 시그니처**:
```go
type VarUpdateFunc func(prev any) (next any, changed bool, err error)
```

### HershContext 인터페이스

| 메서드 | 설명 |
|--------|-------------|
| `WatcherID()` | Watcher 고유 식별자 |
| `Message()` | 현재 사용자 메시지 (`nil` 가능) |
| `GetValue(key)` | 저장된 값 조회 (복사본이 아닌 실제 값 반환) |
| `SetValue(key, value)` | 값 저장 (단순 할당) |
| `UpdateValue(key, updateFn)` | **깊은 복사** 기반 원자적 업데이트 (스레드 안전) |
| `GetEnv(key)` | 불변 환경 변수 조회 (Watcher 생성 시 설정) |
| `GetWatcher()` | Watcher 참조 반환 (`any` 타입) |

**UpdateValue 예시** (원자적, 스레드 안전):
```go
// updateFn은 현재 값의 깊은 복사본을 받음
ctx.UpdateValue("stats", func(current any) any {
    if current == nil {
        return map[string]int{"count": 1}
    }
    stats := current.(map[string]int)
    stats["count"]++
    return stats
})
```

### 오류 생성자

| 생성자 | 라이프사이클 | Cleanup? | Recovery? |
|-------------|-----------|----------|-----------|
| `NewStopErr(reason)` | Stopped (영구) | ✅ 예 | ❌ 아니오 |
| `NewKillErr(reason)` | Killed (영구) | ❌ 아니오 | ❌ 아니오 |
| `NewCrashErr(reason)` | Crashed → WaitRecover | ✅ 예 | ✅ 예 (설정 시) |

### 설정 타입

#### WatcherConfig

```go
type WatcherConfig struct {
    DefaultTimeout     time.Duration  // 관리 함수 타임아웃 (기본값: 1분)
    RecoveryPolicy     RecoveryPolicy // 장애 허용 정책
    ServerPort         int            // API 서버 포트 (기본값: 8080)
    MaxLogEntries      int            // 로그 버퍼 크기 (기본값: 50,000)
    MaxWatches         int            // 동시 watch 최대 개수 (기본값: 1,000)
    MaxMemoEntries     int            // memo 캐시 최대 개수 (기본값: 1,000)
    SignalChanCapacity int            // 시그널 큐 크기 (기본값: 50,000)
}

func DefaultWatcherConfig() WatcherConfig
```

#### RecoveryPolicy

```go
type RecoveryPolicy struct {
    MinConsecutiveFailures int           // WaitRecover 진입 전 실패 횟수 (기본값: 3)
    MaxConsecutiveFailures int           // 영구 Crashed 전 실패 횟수 (기본값: 6)
    BaseRetryDelay         time.Duration // 초기 재시도 지연 (기본값: 5초)
    MaxRetryDelay          time.Duration // 재시도 지연 최대치 (기본값: 5분)
    LightweightRetryDelays []time.Duration // 실패 <3에서 사용하는 지연 (기본값: [15초, 30초, 60초])
}

func DefaultRecoveryPolicy() RecoveryPolicy
```

**동작 방식**:
- **실패 < 3**: 경량 재시도 지연 `[15s, 30s, 60s]` 후 `Ready`
- **실패 ≥ 3**: 지수 백오프 기반 중량 재시도 (`5s → 10s → 20s → ...`) 후 `WaitRecover`
- **실패 ≥ 6**: 영구 `Crashed` 상태 (더 이상 재시도 없음)

**예시**:
```go
config := hersh.WatcherConfig{
    DefaultTimeout: 30 * time.Second,
    RecoveryPolicy: hersh.RecoveryPolicy{
        MinConsecutiveFailures: 2,
        MaxConsecutiveFailures: 5,
        BaseRetryDelay:         3 * time.Second,
        MaxRetryDelay:          1 * time.Minute,
    },
}
```

## WatcherAPI (HTTP 엔드포인트)

### 제어 및 상태

```bash
# 현재 상태 조회 (Ready, Running, Stopped, Killed, Crashed, WaitRecover)
GET /watcher/status

# 상세 상태 조회 (실행 횟수, 오류 횟수, 업타임)
GET /watcher/state

# 관리 함수 실행 트리거 메시지 전송
POST /watcher/message
Content-Type: application/json
{"content": "your-command"}

# Watcher 설정 조회
GET /watcher/config
```

### 모니터링

```bash
# 환경 변수
GET /watcher/vars

# Watch 변수 (WatchCall/WatchFlow 현재 값)
GET /watcher/watching

# Memo 캐시 내용
GET /watcher/memoCache

# HershContext 변수 상태 (GetValue/SetValue)
GET /watcher/varState
```

### 로그

```bash
# 상태 전이 로그 (Reducer 액션)
GET /watcher/logs/reduce

# 이펙트 실행 로그 (관리 함수 실행)
GET /watcher/logs/effect

# Watch 오류 로그 (계산 실패)
GET /watcher/logs/watch-error

# 컨텍스트 값 변경 로그 (SetValue/UpdateValue)
GET /watcher/logs/context

# 상태 전이 오류 로그 (잘못된 전이)
GET /watcher/logs/state-fault
```

**예시**:
```bash
# API 서버 시작 (관리 함수 내부 또는 Start 이전)
watcher.StartAPIServer()

# 외부 프로세스에서 조회
curl http://localhost:8080/watcher/status
# {"status": "Ready"}

curl http://localhost:8080/watcher/logs/context
# [{"logID": 1, "key": "totalRuns", "newValue": 5, ...}]
```

## 예제

### 예제 1: 복구 정책 데모

지수 백오프를 이용한 자동 복구를 보여줍니다.

```go
config := hersh.DefaultWatcherConfig()
config.RecoveryPolicy = hersh.RecoveryPolicy{
    MinConsecutiveFailures: 2,
    MaxConsecutiveFailures: 4,
    BaseRetryDelay:         1 * time.Second,
    LightweightRetryDelays: []time.Duration{500 * time.Millisecond, 1 * time.Second},
}

watcher := hersh.NewWatcher(config, nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    failCount := ctx.GetValue("failCount")
    if failCount == nil {
        failCount = 0
    }
    count := failCount.(int)

    fmt.Printf("Execution attempt %d (state: %s)\n", count+1, watcher.GetState())

    if count < 5 {
        ctx.SetValue("failCount", count+1)
        return hersh.NewCrashErr(fmt.Sprintf("simulated failure %d", count+1))
    }

    fmt.Println("Success after recovery!")
    return nil
}, "recovery-demo")

watcher.Start()
time.Sleep(15 * time.Second) // 재시도 대기
watcher.GetLogger().PrintSummary()
watcher.Stop()
```

**출력**:
```
Execution attempt 1 (state: InitRun)
Execution attempt 2 (state: Ready)      # 500ms 지연 (경량)
Execution attempt 3 (state: Ready)      # 1s 지연 (경량)
Execution attempt 4 (state: WaitRecover) # 1s 지연 (중량)
Execution attempt 5 (state: WaitRecover) # 2s 지연 (지수)
Execution attempt 6 (state: WaitRecover) # 4s 지연 (지수)
Success after recovery!

=== Logger Summary ===
State Transition Fault Entries: 5
```

### 예제 2: 실시간 이벤트 파이프라인

WatchFlow 기반 처리 파이프라인을 보여줍니다.

```go
eventChan := make(chan any, 100)
watcher := hersh.NewWatcher(hersh.DefaultWatcherConfig(), nil, nil)

watcher.Manage(func(msg *hersh.Message, ctx hersh.HershContext) error {
    // 들어오는 이벤트 감시
    event := hersh.WatchFlow(eventChan, "eventStream", ctx)

    if event != nil {
        // 이벤트 처리
        processed := fmt.Sprintf("processed_%v", event)

        // 컨텍스트에 저장
        ctx.SetValue("lastProcessed", processed)

        // 통계 원자적 업데이트
        ctx.UpdateValue("stats", func(current any) any {
            if current == nil {
                return map[string]int{"total": 1}
            }
            stats := current.(map[string]int)
            stats["total"]++
            return stats
        })

        stats := ctx.GetValue("stats").(map[string]int)
        fmt.Printf("Event: %v → %s (total: %d)\n", event, processed, stats["total"])
    }

    // 제어 메시지 처리
    if msg != nil && msg.Content == "status" {
        stats := ctx.GetValue("stats")
        fmt.Printf("Pipeline stats: %+v\n", stats)
    }

    return nil
}, "pipeline")

watcher.Start()

// Producer goroutine
go func() {
    for i := 1; i <= 5; i++ {
        eventChan <- fmt.Sprintf("event%d", i)
        time.Sleep(100 * time.Millisecond)
    }
}()

time.Sleep(600 * time.Millisecond)
watcher.SendMessage("status")
time.Sleep(100 * time.Millisecond)
watcher.Stop()
```

**출력**:
```
Event: event1 → processed_event1 (total: 1)
Event: event2 → processed_event2 (total: 2)
Event: event3 → processed_event3 (total: 3)
Event: event4 → processed_event4 (total: 4)
Event: event5 → processed_event5 (total: 5)
Pipeline stats: map[total:5]
```

## 아키텍처

### 상태 머신

```
NotRun → InitRun → Ready ⇄ Running
                    ↓
                Stopped/Killed (영구)
                    ↓
                Crashed → WaitRecover → Ready
                                  ↓
                                Crashed (영구, MaxConsecutiveFailures 이후)
```

**상태 설명**:
- **NotRun**: `Start()` 호출 전
- **InitRun**: 첫 실행 (초기화 단계)
- **Ready**: 유휴 상태, 트리거(메시지/리액티브 변경) 대기
- **Running**: 관리 함수 실행 중
- **Stopped**: `StopErr` 또는 `Stop()`으로 정상 종료 (영구)
- **Killed**: `KillErr`로 강제 종료 (영구)
- **Crashed**: 최대 재시도 후 복구 불가 오류 (영구)
- **WaitRecover**: 크래시 후 재시도 대기 (임시)

### 시그널 우선순위

내부 시그널 처리 순서 (숫자가 낮을수록 우선순위 높음):

```
Priority 0: WatcherSig (라이프사이클: InitRun, Stop, Kill, Recover)
Priority 1: UserSig     (SendMessage, API /message)
Priority 2: VarSig      (WatchCall/WatchFlow 리액티브 트리거)
```

이 순서를 통해 라이프사이클 명령이 사용자 메시지/리액티브 트리거보다 항상 우선 처리됩니다.

### 패키지 구조

```
github.com/HershyOrg/hersh/
├── watcher.go           # 핵심 Watcher API
├── watcher_api.go       # HTTP API 서버
├── watch.go             # WatchCall, WatchFlow
├── memo.go              # Memo 캐싱
├── types.go             # 공개 타입 (shared 재노출)
├── manager/             # 내부 Manager (Reducer-Effect 패턴)
│   ├── manager.go       # Manager 오케스트레이터
│   ├── reducer.go       # 순수 상태 전이
│   ├── effect_handler.go # 이펙트 실행
│   ├── logger.go        # 실행 로깅
│   └── signal.go        # 시그널 우선순위 큐
├── hctx/                # HershContext 구현
│   └── context.go
├── shared/              # 공통 타입 및 오류
│   ├── types.go
│   └── errors.go
├── api/                 # WatcherAPI HTTP 핸들러
└── demo/                # 사용 예제
    ├── example_simple.go
    ├── example_watchcall.go
    └── example_trading.go
```

## 실제 사용 사례

Hersh는 Docker 컨테이너를 리액티브 상태 관리 방식으로 제어하는 컨테이너 오케스트레이션 시스템 **[Hershy](https://github.com/HershyOrg/hershy)** 에서 사용됩니다.

**프로덕션 예제**:
- [simple-counter](https://github.com/HershyOrg/hershy/tree/main/examples/simple-counter): WatcherAPI 제어가 포함된 기본 카운터
- [trading-long](https://github.com/HershyOrg/hershy/tree/main/examples/trading-long): 실시간 트레이딩 시뮬레이터
- [watcher-server](https://github.com/HershyOrg/hershy/tree/main/examples/watcher-server): 영속 상태를 가진 HTTP 서버

## 링크

- **저장소**: https://github.com/HershyOrg/hersh
- **문서**: https://pkg.go.dev/github.com/HershyOrg/hersh
- **이슈**: https://github.com/HershyOrg/hersh/issues
- **Hershy (컨테이너 오케스트레이션)**: https://github.com/HershyOrg/hershy
