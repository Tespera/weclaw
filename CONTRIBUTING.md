# Contributing to WeClaw

Thanks for helping improve WeClaw. Issues and pull requests are welcome in English or Chinese (中文同样欢迎).

## Before you start

- **Bugs:** open an issue with the [bug report template](https://github.com/Tespera/weclaw/issues/new?template=bug_report.yml). Include `weclaw version`, your OS, the agent and mode (ACP / CLI / HTTP), and the relevant lines from `~/.weclaw/weclaw.log`. Remove tokens, WeChat IDs and API keys first.
- **Features:** open an issue before you write a large change, so we can agree on the approach first.
- **Security issues:** don't open a public issue. See [SECURITY.md](SECURITY.md).

## Development setup

Requires Go 1.25+.

```bash
git clone https://github.com/Tespera/weclaw.git
cd weclaw
make build    # ./bin/weclaw
make test     # go test ./...
make dev      # hot reload with air
```

Changes to ACP sessions or model handling should also pass the end-to-end test, which calls the real `claude-agent-acp`:

```bash
WECLAW_ACP_E2E=1 go test ./agent -run TestACPModelE2E -v
```

launchd logic is tested with a fake `launchctl` (`cmd/service_darwin_test.go`). Tests must never call the real one.

## Pull request checklist

- [ ] `go test ./...` passes.
- [ ] User-facing changes are documented in **both** `README.md` and `README_CN.md`. The two files must stay in sync.
- [ ] Notable changes are added to `CHANGELOG.md` (written in Chinese).
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`fix:`, `feat:`, `docs:`, `chore:` …).
- [ ] No hard-coded version strings. Versions come from `git describe` via ldflags.

## README images

The images in `docs/assets/` are rendered from HTML. Edit `docs/assets/src/render.py` (copy, layout) or `docs/assets/src/base.css` (styles), then run:

```bash
python3 -m pip install playwright pillow && python3 -m playwright install chromium
python3 docs/assets/src/render.py
```

## License

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE).
