package messaging

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// friendlyError turns an agent error into a short Chinese message for the
// WeChat user. The raw error stays in the log; the user only sees what
// happened, whether it is their side or not, and what to do next. Unknown
// errors keep a truncated raw reason so nothing is hidden.
func friendlyError(agentName string, err error) string {
	if err == nil {
		return ""
	}
	name := agentDisplayName(agentName)
	raw := err.Error()
	low := strings.ToLower(raw)

	code := statusCode(raw)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(low, s) {
				return true
			}
		}
		return false
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("%s 处理超时，这条消息没有完成。可以重发，或把任务拆小一点。", name)

	// Usage quota (Claude subscription limits). Checked before network
	// errors because the limit text can arrive wrapped in other noise.
	case has("hit your", "usage limit", "limit reached", "limit · resets", "limit resets"):
		return quotaMessage(name, raw, low)

	case has("credit balance", "billing", "insufficient_quota", "insufficient balance"):
		return fmt.Sprintf("%s 的账户余额不足，充值后再发。", name)

	case code == 429 || has("rate_limit", "rate limit", "too many requests"):
		return fmt.Sprintf("请求太频繁，被 %s 暂时限流了。等一两分钟再发。", name)

	case code == 529 || has("overloaded"):
		return fmt.Sprintf("%s 服务器现在太忙，稍后再发一次。", name)

	case has("authentication_error", "invalid api key", "invalid x-api-key", "/login",
		"not logged in", "oauth token", "token has expired", "unauthorized", "please log in") || code == 401:
		return fmt.Sprintf("%s 的登录已失效，需要在电脑上重新登录后再发（终端运行 claude，输入 /login）。", name)

	case has("prompt is too long", "context length", "context window", "too many tokens", "maximum context"):
		return "当前会话太长，超出了上下文上限。发送 /new 开个新会话再继续。"

	case has("unrecognized_model", "model not found", "invalid model", "unknown model", "model_not_found"):
		return fmt.Sprintf("配置的模型 %s 不可用。请检查 ~/.weclaw/config.json 里的 model 设置。", modelFromError(raw))

	case has("econnreset", "etimedout", "econnrefused", "enotfound", "eai_again", "socket hang up",
		"fetch failed", "connection dropped", "connection error", "connection reset", "network",
		"no such host", "request timed out", "tls handshake"):
		return fmt.Sprintf("电脑连不上 %s 服务器（网络中断，已自动重试仍失败），这条消息没有处理。检查电脑网络或代理后重发即可。", name)

	case code >= 500 || has("api_error", "internal server error", "bad gateway", "service unavailable"):
		return fmt.Sprintf("%s 服务端出错了，不是你这边的问题。稍后重发即可。", name)

	case has("unexpected case"):
		return fmt.Sprintf("%s 返回了桥接器还不认识的内容，这轮回复中断了。任务可能已在电脑上做完，发「继续」查看结果。", name)

	case has("empty response"):
		return emptyReplyMessage(name, low)

	case has("agent startup failed", "start acp agent", "executable file not found"):
		return fmt.Sprintf("%s 启动失败，暂时无法回复。请在电脑上运行 weclaw status 查看情况。", name)

	case has("broken pipe", "file already closed", "write to stdin", "process exited", "signal: killed"):
		return fmt.Sprintf("%s 后台进程意外退出了。请在电脑上运行 weclaw restart 后重发。", name)

	case has("session error", "create new session", "create new thread"):
		return fmt.Sprintf("%s 新建会话失败。稍后重发，或发送 /new 再试。", name)

	case has("not available", "not found and no factory"):
		return fmt.Sprintf("%s 当前不可用（未安装或配置有误）。", name)
	}

	return fmt.Sprintf("%s 出错了，这条消息没有处理成功。\n原因：%s", name, truncateRunes(lastReason(raw), 160))
}

