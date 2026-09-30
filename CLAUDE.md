# weclaw（独立维护版）

微信 ClawBot/iLink → AI Agent（Claude via ACP、Codex app-server 等）的桥接器。Go 单二进制。
fork 自 fastclaw-ai/weclaw v0.7.1，**不再跟随上游**，不要加回 upstream remote，也不要提议"同步上游"。

## 构建与发布

- `make build` / `make test` / `make install`（装到 `~/.local/bin/weclaw`，并重启运行中的实例）。
- 版本号来自 `git describe`，经 ldflags 注入 `weclaw/cmd.Version`；`weclaw/cmd.SourceDir` 记录源码目录供 `weclaw update` 使用。不要手写版本字符串。
- 发版：更新 CHANGELOG.md → 提交 → `git tag vX.Y.Z` → `make install`。
- 安装必须"写临时文件再 mv"。macOS 上原地覆盖正在运行（或运行过）的二进制可能导致进程被 SIGKILL。

## 运行时约定

- 本机以 launchd 服务运行：`com.weclaw.bridge`，plist 由 `weclaw service install` 生成，不要手改。
- 服务模式下只用 `weclaw start/stop/restart`（它们会走 launchctl），不要直接 `kill` 或另起 `weclaw start -f`，单实例锁会拒绝第二个实例。
- 配置 `~/.weclaw/config.json`；Claude 走 ACP（`claude-agent-acp`，npm `@agentclientprotocol/claude-agent-acp`），模型由 `model` 字段决定，不要再用 `ANTHROPIC_MODEL` 环境变量。
- ACP 模式自动放行所有工具权限（用户在电脑上也常开 bypass，这是预期行为）。

## 测试

- `go test ./...` 全绿是提交前提。
- `WECLAW_ACP_E2E=1 go test ./agent -run TestACPModelE2E -v` 会真实调用 claude-agent-acp，并在 Claude 会话记录中核对模型。改动 ACP 会话或模型逻辑后要跑。
- launchd 相关逻辑在 `cmd/service_darwin_test.go` 里用假的 launchctl 测试，不要在测试中调用真实 launchctl。
