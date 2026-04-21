# Claude TMUX Driver 设计文档

## 概述

为 claude agent 实现 tmux driver，参照现有的 codex tmux driver 实现。该 driver 通过 tmux session 运行 claude-code CLI，并通过 SQLite 数据库跟踪运行状态。

## 架构设计

### 核心组件

**TMUXDriver**
- 实现 `agent.Driver` 接口
- 注册名称：`"claude-tmux"`
- 负责初始化 TMUXRuntime 实例
- 包含工厂依赖：`tmuxRuntimeFactory` 和 `tmuxRunStoreFactory`

**TMUXRuntime**
- 实现 `agent.SessionRuntime` 接口
- 管理单个 tmux session 的生命周期
- 维护运行时状态：starting、ready、running、broken
- 使用互斥锁保护并发访问：`mu`（状态锁）、`runMu`（运行锁）
- 字段：
  - `state`: 当前运行时状态
  - `pane`: tmux pane 接口，用于发送命令和捕获输出
  - `session`: tmux session 接口，用于终止 session
  - `readErr`: 记录错误信息
  - `waitGap`: 轮询间隔（默认 10ms）
  - `spec`: agent 规格配置
  - `runStore`: run 状态存储接口

**接口定义**

```go
type tmuxPane interface {
    SendKeys(keys ...string) error
    CapturePane() (string, error)
}

type tmuxSession interface {
    Kill() error
}

type tmuxRuntimeFactory interface {
    Start(ctx context.Context, spec agent.Spec, sessionName string) (tmuxSession, tmuxPane, error)
}

type tmuxRunStore interface {
    CreatePending(ctx context.Context, runID, botName, runtimeType string) error
    UpsertDone(ctx context.Context, runID, botName, runtimeType string) error
    GetByRunID(ctx context.Context, runID string) (tmuxRunRecord, error)
}

type tmuxRunStoreFactory interface {
    Open(spec agent.Spec) (tmuxRunStore, error)
}
```

**具体实现**

- `tmuxGotmuxFactory`: 使用 gotmux 库创建和管理 tmux session
- `tmuxGotmuxSession`: 包装 `*gotmux.Session`
- `tmuxGotmuxPane`: 包装 `*gotmux.Pane`
- `sqliteTMUXRunStoreFactory`: 创建 SQLite 支持的 run store
- `sqliteTMUXRunStore`: 使用 `repositories.AgentRunRepository` 存储 run 状态

### 与 Codex Driver 的差异

| 项目 | Codex | Claude |
|------|-------|--------|
| Driver 名称 | `codex-tmux` | `claude-tmux` |
| CLI 命令 | `codex` | `claude-code` |
| Notify 命令 | `myclaw notify codex <botname>` | `myclaw notify claude <botname>` |
| Session 名称 | `myclaw-codex-<botname>` | `myclaw-claude-<botname>` |
| Runtime Type | `codex` | `claude` |

## 数据流程

### 初始化流程 (Init)

1. **参数验证**
   - 检查 `spec.Command` 非空
   - 检查 `spec.WorkDir` 非空
   - 检查 `spec.SQLitePath` 非空（由 run store factory 验证）

2. **创建 Runtime 实例**
   - 初始化 `TMUXRuntime`，状态设为 `stateStarting`
   - 设置 `waitGap` 为 10ms

3. **启动 TMUX Session**
   - 调用 `tmuxRuntimeFactory.Start()`
   - Session 名称：`nextTMUXSessionName(spec.BotName)` → `myclaw-claude-<botname>`
   - 如果 session 已存在则复用，否则创建新 session
   - 获取 window 0 的第一个 pane

4. **初始化 Run Store**
   - 调用 `tmuxRunStoreFactory.Open(spec)`
   - 打开 SQLite 数据库并执行迁移

5. **等待就绪**
   - 调用 `waitUntilReady(ctx)`
   - 轮询 pane 输出，直到有内容出现
   - 状态转换：`stateStarting` → `stateReady`