var (
	reResets   = regexp.MustCompile(`(?i)resets\s+(?:at\s+)?([^·()\n;]+?)\s*(?:\(([^)]+)\))?\s*(?:$|[;\n])`)
	reClock    = regexp.MustCompile(`(?i)^(\d{1,2})(?::(\d{2}))?\s*(am|pm)$`)
	reMonthDay = regexp.MustCompile(`(?i)^(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+(\d{1,2}),?\s*(?:at\s+)?(.*)$`)
	reStatus   = regexp.MustCompile(`(?i)(?:api error|status(?: code)?|http)[\s:=]*(\d{3})\b`)
	reModel    = regexp.MustCompile(`"model"\s*:\s*"([^"]+)"`)
)

var monthNumbers = map[string]int{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

func quotaMessage(name, raw, low string) string {
	kind := "用量额度"
	switch {
	case strings.Contains(low, "weekly"):
		kind = "本周用量额度"
	case strings.Contains(low, "session limit"), strings.Contains(low, "5-hour"):
		kind = "5 小时用量额度"
	case strings.Contains(low, "opus"):
		kind = "Opus 用量额度"
	}
	when := resetTime(raw)
	if when == "" {
		return fmt.Sprintf("%s 的%s已用完，暂时无法回复。额度恢复后再发即可。", name, kind)
	}
	return fmt.Sprintf("%s 的%s已用完，%s 恢复。到时重发即可。", name, kind, when)
}

// resetTime extracts "4:10pm" / "Nov 5, 7am" from a limit message and renders
// it as "16:10" / "11月5日 07:00". Unparseable values are returned as-is.
func resetTime(raw string) string {
	m := reResets.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	s := strings.TrimSpace(m[1])
	prefix := ""
	if md := reMonthDay.FindStringSubmatch(s); md != nil {
		prefix = fmt.Sprintf("%d月%s日 ", monthNumbers[strings.ToLower(md[1][:3])], md[2])
		s = strings.TrimSpace(md[3])
	}
	if c := reClock.FindStringSubmatch(s); c != nil {
		h, _ := strconv.Atoi(c[1])
		min := 0
		if c[2] != "" {
			min, _ = strconv.Atoi(c[2])
		}
		pm := strings.EqualFold(c[3], "pm")
		if pm && h < 12 {
			h += 12
		}
		if !pm && h == 12 {
			h = 0
		}
		return fmt.Sprintf("%s%02d:%02d", prefix, h, min)
	}
	return strings.TrimSpace(prefix + s)
}

func emptyReplyMessage(name, low string) string {
	switch {
	case strings.Contains(low, "stopreason=refusal"):
		return fmt.Sprintf("%s 拒绝回答这条消息。换个说法试试。", name)
	case strings.Contains(low, "stopreason=max_tokens"):
		return fmt.Sprintf("%s 的回复超出长度上限被截断了。发「继续」让它接着说。", name)
	case strings.Contains(low, "stopreason=cancelled"):
		return "这一轮被中途取消了，没有回复。需要的话请重发。"
	}
	return fmt.Sprintf("%s 这一轮没有输出文字（可能只在电脑上执行了操作）。发「继续」查看结果，或换个问法。", name)
}

// statusCode returns an HTTP status mentioned as "API Error: 529",
// "status 429" or "HTTP 401"; bare numbers are ignored because session IDs and
// durations contain them by chance.
func statusCode(raw string) int {
	if m := reStatus.FindStringSubmatch(raw); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func modelFromError(raw string) string {
	if m := reModel.FindStringSubmatch(raw); m != nil {
		return "「" + m[1] + "」"
	}
	return ""
}

// lastReason drops the wrapper prefixes ("prompt error: agent error: ...")
// that only describe where the error travelled through.
func lastReason(raw string) string {
	s := raw
	for _, p := range []string{"prompt error: ", "agent error: ", "session error: ", "Internal error: ", "RequestError: "} {
		for {
			i := strings.Index(s, p)
			if i < 0 {
				break
			}
			s = s[i+len(p):]
		}
	}
	return strings.TrimSpace(s)
}

func agentDisplayName(name string) string {
	switch strings.ToLower(name) {
	case "", "claude", "cc":
		return "Claude"
	case "codex", "cx":
		return "Codex"
	case "gemini", "gm":
		return "Gemini"
	case "cursor", "cs":
		return "Cursor"
	case "kimi", "km":
		return "Kimi"
	}
	return name
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
