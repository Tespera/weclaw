#!/usr/bin/env python3
"""Render the README artwork in docs/assets/ from HTML templates.

Usage:  python3 docs/assets/src/render.py
Needs:  pip install playwright pillow && python3 -m playwright install chromium

Every image is drawn in HTML/CSS with system fonts and screenshotted at 2x,
so copy changes are a text edit here plus one re-run.
"""

import html
import pathlib
import tempfile

from PIL import Image
from playwright.sync_api import sync_playwright

SRC = pathlib.Path(__file__).resolve().parent
OUT = SRC.parent
PREVIEWS = SRC / "screenshots"

AGENTS = [
    ("Claude Code", "claude"),
    ("Codex", "codex"),
    ("Gemini CLI", "gemini"),
    ("Kimi", "kimi"),
    ("Cursor", "cursor"),
    ("OpenCode", "opencode"),
    ("OpenClaw", "openclaw"),
]

LOGO = """
<svg viewBox="0 0 64 64" width="{s}" height="{s}" aria-hidden="true">
  <defs>
    <linearGradient id="g" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#2bd97e"/><stop offset="1" stop-color="#07a352"/>
    </linearGradient>
  </defs>
  <rect x="2" y="2" width="60" height="60" rx="16" fill="url(#g)"/>
  <path d="M15 19h34a5 5 0 0 1 5 5v15a5 5 0 0 1-5 5H30l-9 7v-7h-6a5 5 0 0 1-5-5V24a5 5 0 0 1 5-5z"
        fill="#fff"/>
  <path d="M24 37l5-12M31 37l5-12M38 37l5-12" stroke="#07a352" stroke-width="3.6"
        stroke-linecap="round"/>
</svg>
"""

T = {
    "en": {
        "lang": "en",
        "eyebrow": "Open source · Single Go binary · MIT",
        "tagline": "Your AI coding agents,<br>one WeChat message away.",
        "sub": "Bridge WeChat to Claude Code, Codex, Gemini CLI and more. "
               "Runs on your own machine. No server, no public IP.",
        "chat": [
            ("me", "<span class='tag'>/cc</span> CI on main is red — can you take a look?"),
            ("bot", "Found it: a flaky timeout in one test. Fixed it and pushed "
                    "<code>fix/ci-timeout</code>. All tests pass now ✓"),
            ("file", ("ci-report.pdf", "84 KB · sent by Claude")),
            ("me", "<span class='tag'>/cx</span> review that diff please"),
        ],
        "arch_title": "How WeClaw works",
        "arch_sub": "WeClaw polls WeChat over outbound HTTPS and routes each chat to a local agent.",
        "you": ("You", "WeChat on your phone"),
        "cloud": ("WeChat ClawBot", "iLink relay (Tencent cloud)"),
        "poll": "long-poll · HTTPS<br>outbound only",
        "host": ("WeClaw", "on your Mac / Linux box"),
        "host_rows": [
            ("Router", "/cc · /cx · /gm aliases, default agent"),
            ("Sessions", "one session per chat, queued replies"),
            ("Media", "images, files, video in and out"),
            ("Push", "weclaw send · HTTP API :18011"),
        ],
        "acp": ("ACP", "long-running, fastest"),
        "cli": ("CLI", "one process per message"),
        "http": ("HTTP", "OpenAI-compatible"),
        "shots": [
            ("preview1.png", "Ask any agent", "/claude · /gemini · /codex"),
            ("preview2.png", "Switch the default", "/cc makes Claude the default"),
            ("preview3.png", "Short aliases", "/gm · /cx · /km · /oc"),
        ],
        "og_tag": "Chat with Claude Code, Codex & more<br>right from WeChat.",
    },
    "zh": {
        "lang": "zh-CN",
        "eyebrow": "开源 · Go 单二进制 · MIT",
        "tagline": "在微信里，<br>随时指挥你的 AI 编程 Agent",
        "sub": "把 Claude Code、Codex、Gemini CLI 等接进微信。"
               "运行在你自己的电脑上，不用服务器，不用公网 IP。",
        "chat": [
            ("me", "<span class='tag'>/cc</span> main 分支 CI 挂了，帮我看下"),
            ("bot", "找到了：有个测试超时不稳定。已修好并推送到 "
                    "<code>fix/ci-timeout</code>，测试全部通过 ✓"),
            ("file", ("ci-report.pdf", "84 KB · 来自 Claude")),
            ("me", "<span class='tag'>/cx</span> 帮我 review 一下这个改动"),
        ],
        "arch_title": "WeClaw 如何工作",
        "arch_sub": "WeClaw 主动连接微信拉取消息，再把每个聊天转给本机的 AI Agent。",
        "you": ("你", "手机上的微信"),
        "cloud": ("微信 ClawBot", "iLink 中继（腾讯云端）"),
        "poll": "长轮询 · HTTPS<br>只出不进，无需公网 IP",
        "host": ("WeClaw", "运行在你的 Mac / Linux 上"),
        "host_rows": [
            ("路由", "/cc · /cx · /gm 别名、默认 Agent"),
            ("会话", "每个聊天一个会话，消息排队回复"),
            ("媒体", "图片、文件、视频收发"),
            ("推送", "weclaw send · HTTP API :18011"),
        ],
        "acp": ("ACP", "常驻进程，响应最快"),
        "cli": ("CLI", "每条消息起一个进程"),
        "http": ("HTTP", "OpenAI 兼容接口"),
        "shots": [
            ("preview1.png", "点名任意 Agent", "/claude · /gemini · /codex"),
            ("preview2.png", "一键切换默认", "发 /cc 即切到 Claude"),
            ("preview3.png", "短别名", "/gm · /cx · /km · /oc"),
        ],
        "og_tag": "在微信里和 Claude Code、Codex 等 AI Agent 对话",
    },
}


