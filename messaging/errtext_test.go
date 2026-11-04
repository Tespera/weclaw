package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFriendlyError(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string // substrings that must appear
		not  []string // substrings that must not appear
	}{
		{"econnreset (real log)", "prompt error: agent error: Internal error: API Error: Connection dropped (ECONNRESET)",
			[]string{"网络中断", "重发"}, []string{"ECONNRESET", "prompt error"}},
		{"session limit (real log)", "prompt error: agent error: Session 726b20ed-8ee1-49ff-801e-35e722ae3288: cannot fail active turn because no unsettled active turn exists: RequestError: Internal error: You've hit your session limit · resets 4:10pm (Europe/Oslo)",
			[]string{"5 小时用量额度已用完", "16:10 恢复"}, []string{"Session 726b"}},
		{"weekly limit (real log)", "agent error: Internal error: You've hit your weekly limit · resets 7am (Europe/Oslo)",
			[]string{"本周用量额度已用完", "07:00 恢复"}, nil},
		{"limit with date", "You've hit your weekly limit · resets Nov 5, 7am (Europe/Oslo)",
			[]string{"11月5日 07:00 恢复"}, nil},
		{"unexpected case (real log)", `prompt error: agent error: Unexpected case: {"type":"system","subtype":"code_change_published"}`,
			[]string{"不认识", "继续"}, []string{"code_change_published"}},
		{"empty response", "agent returned empty response (stopReason=end_turn)", []string{"没有输出文字"}, nil},
		{"refusal", "agent returned empty response (stopReason=refusal)", []string{"拒绝"}, nil},
		{"max tokens", "agent returned empty response (stopReason=max_tokens)", []string{"截断"}, nil},
		{"overloaded", `agent error: API Error: 529 {"type":"error","error":{"type":"overloaded_error"}}`, []string{"太忙"}, nil},
		{"rate limited", "agent error: API Error: 429 rate_limit_error", []string{"限流"}, nil},
		{"server 500", "agent error: API Error: 500 Internal server error", []string{"服务端出错"}, nil},
		{"auth", "agent error: Invalid API key · Please run /login", []string{"登录已失效", "/login"}, nil},
		{"too long", "agent error: prompt is too long: 210000 tokens > 200000 maximum", []string{"/new"}, nil},
		{"bad model", `agent error: [claude-code:unrecognized_model] {"model":"kimi-k3"}`, []string{"「kimi-k3」"}, nil},
		{"startup", "agent startup failed: spawn ENOENT", []string{"启动失败"}, nil},
		{"uuid digits are not status codes", "agent error: something odd sessionId=500429-401 durationMs=529",
			[]string{"出错了", "原因：something odd"}, []string{"限流", "太忙", "登录"}},
		{"unknown keeps reason", "prompt error: agent error: weird thing happened",
			[]string{"原因：weird thing happened"}, []string{"prompt error"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := friendlyError("claude", errors.New(c.raw))
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("want %q in %q", w, got)
				}
			}
			for _, n := range c.not {
				if strings.Contains(got, n) {
					t.Errorf("did not want %q in %q", n, got)
				}
			}
		})
	}
}

func TestFriendlyErrorTimeoutAndName(t *testing.T) {
	got := friendlyError("codex", fmt.Errorf("x: %w", context.DeadlineExceeded))
	if !strings.HasPrefix(got, "Codex 处理超时") {
		t.Errorf("got %q", got)
	}
}