6. **返回 Runtime**
   - 如果任何步骤失败，标记为 `stateBroken` 并返回错误

### 运行流程 (Run)

1. **前置检查**
   - 获取 `runMu` 锁（确保同一时间只有一个 run）
   - 验证 prompt 非空
   - 检查状态不是 `stateBroken`
   - 检查状态是 `stateReady` 或 `stateStarting`
   - 状态转换：`stateReady` → `stateRunning`

2. **准备运行上下文**
   - 如果 ctx 没有 deadline，设置默认超时（`defaultRunTimeout`）
   - 生成唯一 runID：`domain.NewPrefixedID("run")`

3. **记录 Run ID**
   - 调用 `writeTMUXCurrentRunID(workDir, runID)`
   - 写入文件：`<workDir>/.myclaw-run-id`
   - 内容：`<runID>\n`

4. **创建 Pending Run**
   - 调用 `runStore.CreatePending(ctx, runID, botName, "claude")`
   - 在数据库中创建状态为 "pending" 的 run 记录

5. **发送命令**
   - 调用 `pane.SendKeys(promptText, "C-m")`
   - `"C-m"` 表示 Enter 键

6. **等待完成**
   - 调用 `waitRunCompletion(ctx, runID)`
   - 轮询数据库，检查 run 状态
   - 每次轮询间隔 `waitGap`（10ms）
   - 直到状态变为 "done" 或超时
   - 状态由 claude-code CLI 通过 `myclaw notify claude <botname>` 命令更新

7. **捕获输出**
   - 调用 `pane.CapturePane()`
   - 规范化输出：`normalizeTMUXOutput()` - 替换 `\r\n` 为 `\n`
   - 清理输出：`cleanupTMUXRunText()` - 移除空行和尾部 `\r`

8. **返回结果**
   - 状态转换：`stateRunning` → `stateReady`
   - 返回 `agent.Response{Text, RuntimeType: "claude", ExitCode: 0, RawOutput}`

9. **错误处理**
   - 任何步骤失败都调用 `markBroken(err)`
   - 状态转换为 `stateBroken`
   - 记录错误到 `readErr`

### 关闭流程 (Close)

1. **获取锁并清理**
   - 获取 `mu` 锁
   - 保存 session 引用
   - 清空 `session` 和 `pane` 字段
   - 状态转换为 `stateBroken`
   - 设置 `readErr` 为 "runtime is closed"

2. **终止 Session**
   - 如果 session 不为 nil，调用 `session.Kill()`
   - 返回 kill 操作的错误（如果有）

## Shell 命令构建

### Session 启动命令

```go
func buildTMUXShellCommand(spec agent.Spec) string {
    command := spec.Command  // "claude-code"
    notifyConfig := fmt.Sprintf(`notify=["myclaw", "notify", "claude", %s]`, strconv.Quote(spec.BotName))
    return command + " -c " + shellQuote(notifyConfig)
}
```

**示例输出：**
```bash
claude-code -c 'notify=["myclaw", "notify", "claude", "my-bot"]'
```

这个配置告诉 claude-code CLI 在每次运行完成后调用 `myclaw notify claude my-bot` 命令。

### Session 选项

```go
func buildTMUXSessionOptions(spec agent.Spec, sessionName string) *gotmux.SessionOptions {
    return &gotmux.SessionOptions{
        Name:           sessionName,           // "myclaw-claude-<botname>"
        ShellCommand:   buildTMUXShellCommand(spec),
        StartDirectory: spec.WorkDir,
    }
}
```

## 状态管理

### 状态定义

```go
type runtimeState string

const (
    stateStarting runtimeState = "starting"  // 初始化中
    stateReady    runtimeState = "ready"     // 就绪，可以接受请求
    stateRunning  runtimeState = "running"   // 正在执行请求
    stateBroken   runtimeState = "broken"    // 损坏，无法继续使用
)
```

