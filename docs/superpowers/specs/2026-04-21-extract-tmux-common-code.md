# 提取共通 TMUX 代码设计文档

## 概述

将 `internal/agent/codex/driver_tmux.go` 和 `internal/agent/claude/driver_tmux.go` 中完全相同的代码提取到 `internal/tmux` 包中，减少代码重复，提高可维护性。

## 目标

- 提取 gotmux 适配器代码（完全相同）
- 提取工具函数（完全相同）
- 保留 agent 特定的业务逻辑在各自的 driver 中
- 最小化改动，避免过度抽象

## 架构设计

### 新包结构

创建 `internal/tmux` 包，包含一个文件：

**`internal/tmux/adapters.go`**
- Gotmux 适配器实现
- 工具函数
- 基础接口定义

### 提取的组件

#### 1. 接口定义

```go
// Pane represents a tmux pane interface
type Pane interface {
    SendKeys(keys ...string) error
    CapturePane() (string, error)
}

// Session represents a tmux session interface
type Session interface {
    Kill() error
}
```

这些接口供适配器实现使用，driver 中仍保留自己的 `tmuxPane` 和 `tmuxSession` 接口定义（因为它们是 driver 内部使用的）。

#### 2. Gotmux 适配器

**GotmuxSession**
- 包装 `*gotmux.Session`
- 实现 `Session` 接口
- 提供 `Kill()` 方法

**GotmuxPane**
- 包装 `*gotmux.Pane`
- 实现 `Pane` 接口
- 提供 `SendKeys()` 和 `CapturePane()` 方法

**GotmuxFactory**
- 空结构体
- 提供 `Start(ctx, spec, sessionName)` 方法创建 tmux session
- 返回 `Session` 和 `Pane` 接口

#### 3. 工具函数

**NormalizeTMUXOutput(text string) string**
- 将 `\r\n` 替换为 `\n`
- 统一行结束符

**CleanupTMUXRunText(text string) string**
- 移除空行
- 移除尾部的 `\r` 字符
- 清理 tmux 输出文本

**ShellQuote(text string) string**
- Shell 单引号转义
- 使用 `'\''` 模式处理单引号
- 空字符串返回 `''`

### 保留在各 driver 中的代码

以下代码保留在 `codex/driver_tmux.go` 和 `claude/driver_tmux.go` 中：

**结构体和类型：**
- `TMUXDriver` - driver 主结构
- `TMUXRuntime` - runtime 状态管理
- `runtimeState` - 状态枚举
- `tmuxPane`, `tmuxSession` 接口（driver 内部使用）
- `tmuxRuntimeFactory`, `tmuxRunStore`, `tmuxRunStoreFactory` 接口
- `tmuxRunRecord` 结构体
- `sqliteTMUXRunStore` 和 `sqliteTMUXRunStoreFactory`

**方法：**
- `NewTMUXDriver()` - 构造函数
- `Init()` - 初始化方法（包含 agent 特定的错误消息）
- `Run()` - 运行方法（包含 agent 特定的 runtime type）
- `Close()` - 关闭方法
- `waitUntilReady()` - 等待就绪
- `waitRunCompletion()` - 等待完成
- `markBroken()` - 标记损坏
- `currentError()` - 获取当前错误

**Agent 特定的工具函数：**
- `nextTMUXSessionName()` - 生成 session 名称（包含 "codex" 或 "claude" 前缀）
- `buildTMUXShellCommand()` - 构建 shell 命令（包含 agent 特定的 notify 命令）
- `buildTMUXSessionOptions()` - 构建 session 选项
- `writeTMUXCurrentRunID()` - 写入 run ID 文件

**SQLite 实现：**
- `sqliteTMUXRunStore` 结构体和方法
- `sqliteTMUXRunStoreFactory` 实现

## 代码迁移策略

### 步骤 1：创建 internal/tmux 包

创建 `internal/tmux/adapters.go`，包含：
1. 包声明和导入
2. 接口定义（`Pane`, `Session`）
3. Gotmux 适配器结构体（`GotmuxSession`, `GotmuxPane`, `GotmuxFactory`）
4. 适配器方法实现
5. 工具函数（`NormalizeTMUXOutput`, `CleanupTMUXRunText`, `ShellQuote`）

### 步骤 2：修改 codex driver

