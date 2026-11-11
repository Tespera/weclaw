# Security Policy

## Supported versions

Only the latest release of WeClaw receives security fixes. Upgrade with `weclaw update`.

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Report privately through [GitHub private vulnerability reporting](https://github.com/Tespera/weclaw/security/advisories/new). Include:

- what an attacker can do, and under which configuration;
- steps to reproduce, or a proof of concept;
- the affected version (`weclaw version`).

You should get an initial response within 7 days.

## Things worth knowing

- WeClaw lets whoever can message the bot run AI agents on your machine. Agents in ACP mode auto-approve tool permissions, and `mode: bypassPermissions` or `--dangerously-skip-permissions` remove all prompts. Only log in with a WeChat account you control.
- The local HTTP API listens on `127.0.0.1:18011` by default. If you bind it elsewhere with `WECLAW_API_ADDR`, anyone who can reach it can send WeChat messages as your bot. Reading local files through `media` is always restricted to loopback callers.
- Login tokens are stored in `~/.weclaw/accounts/`. Treat them like passwords.