### 状态转换

```
Init: nil → stateStarting → stateReady
Run:  stateReady → stateRunning → stateReady
Error: any → stateBroken
Close: any → stateBroken
```

### 并发控制

- `mu`: 保护 runtime 状态字段（state, pane, session, readErr）
- `runMu`: 确保同一时间只有一个 Run 操作执行

## 错误处理

### 错误类型

1. **初始化错误**
   - 参数验证失败
   - TMUX session 创建失败
   - SQLite 数据库打开失败
   - 等待就绪超时

2. **运行时错误**
   - Prompt 为空
   - Runtime 状态不正确（broken 或非 ready）
   - SendKeys 失败
   - Run 状态记录失败
   - 等待完成超时
   - CapturePane 失败

3. **关闭错误**
   - Session kill 失败（非致命）

### 错误处理策略

- 所有运行时错误都调用 `markBroken(err)`
- Broken 状态下的 runtime 无法恢复，必须重新初始化
- 错误信息存储在 `readErr` 字段中
- `currentError()` 方法返回当前错误或默认错误消息

## 测试策略

### 单元测试

参照 `driver_tmux_test.go`，使用 mock 接口：

1. **Mock Pane**
   - 模拟 SendKeys 和 CapturePane 行为
   - 可配置返回值和错误

2. **Mock Session**
   - 模拟 Kill 行为

3. **Mock Run Store**
   - 模拟数据库操作
   - 可控制 run 状态转换时机

### 测试场景

1. 成功初始化和运行
2. 参数验证失败
3. TMUX session 创建失败
4. SendKeys 失败
5. 等待完成超时
6. CapturePane 失败
7. 并发 Run 调用
8. Close 后的操作

## 实现清单

- [ ] 创建 `internal/agent/claude/driver_tmux.go`
- [ ] 实现 `TMUXDriver` 和 `TMUXRuntime`
- [ ] 实现 gotmux 工厂和适配器
- [ ] 实现 SQLite run store 工厂和适配器
- [ ] 实现辅助函数（session 命名、命令构建、输出清理）
- [ ] 注册 driver：`agent.MustRegisterDriver("claude-tmux", ...)`
- [ ] 创建 `internal/agent/claude/driver_tmux_test.go`
- [ ] 编写单元测试
- [ ] 验证与 claude-code CLI 的集成

## 依赖项

- `github.com/GianlucaP106/gotmux/gotmux`: TMUX 操作库
- `github.com/benenen/myclaw/internal/agent`: Agent 接口定义
- `github.com/benenen/myclaw/internal/domain`: Domain 类型（ID 生成、错误）
- `github.com/benenen/myclaw/internal/store`: SQLite 数据库
- `github.com/benenen/myclaw/internal/store/repositories`: AgentRunRepository

## 配置要求

使用此 driver 需要在 agent spec 中提供：

- `Command`: 必须为 `"claude-code"`（或其他 claude CLI 路径）
- `WorkDir`: 工作目录，用于存储 `.myclaw-run-id` 文件
- `SQLitePath`: SQLite 数据库路径
- `BotName`: Bot 名称，用于 session 命名和 notify 命令

## 与 Claude-Code CLI 的集成

Claude-code CLI 需要支持：

1. **Notify 配置**
   - 接受 `-c 'notify=["myclaw", "notify", "claude", "<botname>"]'` 参数
   - 在每次运行完成后执行 notify 命令

2. **Run ID 文件**
   - 读取 `<workdir>/.myclaw-run-id` 文件获取当前 runID
   - 在 notify 命令中传递 runID

3. **Notify 命令格式**
   ```bash
   myclaw notify claude <botname>
   ```
   - 该命令会调用 `runStore.UpsertDone(ctx, runID, botName, "claude")`
   - 将 run 状态从 "pending" 更新为 "done"