def chips(names=None):
    items = [a for a in AGENTS if names is None or a[0] in names]
    return "".join(f'<span class="chip"><i style="background:var(--{k})"></i>{html.escape(n)}</span>'
                   for n, k in items)


def chat(t):
    out = []
    for kind, body in t["chat"]:
        if kind == "file":
            name, meta = body
            out.append(f'<div class="file"><div class="ic">PDF</div><div><b>{name}</b><span>{meta}</span></div></div>')
        else:
            out.append(f'<div class="msg {kind}">{body}</div>')
    return ('<div class="phone"><div class="screen"><div class="bar">微信ClawBot <span class="ai">AI</span></div>'
            f'<div class="msgs"><div class="time">09:41</div>{"".join(out)}</div></div></div>')


def page(t, body, css=""):
    return (f'<!doctype html><html lang="{t["lang"]}"><head><meta charset="utf-8">'
            f'<link rel="stylesheet" href="{(SRC / "base.css").as_uri()}"><style>{css}</style></head>'
            f'<body>{body}</body></html>')


BANNER_CSS = """
.canvas { width: 1280px; height: 480px; display: grid; grid-template-columns: 1fr 360px; gap: 28px;
          padding: 60px 60px 0 72px; }
.left { padding-top: 6px; }
.brand { display: flex; align-items: center; gap: 18px; margin: 26px 0 22px; }
.brand .wordmark { font-size: 76px; }
h2 { font-size: 34px; line-height: 1.22; font-weight: 700; letter-spacing: -0.01em; margin-bottom: 16px; }
p.sub { font-size: 17px; line-height: 1.6; color: var(--muted); max-width: 620px; margin-bottom: 26px; }
.right { display: flex; justify-content: center; align-items: flex-start; }
.right .phone { transform: translateY(8px) rotate(-2deg); }
.right .screen { min-height: 560px; }
.chips { flex-wrap: nowrap; gap: 7px; }
.chip { font-size: 13.5px; padding: 6px 11px 6px 9px; gap: 7px; }
"""


def banner(t):
    return page(t, f"""
<div class="canvas" id="c">
  <div class="left">
    <div class="eyebrow">{t["eyebrow"]}</div>
    <div class="brand">{LOGO.format(s=72)}<div class="wordmark"><span class="we">We</span><span class="claw">Claw</span></div></div>
    <h2>{t["tagline"]}</h2>
    <p class="sub">{t["sub"]}</p>
    <div class="chips">{chips()}</div>
  </div>
  <div class="right">{chat(t)}</div>
</div>""", BANNER_CSS)


