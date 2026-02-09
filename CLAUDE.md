
### Component Structure

```
hersh/
├── watcher.go         # Main entry point - Watcher orchestrates the framework
├── watch.go           # WatchCall/WatchFlow - reactive variable monitoring
├── memo.go            # Memoization for expensive computations
├── types.go           # Type re-exports from shared package
├── manager/           # Core reactive engine
│   ├── manager.go     # Manager orchestrates Reducer-Effect pattern
│   ├── reducer.go     # Pure state transitions (Signals → State changes)
│   ├── effect.go      # Effect definitions (UserRun, VarUpdate, etc.)
│   ├── effect_handler.go  # Effect execution (runs ManagedFunc)
│   ├── state.go       # ManagerState with VarState and lifecycle
│   └── signal.go      # Signal types (WatcherSig, UserSig, VarSig)
├── hctx/              # HershContext implementation
│   └── context.go     # Runtime context for managed functions
└── shared/            # Shared types and interfaces
    ├── types.go       # Core interfaces (HershContext, Signal, etc.)
    ├── errors.go      # Control flow errors (StopError, KillError, etc.)
    └── copy.go        # Deep copy utilities
```

### Key Concepts

#### 1. Watcher
The main orchestrator that:
- Creates and manages the Manager
- Provides the public API (`Manage`, `Start`, `Stop`, `SendMessage`)
- Handles Watch variable registration
- Coordinates lifecycle (initialization, running, cleanup)

#### 2. Manager (Reducer-Effect Engine)
Coordinates three core components synchronously:
- **Reducer**: Processes signals and performs state transitions
- **EffectCommander**: Maps state changes to effects (synchronous)
- **EffectHandler**: Executes effects and runs the ManagedFunc (synchronous)

Signal flow: `Signal → Reducer → Commander → Handler → (WatcherSig?) → loop`

#### 3. Signals (Priority-Based)
Three signal types with priority ordering:
- **WatcherSig** (Priority 0): State control (InitRun, Stop, Kill, Recover)
- **UserSig** (Priority 1): User messages triggering ManagedFunc execution
- **VarSig** (Priority 2): Watch variable updates triggering reactive execution

#### 4. State Machine
States: `Ready → InitRun → Running → Ready` (normal flow)
Terminal states: `Stopped`, `Killed`, `Crashed`
Recovery: `Running → WaitRecover → Ready` (on failure)

#### 5. Watch Variables (Reactive Primitives)
- **WatchCall**: Tick-based polling with computation functions
  - Returns `VarUpdateFunc` that computes next state from previous state
  - State-dependent execution (sequential processing)
- **WatchFlow**: Channel-based event streaming
  - Pushes values directly from channels
  - State-independent execution (uses last value only)

#### 6. ManagedFunc
User-defined function that:
- Receives `Message` and `HershContext`
- Can use Watch, Memo, and context values
- Returns control flow errors (`StopError`, `KillError`, `CrashError`)
- Triggers re-execution when Watch variables change

#### 7. Effects
Effects represent side effects that occur after state transitions:
- **UserRun**: Execute ManagedFunc with user message
- **VarUpdate**: Apply VarUpdateFunc to update watched variable
- **StateTransition**: Handle state lifecycle changes
- **Recovery**: Implement fault tolerance with exponential backoff

### Initialization Flow

1. User creates `Watcher` with config
2. User calls `Manage(fn, name)` to register ManagedFunc
3. User calls `Start()` which:
   - Starts Manager components (Reducer loop)
   - Sends `InitRun` signal
   - Executes ManagedFunc for first time (registers Watch variables)
   - Waits for all Watch variables to initialize
   - Transitions to `Ready` state
4. Framework enters reactive loop

### Reactive Execution Flow

1. Watch variable detects change (tick or channel event)
2. Watch loop sends `VarSig` to Manager
3. Reducer processes `VarSig` (priority-based)
4. Reducer calls EffectCommander to generate effects
5. EffectCommander returns `VarUpdate` effect
6. Reducer calls EffectHandler to execute effect synchronously
7. EffectHandler applies `VarUpdateFunc`, updates state
8. If value changed, EffectHandler generates `UserRun` effect
9. EffectHandler executes ManagedFunc with updated context
10. ManagedFunc reads new Watch values, performs logic
11. Cycle repeats on next Watch change

### Error Handling & Fault Tolerance

**Control Flow Errors**: Special errors for state control
- `StopError`: Gracefully stop (can be recovered)
- `KillError`: Terminate immediately (permanent)
- `CrashError`: Unrecoverable error (permanent)

**Recovery Policy**: Configurable fault tolerance
- **Suppression Phase** (failures < MinConsecutiveFailures): Exponential backoff with `SuppressDelay`
- **Recovery Phase** (failures ≥ MinConsecutiveFailures): Enter `WaitRecover` state, retry with `BaseRetryDelay`
- **Crash** (failures ≥ MaxConsecutiveFailures): Enter `Crashed` state (permanent)

**Deterministic Cleanup**: No timeouts
- Uses channels (`cleanupDone`, `WaitStopped`) for synchronization
- Guaranteed cleanup completion before shutdown
