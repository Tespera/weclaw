# WeClaw (maintained fork)

[中文文档](README_CN.md)

WeChat AI agent bridge: chat with Claude Code, Codex, Gemini, Kimi, Cursor, OpenCode and OpenClaw from WeChat. Built on the WeChat ClawBot (iLink) protocol, a single Go binary; Claude runs over ACP (`claude-agent-acp`).

> **This is an actively maintained fork of [fastclaw-ai/weclaw](https://github.com/fastclaw-ai/weclaw).** Upstream has had no commits since April 2026. This repository continues from upstream v0.7.1 with bug fixes and new features. Config and login data (`~/.weclaw`) are compatible with upstream, so it is a drop-in replacement. See [CHANGELOG.md](CHANGELOG.md).

> This project is inspired by [@tencent-weixin/openclaw-weixin](https://npmx.dev/package/@tencent-weixin/openclaw-weixin). For personal learning only, not for commercial use.

|                                                 |                                                 |                                                 |
| :---------------------------------------------: | :---------------------------------------------: | :---------------------------------------------: |
| <img src="previews/preview1.png" width="280" /> | <img src="previews/preview2.png" width="280" /> | <img src="previews/preview3.png" width="280" /> |

## What this fork fixes

Open upstream issues that are resolved here:

| Problem | Upstream issue | This fork |
| ------- | -------------- | --------- |
| Sending a message before the previous reply arrives interrupts it or fails with `agent returned empty response` | [#48](https://github.com/fastclaw-ai/weclaw/issues/48) | Messages to the same session are queued and each gets its own reply (v0.9.5, ACP mode) |
| `/new` with Codex fails with `unknown variant session/new` | [#60](https://github.com/fastclaw-ai/weclaw/issues/60) | Fixed (v0.9.0) |
| Cannot send local images or files back to WeChat | [#57](https://github.com/fastclaw-ai/weclaw/issues/57), [#34](https://github.com/fastclaw-ai/weclaw/issues/34) | `weclaw send --media <local path>` for any file type; the agent is told to use it (v0.9.3, v0.9.4) |
| Images sent from WeChat are ignored by the agent | [#26](https://github.com/fastclaw-ai/weclaw/issues/26) | Images are downloaded and forwarded to ACP agents that accept images (v0.9.0) |
| Switching the default agent overwrites newer config on disk | [#38](https://github.com/fastclaw-ai/weclaw/issues/38) | Only the `default_agent` field is written (v0.9.0) |

Also:

- **Tools work with the official `claude-agent-acp`.** Upstream only accepted a permission option of kind `allow`; the official adapter offers `allow_once` / `allow_always`, so every tool that needed approval was rejected.
- **`model` and `mode` apply to Claude.** They are set on each new session via the standard ACP `session/set_config_option`; `mode: bypassPermissions` matches `claude --dangerously-skip-permissions`.
- **Single-instance lock and a macOS login service.** `weclaw service install` registers a launchd agent (start at login, restart on crash); two bridges can no longer fight over one message queue.
- **`/new <message>`** starts a new session and asks it right away; `/cwd` takes effect immediately and is saved to the config.
- Each session gets a short WeChat context, and `system_prompt` is appended to Claude Code's built-in system prompt instead of replacing it.

Full list in [CHANGELOG.md](CHANGELOG.md) (written in Chinese).

## Quick Start

Requires Go 1.25+. Used and tested mainly on macOS; Linux builds and runs (manage it with systemd yourself); Windows builds but is untested.

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

1. Show a QR code — scan it with WeChat to log in
2. Auto-detect installed AI agents (Claude, Codex, Gemini, etc.)
3. Save config to `~/.weclaw/config.json`
4. Start receiving and replying to WeChat messages

Use `weclaw login` to add more WeChat accounts.

### Migrating from fastclaw-ai/weclaw

Config and login data live in `~/.weclaw` and are compatible; no need to scan the QR code again.

1. Stop the old version: `weclaw stop`
2. Remove the old binary so it does not shadow the new one on PATH: upstream `install.sh` installs to `/usr/local/bin/weclaw`, `go install` to `$(go env GOPATH)/bin/weclaw`
3. Install as in Quick Start and run `weclaw start`
4. For Claude, prefer ACP: `npm i -g @agentclientprotocol/claude-agent-acp`; see [ACP session options](#acp-session-options-and-environment) below

## How It Works

<p align="center">
  <img src="previews/architecture.png" width="600" />
</p>

**Agent modes:**

| Mode | How it works | Agents |
| ---- | ------------ | ------ |
| ACP  | Long-running subprocess, JSON-RPC over stdio. Fastest — reuses the process and sessions. | Claude, Codex, Kimi, Gemini, Cursor, OpenCode, OpenClaw |
| CLI  | Spawns a new process per message. Resumes sessions via `--resume`. | Claude (`claude -p`), Codex (`codex exec`) |
| HTTP | OpenAI-compatible Chat Completions API. | OpenClaw (HTTP fallback) |

When both ACP and CLI are available, ACP is chosen.

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

**Images in agent replies:** when the agent returns markdown images (`![](url)`), WeClaw downloads them, uploads to the WeChat CDN (AES-128-ECB encrypted) and sends them as image messages.

**Markdown:** replies are converted to plain text before sending — code fences removed, links reduced to their text, bold/italic markers stripped, and so on.

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

All `--media` values are validated first (exists and is a regular file); nothing is sent unless all pass, so you never get half a message.

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

`media` takes a URL or an absolute local path (the old `media_url` field still works). Local paths are only accepted from loopback (127.0.0.1 / ::1) callers, so binding the API to another address does not let remote callers read local files.

Media is sent by type: images (png, jpg, gif, webp, bmp) as images, video (mp4, mov, webm, mkv, avi) as video, anything else as a file.

Set `WECLAW_API_ADDR` to change the listen address (e.g. `0.0.0.0:18011`).

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

- `WECLAW_DEFAULT_AGENT` — override the default agent
- `OPENCLAW_GATEWAY_URL` — OpenClaw HTTP fallback endpoint
- `OPENCLAW_GATEWAY_TOKEN` — OpenClaw API token

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

Some CLI agents ask for interactive permission confirmation, which cannot be answered from WeChat and hangs the request. Skip it with `args`:

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

`cwd` sets the agent's working directory (workspace); default `~/.weclaw/workspace`.

> **Note:** these arguments bypass safety checks. Enable them only if you understand the risk.

### ACP session options and environment

For ACP agents, `model` and `mode` are applied to each new session via the standard ACP `session/set_config_option`; `env` is passed to the agent subprocess. For example, to make Claude in WeChat behave like `TZ=Europe/Oslo claude --dangerously-skip-permissions` in a terminal:

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

The session workspace decides which project context loads: the project CLAUDE.md, `.mcp.json`, project skills and Claude's per-directory auto memory. To match a Claude started from some directory in your terminal, set `cwd` to that directory. Connectors (MCP) from your claude.ai account are off in SDK mode by default; add `"ENABLE_CLAUDEAI_MCP_SERVERS": "true"` to `env` to enable them.

Each new session also gets a short WeChat context (the current user's WeChat ID and how to send files with `weclaw send --media <local path>`), followed by the configured `system_prompt`. Both are appended after the agent's built-in system prompt via `_meta.systemPrompt.append` on `session/new`; they do not replace it.

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

Only one bridge may run at a time: the process takes a `flock` on `~/.weclaw/weclaw.lock` and exits if it cannot (the lock is released when the process exits, including crashes). Two instances would compete for the same WeChat message queue, so this is a hard rule.

### Login service (macOS, recommended)

```bash
weclaw service install     # writes ~/Library/LaunchAgents/com.weclaw.bridge.plist and starts it
weclaw service uninstall   # stops and removes it
```

Once installed, `start/stop/restart` go through `launchctl` instead of forking a background process. `stop` unloads the job; it comes back at next login or `weclaw start`. The plist records the shell `PATH` at install time so node (for ACP adapters), claude, codex and friends can be found; if they move, run `weclaw service install` again.

Linux has no built-in service management yet. Run `weclaw start --foreground` under systemd (`--foreground` is required; otherwise the process forks to the background and exits, and systemd keeps restarting it).

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

> ACP and CLI modes need the agent binaries inside the container. The default image contains only WeClaw; mount the binaries or build a custom image. HTTP mode works out of the box.

## Updates and Versions

```bash
weclaw update     # rebuild and reinstall from the source checkout (git pull --ff-only first if it has a remote); restarts a running bridge
weclaw version    # show the version
```

`update` uses the source directory recorded at build time (written by `make install`) and is equivalent to running `make install` there. The binary is replaced by writing a temp file and renaming it, never overwritten in place.

Versions come from git tags (`git describe`). Release flow: update `CHANGELOG.md` → commit → `git tag vX.Y.Z` → `make install`.

## Development

```bash
make dev      # hot reload
make build    # build to ./bin/weclaw
make test     # tests

# ACP end-to-end test (calls the real claude-agent-acp; skipped by default)
WECLAW_ACP_E2E=1 go test ./agent -run TestACPModelE2E -v
```

## Credits

Based on the work of [fastclaw-ai/weclaw](https://github.com/fastclaw-ai/weclaw) and its [contributors](https://github.com/fastclaw-ai/weclaw/graphs/contributors).

## License

[MIT](LICENSE)