ARCH_CSS = """
.canvas { width: 1280px; padding: 48px 56px 52px; }
h3 { font-size: 28px; font-weight: 750; letter-spacing: -0.01em; }
.lead { color: var(--muted); font-size: 16px; margin: 8px 0 26px; }
.flow { display: grid; grid-template-columns: 160px 46px 196px 168px 296px 46px 1fr; align-items: center; }
.node { background: var(--surface); border: 1px solid var(--line-2); border-radius: 18px; padding: 18px 18px; }
.node h4 { font-size: 18px; font-weight: 700; margin-bottom: 4px; }
.node small { font-size: 13px; color: var(--muted); line-height: 1.4; display: block; }
.node.you { text-align: center; }
.node.you .av { width: 54px; height: 54px; margin: 0 auto 10px; border-radius: 50%;
  background: linear-gradient(160deg, #2a3a33, #15201b); border: 1px solid var(--line-2); display: grid; place-items: center; font-size: 26px; }
.node.cloud { text-align: center; border-style: dashed; }
.node.cloud .ic { font-size: 28px; margin-bottom: 8px; color: var(--green-2); }
.arrow { height: 2px; background: linear-gradient(90deg, var(--line-2), var(--green)); position: relative; }
.arrow::after { content: ""; position: absolute; right: -2px; top: -5px; border: 6px solid transparent; border-left: 9px solid var(--green); }
.duplex { position: relative; height: 70px; }
.duplex .l { position: absolute; left: 8px; right: 8px; height: 2px; background: var(--green); opacity: .9; }
.duplex .l.a { top: 26px; } .duplex .l.b { top: 40px; background: var(--line-2); }
.duplex .l.a::after { content: ""; position: absolute; right: -2px; top: -5px; border: 6px solid transparent; border-left: 9px solid var(--green); }
.duplex .l.b::before { content: ""; position: absolute; left: -2px; top: -5px; border: 6px solid transparent; border-right: 9px solid var(--line-2); }
.duplex span { position: absolute; top: 52px; left: 0; right: 0; text-align: center; font-family: var(--mono); font-size: 11px; color: var(--green-2); line-height: 1.5; padding: 0 6px; }
.host { background: linear-gradient(180deg, rgba(7,193,96,.14), rgba(7,193,96,.03)); border: 1px solid rgba(43,217,126,.45);
  border-radius: 22px; padding: 18px; box-shadow: 0 0 50px -12px rgba(7,193,96,.5); }
.host .hd { display: flex; align-items: center; gap: 12px; margin-bottom: 14px; }
.host .hd b { font-size: 20px; font-weight: 800; letter-spacing: -.02em; display: block; }
.host .hd small { font-size: 12.5px; color: var(--muted); }
.row { display: grid; grid-template-columns: 74px 1fr; gap: 10px; align-items: baseline; padding: 9px 12px; border-radius: 10px;
  background: rgba(0,0,0,.28); border: 1px solid var(--line); margin-top: 7px; }
.row b { font-size: 13px; color: var(--green-2); font-weight: 650; }
.row span { font-size: 12.5px; color: var(--text); opacity: .9; }
.agents { display: flex; flex-direction: column; gap: 14px; }
.grp { background: var(--surface); border: 1px solid var(--line-2); border-radius: 16px; padding: 14px 16px; }
.grp .t { display: flex; align-items: baseline; gap: 10px; margin-bottom: 10px; }
.grp .t b { font-family: var(--mono); font-size: 14px; color: var(--green-2); }
.grp .t small { font-size: 12.5px; color: var(--muted); }
.grp .chips { gap: 6px; } .grp .chip { font-size: 12.5px; padding: 5px 10px 5px 8px; }
.grp .chip code { font-family: var(--mono); font-size: 12px; }
"""


def arch(t):
    rows = "".join(f'<div class="row"><b>{a}</b><span>{b}</span></div>' for a, b in t["host_rows"])
    return page(t, f"""
<div class="canvas" id="c">
  <h3>{t["arch_title"]}</h3>
  <div class="lead">{t["arch_sub"]}</div>
  <div class="flow">
    <div class="node you"><div class="av">💬</div><h4>{t["you"][0]}</h4><small>{t["you"][1]}</small></div>
    <div class="arrow" style="margin:0 6px"></div>
    <div class="node cloud"><div class="ic">☁︎</div><h4>{t["cloud"][0]}</h4><small>{t["cloud"][1]}</small></div>
    <div class="duplex"><div class="l a"></div><div class="l b"></div><span>{t["poll"]}</span></div>
    <div class="host"><div class="hd">{LOGO.format(s=40)}<div><b>{t["host"][0]}</b><small>{t["host"][1]}</small></div></div>{rows}</div>
    <div class="arrow" style="margin:0 6px"></div>
    <div class="agents">
      <div class="grp"><div class="t"><b>{t["acp"][0]}</b><small>{t["acp"][1]}</small></div>
        <div class="chips">{chips(["Claude Code","Codex","Gemini CLI","Kimi","Cursor","OpenCode","OpenClaw"])}</div></div>
      <div class="grp"><div class="t"><b>{t["cli"][0]}</b><small>{t["cli"][1]}</small></div>
        <div class="chips"><span class="chip"><i style="background:var(--claude)"></i><code>claude -p</code></span><span class="chip"><i style="background:var(--codex)"></i><code>codex exec</code></span></div></div>
      <div class="grp"><div class="t"><b>{t["http"][0]}</b><small>{t["http"][1]}</small></div>
        <div class="chips">{chips(["OpenClaw"])}</div></div>
    </div>
  </div>
</div>""", ARCH_CSS)


