# Changelog

本仓库自 v0.9.0 起独立维护，不再跟随 [fastclaw-ai/weclaw](https://github.com/fastclaw-ai/weclaw)。更早的版本见上游。

## v0.9.5 — 2026-10-01

### 修复
- 同一聊天的上一条还没回完时再发消息，两条都会报 `agent returned empty response`：两个请求共用一个会话的回复通道，后一条覆盖了前一条，前一条结束时又把它删掉。现在同一会话的消息排队依次处理，每条各自拿到回复
- 同一聊天的几条消息同时到达会各建一个会话；首个会话还在创建时发 `/new`，随后的消息仍落进旧会话。现在同一聊天的建会话与 `/new` 串行执行
- 空回复报错附带 `stopReason`，便于排查
- Agent 回复里写 `[旺柴]` 这类微信表情代码，微信只显示成文字。会话说明里告诉 Agent 改用 Unicode emoji

### 新增
- `/new <消息>`（及 `/clear <消息>`）：新建会话后直接把消息发给新会话。此前整句会原样发给旧会话

### 文档
- 公开发布到 GitHub（Tespera/weclaw）：README 改为英文（`README.md`）+ 中文（`README_CN.md`），新增与上游的差异对照和迁移步骤

## v0.9.4 — 2026-09-30

### 修复
- ACP Agent 的 `system_prompt` 配置此前被读取但从未使用，现在追加到会话系统提示

### 新增
- 每个 ACP 会话自动附带微信会话说明（用户微信 ID、用 `weclaw send --media` 发本地文件），Agent 不再临时起 HTTP 服务来发文件

## v0.9.3 — 2026-09-30

### 新增
- `weclaw send --media` 支持本地路径（任意文件类型；`~`、相对路径、`file://`），可重复传多个；发送前先校验全部媒体，避免只发出一半
- HTTP `/api/send` 新增 `media` 字段（网址或本地绝对路径），`media_url` 保留兼容；本地路径仅接受来自本机的请求

## v0.9.2 — 2026-09-30

### 修复
- `/cwd` 切换后当前会话仍在旧目录工作（会话的工作目录创建时即固定），现在切换后立即为该用户新建会话
- `/cwd` 只作用于已启动的 Agent，之后按需启动的 Agent 仍用旧目录；现在对所有已配置的 Agent 生效
- 不带参数的 `/cwd` 只回"(check agent config)"，现在显示实际工作区

### 新增
- `/cwd` 写回配置文件（只改对应 Agent 的 `cwd` 字段，保留其余内容），重启后不丢

## v0.9.1 — 2026-09-30

### 修复
- `/new` 回复里的 Agent 名显示成了 ACP 适配器的命令路径（如 `/opt/homebrew/bin/claude-agent-acp`），现在显示配置名（如 `claude`）

### 新增
- `/new` 回复和 `/info` 显示当前工作区（主目录缩写为 `~`）

## v0.9.0 — 2026-09-30

基于上游 v0.7.1。

### 从上游合入
- Codex app-server 的 `/new` 正确重置会话（上游 `main` 12b2086）
- 微信图片消息下载后转发给支持图片的 ACP Agent（上游 `dev` 140b1ed）
- 切换 Agent 时不再覆盖外部修改过的配置（上游 `dev` fd558f8，#38）

未合入上游 `dev` 的 Web 配置界面（需要 Next.js 构建链）和 shell Agent 类型。

### 修复
- ACP 自动审批按协议选择 `allow_once` / `allow_always`。此前只认 `kind == "allow"`，配合官方 `@agentclientprotocol/claude-agent-acp` 时所有需要审批的工具都会被拒绝（`Permission option was not offered: allow`）
- 图片消息回复与附件回复的函数签名不一致导致无法编译

### 新增
- ACP Agent 的 `model` 配置生效：新会话通过标准的 `session/set_config_option` 设置模型（此前只对 Codex 生效）
- 自动探测到的 Claude（ACP/CLI）默认模型由 `sonnet` 改为 `opus`
- ACP Agent 新增 `mode` 配置（如 `bypassPermissions`，等价于 `claude --dangerously-skip-permissions`），与 `model` 同样通过 `session/set_config_option` 应用到新会话
- 退出时停止 Agent 子进程；首次运行之后不再为未安装的 Agent 逐个启动登录 shell 探测（此前每次启动要多花约 10 秒）
- 单实例锁：`~/.weclaw/weclaw.lock` 上的 `flock`，防止两个 bridge 抢同一个微信消息队列
- `weclaw service install|uninstall`：生成并管理 launchd LaunchAgent（PATH 取自安装时的 shell，剔除临时目录、不存在的目录和重复项）；注册后 `start/stop/restart/status` 走 `launchctl`
- `weclaw update` 改为从本地源码仓库重新编译安装（有远程时先 `git pull --ff-only`）
- `make install` 默认安装到 `~/.local/bin`，以改名方式替换二进制

### 移除
- 下载上游发布包的 `update` 实现、`install.sh`、GitHub Actions 发布流程、`service/` 下的静态模板（其 plist/systemd 配置未带 `--foreground`，会反复重启）