在 `internal/agent/codex/driver_tmux.go` 中：
1. 添加 `import "github.com/benenen/myclaw/internal/tmux"`
2. 删除 gotmux 适配器代码（`tmuxGotmuxSession`, `tmuxGotmuxPane`, `tmuxGotmuxFactory` 及其方法）
3. 删除工具函数（`normalizeTMUXOutput`, `cleanupTMUXRunText`, `shellQuote`）
4. 更新引用：
   - `tmuxGotmuxFactory{}` → `tmux.GotmuxFactory{}`
   - `normalizeTMUXOutput()` → `tmux.NormalizeTMUXOutput()`
   - `cleanupTMUXRunText()` → `tmux.CleanupTMUXRunText()`
   - `shellQuote()` → `tmux.ShellQuote()`

### 步骤 3：修改 claude driver

在 `internal/agent/claude/driver_tmux.go` 中执行与步骤 2 相同的操作。

### 步骤 4：验证

1. 运行测试：`go test ./...`
2. 验证编译：`go build ./cmd/server`
3. 确保所有测试通过

## 接口兼容性

### Driver 内部接口 vs 共通接口

Driver 中的 `tmuxPane` 和 `tmuxSession` 接口与 `internal/tmux` 包中的 `Pane` 和 `Session` 接口是兼容的：

```go
// Driver 内部接口
type tmuxPane interface {
    SendKeys(keys ...string) error
    CapturePane() (string, error)
}

// tmux 包接口
type Pane interface {
    SendKeys(keys ...string) error
    CapturePane() (string, error)
}
```

`tmux.GotmuxPane` 实现了 `tmux.Pane` 接口，同时也满足 driver 的 `tmuxPane` 接口要求。Go 的结构化类型系统允许这种隐式兼容。

### 类型转换

在 driver 中使用时：

```go
// tmuxGotmuxFactory 返回 tmux.Session 和 tmux.Pane
session, pane, err := tmux.GotmuxFactory{}.Start(ctx, spec, sessionName)

// 可以直接赋值给 driver 的接口类型
runtime.session = session  // tmuxSession 接口
runtime.pane = pane        // tmuxPane 接口
```

不需要显式类型转换，因为接口方法签名完全匹配。

## 依赖关系

**新的依赖：**
- `internal/agent/codex` → `internal/tmux`
- `internal/agent/claude` → `internal/tmux`

**internal/tmux 的依赖：**
- `github.com/GianlucaP106/gotmux/gotmux`
- `github.com/benenen/myclaw/internal/agent` (用于 `agent.Spec` 类型)
- 标准库：`context`, `fmt`, `strings`

## 测试策略

### 现有测试

codex 和 claude 的现有测试不需要修改，因为：
1. 它们使用 mock 接口，不依赖具体实现
2. 接口签名保持不变
3. 行为保持不变

### 新测试（可选）

可以为 `internal/tmux` 包添加单元测试：
- `TestNormalizeTMUXOutput` - 测试行结束符规范化
- `TestCleanupTMUXRunText` - 测试文本清理
- `TestShellQuote` - 测试 shell 引号转义
- `TestGotmuxAdapters` - 测试适配器（需要 mock gotmux）

但这不是必需的，因为这些函数已经在 driver 测试中被间接测试。

## 代码量变化

**减少的重复代码：**
- Gotmux 适配器：~80 行 × 2 = 160 行
- 工具函数：~30 行 × 2 = 60 行
- 总计：~220 行重复代码被消除

**新增代码：**
- `internal/tmux/adapters.go`：~120 行（包含注释和接口定义）

**净减少：**
- ~100 行代码

## 向后兼容性

此重构不影响：
- 外部 API（driver 注册名称保持不变）
- Driver 行为（功能完全相同）
- 测试（所有现有测试应该通过）

## 未来扩展

如果将来需要添加更多 agent（如 gemini, gpt），可以：
1. 直接使用 `internal/tmux` 包中的适配器和工具函数
2. 在新 agent 的 driver 中实现 agent 特定的逻辑
3. 遵循相同的模式，保持代码一致性

## 实施清单

- [ ] 创建 `internal/tmux/adapters.go`
- [ ] 实现接口定义
- [ ] 实现 Gotmux 适配器
- [ ] 实现工具函数
- [ ] 修改 `codex/driver_tmux.go`
- [ ] 修改 `claude/driver_tmux.go`
- [ ] 运行测试验证
- [ ] 提交更改

## 风险和缓解

**风险 1：接口不兼容**
- 缓解：仔细验证接口签名完全匹配
- 验证：编译时会捕获不兼容问题

**风险 2：行为变化**
- 缓解：提取的代码是逐字复制，不做修改
- 验证：运行完整测试套件

**风险 3：导入循环**
- 缓解：`internal/tmux` 只依赖 `internal/agent`（类型定义），不依赖具体 driver
- 验证：编译时会检测循环依赖
