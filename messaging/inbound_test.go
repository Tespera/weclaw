package messaging

import (
	"strings"
	"testing"

	"weclaw/ilink"
)

func TestTakePendingInboundPrefixesAndClears(t *testing.T) {
	h := newTestHandler()
	msg := ilink.WeixinMessage{FromUserID: "u1"}
	h.queueInbound(nil, nil, msg, inboundAttachment{Kind: "文件", Name: "报告.pdf", Path: "/tmp/inbox/报告.pdf", Size: 1536})
	h.queueInbound(nil, nil, msg, inboundAttachment{Kind: "图片", Path: "/tmp/inbox/image-1.jpg", Size: 2 << 20})

	got := h.takePendingInbound("u1", "帮我看看")
	for _, w := range []string{"2 个附件", "文件「报告.pdf」（2 KB）：/tmp/inbox/报告.pdf", "图片（2.0 MB）：/tmp/inbox/image-1.jpg", "\n\n帮我看看"} {
		if !strings.Contains(got, w) {
			t.Errorf("want %q in %q", w, got)
		}
	}
	if again := h.takePendingInbound("u1", "再问"); again != "再问" {
		t.Errorf("pending not cleared: %q", again)
	}
	if other := h.takePendingInbound("u2", "hi"); other != "hi" {
		t.Errorf("other user affected: %q", other)
	}
}

func TestInboundAckText(t *testing.T) {
	if got := inboundAckText([]inboundAttachment{{Kind: "文件", Name: "a.xlsx", Size: 3 << 20}}); got != "已收到文件「a.xlsx」（3.0 MB）。接着说说要我做什么。" {
		t.Errorf("got %q", got)
	}
	if got := inboundAckText([]inboundAttachment{{Kind: "图片"}}); got != "已收到图片。接着说说要我做什么。" {
		t.Errorf("got %q", got)
	}
	if got := inboundAckText(make([]inboundAttachment, 3)); !strings.HasPrefix(got, "已收到 3 个附件") {
		t.Errorf("got %q", got)
	}
}

func TestHasInboundMedia(t *testing.T) {
	file := ilink.WeixinMessage{ItemList: []ilink.MessageItem{{Type: ilink.ItemTypeFile, FileItem: &ilink.FileItem{FileName: "a.pdf"}}}}
	text := ilink.WeixinMessage{ItemList: []ilink.MessageItem{{Type: ilink.ItemTypeText, TextItem: &ilink.TextItem{Text: "hi"}}}}
	if !hasInboundMedia(file) || hasInboundMedia(text) {
		t.Fatal("hasInboundMedia wrong")
	}
}

func TestInboundFailureText(t *testing.T) {
	item := ilink.MessageItem{Type: ilink.ItemTypeFile, FileItem: &ilink.FileItem{FileName: "big.zip"}}
	got := inboundFailureText(item, errInboundTooLarge)
	if !strings.Contains(got, "文件「big.zip」超过 200.0 MB 上限") {
		t.Errorf("got %q", got)
	}
}

func TestSanitizeInboundName(t *testing.T) {
	for in, want := range map[string]string{"../../etc/passwd": "passwd", `C:\x\y.txt`: "y.txt", "a:b.pdf": "a_b.pdf", "": "", "..": ""} {
		if got := sanitizeInboundName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnsupportedMessageText(t *testing.T) {
	voice := ilink.WeixinMessage{ItemList: []ilink.MessageItem{{Type: ilink.ItemTypeVoice, VoiceItem: &ilink.VoiceItem{}}}}
	if !strings.Contains(unsupportedMessageText(voice), "语音没能转成文字") {
		t.Error("voice text")
	}
}