SHOTS_CSS = """
.canvas { width: 1280px; padding: 54px 70px 0; display: grid; grid-template-columns: repeat(3, 1fr); gap: 46px; height: 720px; }
.col { display: flex; flex-direction: column; align-items: center; }
.cap { text-align: center; margin-bottom: 22px; }
.cap b { display: block; font-size: 21px; font-weight: 750; margin-bottom: 6px; }
.cap span { font-family: var(--mono); font-size: 13px; color: var(--green-2); }
.dev { width: 318px; border-radius: 40px 40px 0 0; padding: 11px 11px 0; background: linear-gradient(160deg, #2a3832, #0f1813);
  box-shadow: 0 0 0 1px rgba(255,255,255,.08) inset, 0 30px 70px -20px rgba(0,0,0,.8), 0 0 60px -18px rgba(7,193,96,.45); }
.dev img { width: 100%; display: block; border-radius: 30px 30px 0 0; }
"""


def shots(t):
    cols = "".join(
        f'<div class="col"><div class="cap"><b>{b}</b><span>{s}</span></div>'
        f'<div class="dev"><img src="{(PREVIEWS / f).as_uri()}"></div></div>'
        for f, b, s in t["shots"])
    return page(t, f'<div class="canvas" id="c">{cols}</div>', SHOTS_CSS)


OG_CSS = """
.canvas { width: 1280px; height: 640px; border-radius: 0; border: 0; padding: 0 64px 0 80px; display: grid;
          grid-template-columns: 1fr 340px; gap: 28px; align-items: center; }
.brand { display: flex; align-items: center; gap: 22px; margin: 22px 0 22px; }
.brand .wordmark { font-size: 96px; }
h2 { font-size: 34px; line-height: 1.3; font-weight: 700; margin-bottom: 30px; max-width: 640px; }
#c .chips { flex-wrap: nowrap; gap: 7px; } #c .chip { font-size: 13.5px; padding: 6px 11px 6px 9px; }
.repo { font-family: var(--mono); font-size: 18px; color: var(--muted); margin-top: 30px; }
.phone { transform: rotate(-3deg); }
"""


def og(t):
    return page(t, f"""
<div class="canvas" id="c">
  <div>
    <div class="eyebrow">{t["eyebrow"]}</div>
    <div class="brand">{LOGO.format(s=88)}<div class="wordmark"><span class="we">We</span><span class="claw">Claw</span></div></div>
    <h2>{t["og_tag"]}</h2>
    <div class="chips">{chips()}</div>
    <div class="repo">github.com/Tespera/weclaw</div>
  </div>
  <div>{chat(t)}</div>
</div>""", OG_CSS)


JOBS = [
    ("banner", banner, ["en", "zh"]),
    ("architecture", arch, ["en", "zh"]),
    ("screenshots", shots, ["en", "zh"]),
    ("social-preview", og, ["en"]),
]


def main():
    with sync_playwright() as p, tempfile.TemporaryDirectory() as tmp:
        browser = p.chromium.launch()
        ctx = browser.new_context(device_scale_factor=2, viewport={"width": 1400, "height": 1000})
        pg = ctx.new_page()
        for name, fn, langs in JOBS:
            for lang in langs:
                f = pathlib.Path(tmp) / f"{name}-{lang}.html"
                f.write_text(fn(T[lang]), encoding="utf-8")
                pg.goto(f.as_uri())
                pg.wait_for_load_state("networkidle")
                suffix = "" if lang == "en" else f"-{lang}"
                out = OUT / f"{name}{suffix}.png"
                pg.locator("#c").screenshot(path=str(out), omit_background=True)
                if name != "social-preview":
                    webp = out.with_suffix(".webp")
                    Image.open(out).save(webp, "WEBP", quality=90, method=6)
                    out.unlink()
                    out = webp
                print("wrote", out.relative_to(OUT.parents[1]), f"{out.stat().st_size // 1024} KB")
        browser.close()


if __name__ == "__main__":
    main()
