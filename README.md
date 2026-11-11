<p align="center">
  <img src="docs/assets/banner.webp" alt="WeClaw: chat with Claude Code, Codex, Gemini CLI, Kimi, Cursor, OpenCode and OpenClaw from WeChat" width="100%" />
</p>

<h1 align="center">WeClaw: WeChat bridge for Claude Code, Codex and AI coding agents</h1>

<p align="center">
  <a href="https://github.com/Tespera/weclaw/releases"><img src="https://img.shields.io/github/v/tag/Tespera/weclaw?label=release&color=07c160&sort=semver" alt="Latest release" /></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/Tespera/weclaw?color=00add8" alt="Go version" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/Tespera/weclaw?color=blue" alt="MIT license" /></a>
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey" alt="Platform: macOS and Linux" />
  <img src="https://img.shields.io/badge/protocol-ACP-8a2be2" alt="Agent Client Protocol" />
</p>

<p align="center">
  <b>English</b> · <a href="README_CN.md">简体中文</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#faq">FAQ</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

**WeClaw** is an open-source bridge that connects **WeChat (微信)** to the AI coding agents on your own computer. Send a WeChat message from your phone and **Claude Code**, **OpenAI Codex**, **Gemini CLI**, **Kimi**, **Cursor**, **OpenCode** or **OpenClaw** answers it, edits code, runs commands and sends files back. It is a single Go binary built on the WeChat ClawBot (iLink) protocol. Claude runs through the [Agent Client Protocol](https://agentclientprotocol.com/) (ACP) via [`claude-agent-acp`](https://www.npmjs.com/package/@agentclientprotocol/claude-agent-acp).

> [!NOTE]
> **This is the actively maintained fork of [fastclaw-ai/weclaw](https://github.com/fastclaw-ai/weclaw).** Upstream has had no commits since April 2026. This repository continues from upstream v0.7.1 with bug fixes and new features. Config and login data (`~/.weclaw`) are compatible, so it is a drop-in replacement. See [what this fork fixes](#what-this-fork-fixes).

## Highlights

- **Every major coding agent in one chat.** Claude Code, Codex, Gemini CLI, Kimi, Cursor, OpenCode and OpenClaw, plus GitHub Copilot CLI, Qwen Code, Kiro, Factory Droid, iFlow and pi over ACP. Mention one with `/codex …`, or switch the default with a short alias like `/cc`.
- **Runs on your machine, no server needed.** WeClaw long-polls WeChat over outbound HTTPS. You don't need a public IP, port forwarding or a cloud VM. Your code stays on your disk.
- **Real sessions, not one-shot prompts.** Each chat keeps its own agent session. Messages sent while a reply is still running are queued, not dropped. `/new` starts fresh and `/cwd` switches project.
- **Files both ways.** Send images, PDFs and videos to the agent. It sends screenshots, reports and any local file back with `weclaw send --media`.
- **Push notifications from your agents.** Long-running jobs can message you when they finish, from the `weclaw send` CLI or a local HTTP API.
- **Built to stay up.** macOS login service (launchd), single-instance lock, crash restart, and replies that a restart cut off are flagged so you can resend.
- **Plain-language errors.** Network drops, usage limits (with the reset time), expired logins and so on are explained in plain Chinese instead of raw stack traces.

<p align="center">
  <img src="docs/assets/screenshots.webp" alt="WeClaw screenshots: asking Claude, Gemini, Codex, Kimi and OpenClaw in one WeChat chat and switching agents with /cc, /gm, /cx aliases" width="100%" />
</p>

## Quick Start

Requires Go 1.25+. Used and tested mainly on macOS. Linux builds and runs, but you manage it with systemd yourself. Windows builds but is untested.

```bash
git clone https://github.com/Tespera/weclaw.git
cd weclaw

# Build and install to ~/.local/bin (override with PREFIX); make sure it is on PATH
make install

# First start shows a WeChat QR code to log in, then runs in the background
weclaw start

# Recommended on macOS: register as a login service (start at login, restart on crash)
weclaw service install
```

Later, `weclaw update` pulls the latest code and reinstalls.

On first start, WeClaw will:

1. Show a QR code. Scan it with WeChat to log in.
2. Auto-detect installed AI agents (Claude, Codex, Gemini and so on).
3. Save config to `~/.weclaw/config.json`.
4. Start receiving and replying to WeChat messages.

Use `weclaw login` to add more WeChat accounts.

For Claude, ACP mode is recommended: `npm i -g @agentclientprotocol/claude-agent-acp`. See [ACP session options](#acp-session-options-and-environment).

### Migrating from fastclaw-ai/weclaw

Config and login data live in `~/.weclaw` and are compatible, so you don't need to scan the QR code again.

1. Stop the old version: `weclaw stop`
2. Remove the old binary so it does not shadow the new one on PATH. Upstream `install.sh` installs to `/usr/local/bin/weclaw`, and `go install` to `$(go env GOPATH)/bin/weclaw`.
3. Install as in Quick Start and run `weclaw start`.
4. For Claude, prefer ACP: `npm i -g @agentclientprotocol/claude-agent-acp`. See [ACP session options](#acp-session-options-and-environment) below.

## How It Works

<p align="center">
  <img src="docs/assets/architecture.webp" alt="WeClaw architecture: WeChat on your phone, WeChat ClawBot iLink relay, WeClaw long-polling over outbound HTTPS on your Mac or Linux machine, routing to ACP agents (Claude Code, Codex, Gemini CLI, Kimi, Cursor, OpenCode, OpenClaw), CLI agents (claude -p, codex exec) and HTTP agents" width="100%" />
</p>

You chat with the WeChat ClawBot. WeClaw, running on your computer, long-polls the ClawBot relay for new messages, hands each one to the right agent, and sends the reply back the same way. Each WeChat chat maps to its own agent session.

**Agent modes:**

| Mode | How it works | Agents |
| ---- | ------------ | ------ |
| ACP  | Long-running subprocess, JSON-RPC over stdio. Fastest, because it reuses the process and sessions. | Claude, Codex, Kimi, Gemini, Cursor, OpenCode, OpenClaw, Copilot, Qwen, Kiro, Droid, iFlow, pi |
| CLI  | Spawns a new process per message. Resumes sessions via `--resume`. | Claude (`claude -p`), Codex (`codex exec`) |
| HTTP | OpenAI-compatible Chat Completions API. | OpenClaw (HTTP fallback) |

When both ACP and CLI are available, ACP is chosen.

## What this fork fixes

Open upstream issues that are resolved here:

| Problem | Upstream issue | This fork |
| ------- | -------------- | --------- |
| Sending a message before the previous reply arrives interrupts it or fails with `agent returned empty response` | [#48](https://github.com/fastclaw-ai/weclaw/issues/48) | Messages to the same session are queued and each gets its own reply (v0.9.5, ACP mode) |
| `/new` with Codex fails with `unknown variant session/new` | [#60](https://github.com/fastclaw-ai/weclaw/issues/60) | Fixed (v0.9.0) |
| Cannot send local images or files back to WeChat | [#57](https://github.com/fastclaw-ai/weclaw/issues/57), [#34](https://github.com/fastclaw-ai/weclaw/issues/34) | `weclaw send --media <local path>` for any file type; the agent is told to use it (v0.9.3, v0.9.4) |
| Images and files sent from WeChat are ignored by the agent | [#26](https://github.com/fastclaw-ai/weclaw/issues/26) | Images, files and videos are saved locally and their paths handed to the agent with the next message (v0.9.9) |
| Switching the default agent overwrites newer config on disk | [#38](https://github.com/fastclaw-ai/weclaw/issues/38) | Only the `default_agent` field is written (v0.9.0) |

Also:

- **Tools work with the official `claude-agent-acp`.** Upstream only accepted a permission option of kind `allow`. The official adapter offers `allow_once` / `allow_always`, so every tool that needed approval was rejected.
- **`model` and `mode` apply to Claude.** They are set on each new session via the standard ACP `session/set_config_option`. `mode: bypassPermissions` matches `claude --dangerously-skip-permissions`.
- **Single-instance lock and a macOS login service.** `weclaw service install` registers a launchd agent (start at login, restart on crash). Two bridges can no longer fight over one message queue.
- **Restarts don't swallow replies.** If the bridge restarts while a reply is being generated (an update, `weclaw restart`, even one issued from WeChat), the chat is told afterwards to resend instead of never hearing back. `/new` and `/cwd` also close the old session's Claude Code process.
- **`/new <message>`** starts a new session and asks it right away. `/cwd` takes effect immediately and is saved to the config.
- Each session gets a short WeChat context, and `system_prompt` is appended to Claude Code's built-in system prompt instead of replacing it.

Full list in [CHANGELOG.md](CHANGELOG.md) (written in Chinese).

## Chat Commands

Send these as WeChat messages:

| Command | Description |
| ------- | ----------- |
| `hello` | Send to the default agent |
| `/codex write a sort function` | Send to a specific agent |
| `/cc explain this code` | Send via an alias |
| `/claude` | Switch the default agent to Claude |
| `/cwd /path/to/project` | Switch workspace: applies to all agents, saved to config, starts a new session |
| `/cwd` | Show the current workspace |
| `/new` | Start a new conversation (clear the session) |
| `/new look at this error` | Start a new conversation and send this message to it |
| `/info` | Show current agent info (including workspace) |
| `/help` | Show help |

### Aliases

| Alias | Agent |
| ----- | ----- |
| `/cc`  | Claude   |
| `/cx`  | Codex    |
| `/cs`  | Cursor   |
| `/km`  | Kimi     |
| `/gm`  | Gemini   |
| `/ocd` | OpenCode |
| `/oc`  | OpenClaw |

You can define custom triggers per agent in the config:

```json
{
  "agents": {
    "claude": {
      "type": "acp",
      "aliases": ["ai", "c"]
    }
  }
}
```

Then `/ai hello` or `/c hello` routes to claude.

Switching the default agent is saved to the config file and survives restarts.

## Media Messages

WeClaw sends and receives images, video, files and voice messages.

**Voice:** WeChat's own speech-to-text transcription is forwarded to the agent as text. Duplicate voice events are de-duplicated.

**Images, files and videos you send:** saved to `~/.weclaw/workspace/inbox/<date>/` and acknowledged right away. Their local paths are handed to the agent together with your next message, because on WeChat you usually send the file first and say what to do second. Up to 200 MB per file.

**Errors:** agent failures are replied in plain Chinese: what happened, whether it is on your side, and what to do next (for example, when a usage limit resets). The raw error goes to the log only.

**Images in agent replies:** when the agent returns markdown images (`![](url)`), WeClaw downloads them, uploads them to the WeChat CDN (AES-128-ECB encrypted) and sends them as image messages.

**Markdown:** replies are converted to plain text before sending. Code fences are removed, links are reduced to their text, bold/italic markers are stripped, and so on.

## Proactive Messages

Push messages to a WeChat user without waiting for them to write first.

**CLI:**

```bash
# Text
weclaw send --to "user_id@im.wechat" --text "Hello from weclaw"

# Image
weclaw send --to "user_id@im.wechat" --media "https://example.com/photo.png"

# Text + image
weclaw send --to "user_id@im.wechat" --text "Check this out" --media "https://example.com/photo.png"

# File
weclaw send --to "user_id@im.wechat" --media "https://example.com/report.pdf"

# Local files (any type; ~, relative paths and file:// work; --media is repeatable)
weclaw send --to "user_id@im.wechat" --media ~/Desktop/report.pdf --media ./chart.png
```

All `--media` values are validated first (each must exist and be a regular file). Nothing is sent unless all of them pass, so the recipient never gets half a message.

**HTTP API** (while `weclaw start` is running, default `127.0.0.1:18011`):

```bash
# Text
curl -X POST http://127.0.0.1:18011/api/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user_id@im.wechat", "text": "Hello from weclaw"}'

# Image
curl -X POST http://127.0.0.1:18011/api/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user_id@im.wechat", "media": "https://example.com/photo.png"}'

# Local file (absolute path required)
curl -X POST http://127.0.0.1:18011/api/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user_id@im.wechat", "media": "/Users/me/Desktop/report.pdf"}'

# Text + media
curl -X POST http://127.0.0.1:18011/api/send \
  -H "Content-Type: application/json" \
  -d '{"to": "user_id@im.wechat", "text": "Check this out", "media": "https://example.com/photo.png"}'
```

`media` takes a URL or an absolute local path (the old `media_url` field still works). Local paths are only accepted from loopback callers (127.0.0.1 / ::1), so binding the API to another address does not let remote callers read local files.

Media is sent by type: images (png, jpg, gif, webp, bmp) as images, video (mp4, mov, webm, mkv, avi) as video, and anything else as a file.

Set `WECLAW_API_ADDR` to change the listen address (e.g. `0.0.0.0:18011`).

> [!IMPORTANT]
> WeChat only delivers bot-initiated messages for a limited time after the user last wrote to the bot. In our testing that window is about 24 hours. Outside it, sends fail with `ret=-2 errmsg=prepare failed`. Send the bot any message to reopen the window.

## Configuration

Config file: `~/.weclaw/config.json`

```json
{
  "default_agent": "claude",
  "agents": {
    "claude": {
      "type": "acp",
      "command": "/usr/local/bin/claude-agent-acp",
      "env": {
        "ANTHROPIC_API_KEY": "sk-ant-xxx"
      },
      "model": "opus"
    },
    "codex": {
      "type": "acp",
      "command": "/usr/local/bin/codex-acp",
      "env": {
        "OPENAI_API_KEY": "sk-xxx"
      }
    },
    "openclaw": {
      "type": "http",
      "endpoint": "https://api.example.com/v1/chat/completions",
      "api_key": "sk-xxx",
      "model": "openclaw:main"
    }
  }
}
```

Environment variables:

- `WECLAW_DEFAULT_AGENT`: override the default agent
- `OPENCLAW_GATEWAY_URL`: OpenClaw HTTP fallback endpoint
- `OPENCLAW_GATEWAY_TOKEN`: OpenClaw API token

Per-agent environment variables are set with `env`:

```json
{
  "agents": {
    "claude": {
      "env": {
        "ENV_NAME": "ENV_VALUE"
      }
    }
  }
}
```

### Permissions (CLI agents)

Some CLI agents ask for interactive permission confirmation. That can't be answered from WeChat, so the request hangs. Skip it with `args`:

| Agent | Argument | Effect |
|-------|----------|--------|
| Claude (CLI) | `--dangerously-skip-permissions` | Skip all tool permission prompts |
| Codex (CLI) | `--skip-git-repo-check` | Allow running outside a git repository |

```json
{
  "claude": {
    "type": "cli",
    "command": "/usr/local/bin/claude",
    "cwd": "/home/user/my-project",
    "args": ["--dangerously-skip-permissions"]
  },
  "codex": {
    "type": "cli",
    "command": "/usr/local/bin/codex",
    "cwd": "/home/user/my-project",
    "args": ["--skip-git-repo-check"]
  }
}
```

`cwd` sets the agent's working directory (workspace). The default is `~/.weclaw/workspace`.

> [!WARNING]
> These arguments bypass safety checks. Enable them only if you understand the risk.

### ACP session options and environment

For ACP agents, `model` and `mode` are applied to each new session via the standard ACP `session/set_config_option`, and `env` is passed to the agent subprocess. For example, to make Claude in WeChat behave like `TZ=Europe/Oslo claude --dangerously-skip-permissions` in a terminal:

```json
{
  "claude": {
    "type": "acp",
    "command": "/opt/homebrew/bin/claude-agent-acp",
    "model": "opus",
    "mode": "bypassPermissions",
    "env": { "TZ": "Europe/Oslo" }
  }
}
```

The session workspace decides which project context loads: the project CLAUDE.md, `.mcp.json`, project skills and Claude's per-directory auto memory. To match a Claude started from some directory in your terminal, set `cwd` to that directory. Connectors (MCP) from your claude.ai account are off in SDK mode by default. Add `"ENABLE_CLAUDEAI_MCP_SERVERS": "true"` to `env` to enable them.

Each new session also gets a short WeChat context, followed by the configured `system_prompt`. The context covers the current user's WeChat ID, how to send files with `weclaw send --media <local path>`, and where received attachments are saved. Both are appended after the agent's built-in system prompt via `_meta.systemPrompt.append` on `session/new`, so they don't replace it.

Valid `mode` values depend on the agent (claude-agent-acp: `default`, `acceptEdits`, `plan`, `bypassPermissions`, …). Without `mode`, ACP mode still auto-approves every permission request. Run `weclaw restart` after changing the config.

## Running as a Service

```bash
weclaw start      # start (in the background; via launchd once the service is installed)
weclaw status     # show status
weclaw restart    # restart
weclaw stop       # stop
weclaw start -f   # run in the foreground (debugging)
```

Logs go to `~/.weclaw/weclaw.log`.

Only one bridge may run at a time. The process takes a `flock` on `~/.weclaw/weclaw.lock` and exits if it can't get it. The lock is released when the process exits, including crashes. Two instances would compete for the same WeChat message queue, so this is a hard rule.

### Login service (macOS, recommended)

```bash
weclaw service install     # writes ~/Library/LaunchAgents/com.weclaw.bridge.plist and starts it
weclaw service uninstall   # stops and removes it
```

Once installed, `start/stop/restart` go through `launchctl` instead of forking a background process. `stop` unloads the job, and it comes back at next login or `weclaw start`. The plist records the shell `PATH` at install time so node (for ACP adapters), claude, codex and friends can be found. If they move, run `weclaw service install` again.

Linux has no built-in service management yet. Run `weclaw start --foreground` under systemd. `--foreground` is required: without it the process forks to the background and exits, and systemd keeps restarting it.

## Docker

```bash
# Build
docker build -t weclaw .

# Log in (interactive, scan the QR code)
docker run -it -v ~/.weclaw:/root/.weclaw weclaw login

# Start with an HTTP agent
docker run -d --name weclaw \
  -v ~/.weclaw:/root/.weclaw \
  -e OPENCLAW_GATEWAY_URL=https://api.example.com \
  -e OPENCLAW_GATEWAY_TOKEN=sk-xxx \
  weclaw

# Logs
docker logs -f weclaw
```

> [!NOTE]
> ACP and CLI modes need the agent binaries inside the container. The default image contains only WeClaw, so mount the binaries or build a custom image. HTTP mode works out of the box.

## Updates and Versions

```bash
weclaw update     # rebuild and reinstall from the source checkout (git pull --ff-only first if it has a remote); restarts a running bridge
weclaw version    # show the version
```

`update` uses the source directory recorded at build time (written by `make install`) and is equivalent to running `make install` there. The binary is replaced by writing a temp file and renaming it, never overwritten in place.

Versions come from git tags (`git describe`). Release flow: update `CHANGELOG.md` → commit → `git tag vX.Y.Z` → `make install`.

## FAQ

### What is WeClaw?

WeClaw is a free, open-source (MIT) bridge between WeChat and AI coding agents. It runs on your own Mac or Linux machine and lets you use Claude Code, OpenAI Codex, Gemini CLI, Kimi, Cursor, OpenCode and OpenClaw by sending WeChat messages from your phone.

### How do I use Claude Code from WeChat?

1. Install Claude Code and the ACP adapter: `npm i -g @agentclientprotocol/claude-agent-acp`.
2. Install WeClaw (`make install`) and run `weclaw start`.
3. Scan the QR code with WeChat.
4. Message the WeChat ClawBot. If Claude is installed it is the default agent; otherwise send `/cc …` or `/claude` to switch.

### Do I need a server, public IP or port forwarding?

No. WeClaw only makes outbound HTTPS requests: it long-polls WeChat for new messages. It works behind NAT, on a laptop or on a home machine.

### Which AI agents are supported?

Claude Code (ACP or `claude -p`), OpenAI Codex (ACP, `codex app-server` or `codex exec`), Gemini CLI, Kimi CLI, Cursor, OpenCode and OpenClaw (ACP or an OpenAI-compatible HTTP endpoint). GitHub Copilot CLI, Qwen Code, Kiro CLI, Factory Droid, iFlow CLI and pi are also detected over ACP. Agents installed on the machine are detected automatically on first start, and Claude becomes the default if it is installed.

### Where does my data go?

Agents run locally with your own accounts and API keys. Your code never leaves the machine except through the agents' own API calls. Messages travel through WeChat's servers, as any WeChat message does. Login data and config are stored in `~/.weclaw`.

### Can the agent message me first, e.g. when a long task finishes?

Yes. Use `weclaw send` or the local HTTP API (see [Proactive Messages](#proactive-messages)). The WeChat context given to each session tells the agent how. WeChat only delivers bot-initiated messages for about 24 hours after you last wrote to the bot.

### How long does a WeChat chat session last?

A chat keeps the same agent session, with no idle timeout, until you send `/new` (or `/clear`), switch workspace with `/cwd`, or WeClaw restarts. After a restart the next message starts a fresh session. Previous Claude transcripts stay on disk.

### Is WeClaw an official Tencent, WeChat, Anthropic or OpenAI product?

No. WeClaw is an independent community project and is not affiliated with or endorsed by Tencent, Anthropic, OpenAI or Google.

### How is it different from fastclaw-ai/weclaw?

This fork is actively maintained, fixes several open upstream issues, and adds a launchd service, a single-instance lock, files in both directions, message queueing and plain-language errors. See [What this fork fixes](#what-this-fork-fixes). It reads the same `~/.weclaw` data, so switching takes a minute.

## Development

```bash
make dev      # hot reload
make build    # build to ./bin/weclaw
make test     # tests

# ACP end-to-end test (calls the real claude-agent-acp; skipped by default)
WECLAW_ACP_E2E=1 go test ./agent -run TestACPModelE2E -v
```

README images are rendered from HTML: edit `docs/assets/src/render.py` and run `python3 docs/assets/src/render.py`.

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Report security issues privately as described in [SECURITY.md](SECURITY.md).

## Credits

Based on the work of [fastclaw-ai/weclaw](https://github.com/fastclaw-ai/weclaw) and its [contributors](https://github.com/fastclaw-ai/weclaw/graphs/contributors). Inspired by [@tencent-weixin/openclaw-weixin](https://npmx.dev/package/@tencent-weixin/openclaw-weixin).

For personal learning only, not for commercial use.

## License

[MIT](LICENSE)
