package messaging

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"weclaw/ilink"
)

// Files, images and videos a user sends are saved under the attachment
// workspace and handed to the agent together with the user's next text
// message. WeChat users usually send the file first and the instruction
// second, so forwarding the file on its own would make the agent guess.

const (
	maxInboundFileSize = 200 << 20
	// inboundAckDelay batches the acknowledgement when several files arrive
	// back to back (e.g. nine images picked at once).
	inboundAckDelay = 2 * time.Second
)

var errInboundTooLarge = errors.New("file too large")

type inboundAttachment struct {
	Kind string // "图片" / "文件" / "视频"
	Name string // original file name, empty for images and videos
	Path string
	Size int64
}

type pendingInbound struct {
	items []inboundAttachment
	ack   *time.Timer
}

// InboxDir is where files received from WeChat users are saved.
func InboxDir() string {
	return filepath.Join(defaultAttachmentWorkspace(), "inbox")
}

// hasInboundMedia reports whether the message carries a file, image or video.
func hasInboundMedia(msg ilink.WeixinMessage) bool {
	for _, item := range msg.ItemList {
		switch {
		case item.Type == ilink.ItemTypeImage && item.ImageItem != nil,
			item.Type == ilink.ItemTypeFile && item.FileItem != nil,
			item.Type == ilink.ItemTypeVideo && item.VideoItem != nil:
			return true
		}
	}
	return false
}

// handleInboundMedia saves every file, image and video in msg, queues them
// for the user's next message, and tells the user they arrived (or why not).
func (h *Handler) handleInboundMedia(ctx context.Context, client *ilink.Client, msg ilink.WeixinMessage) {
	h.contextTokens.Store(msg.FromUserID, msg.ContextToken)

	var failures []string
	for _, item := range msg.ItemList {
		att, err := h.saveInboundItem(ctx, msg, item)
		if att == nil && err == nil {
			continue // not a media item
		}
		if err != nil {
			log.Printf("[handler] failed to receive %s from %s: %v", describeItem(item), msg.FromUserID, err)
			failures = append(failures, inboundFailureText(item, err))
			continue
		}
		log.Printf("[handler] saved %s from %s to %s (%d bytes)", att.Kind, msg.FromUserID, att.Path, att.Size)
		if item.Type == ilink.ItemTypeImage && h.saveDir != "" {
			go h.handleImageSave(ctx, client, msg, item.ImageItem) // optional Linkhoard copy
		}
		h.queueInbound(ctx, client, msg, *att)
	}

	if len(failures) > 0 {
		reply := strings.Join(failures, "\n")
		if err := SendTextReply(ctx, client, msg.FromUserID, reply, msg.ContextToken, NewClientID()); err != nil {
			log.Printf("[handler] failed to send reply to %s: %v", msg.FromUserID, err)
		}
	}
}

// saveInboundItem downloads one media item and writes it to the inbox.
// It returns (nil, nil) for items that are not media.
func (h *Handler) saveInboundItem(ctx context.Context, msg ilink.WeixinMessage, item ilink.MessageItem) (*inboundAttachment, error) {
	var (
		kind, name string
		media      *ilink.MediaInfo
		aesKey     string
		declared   int64
	)
	switch {
	case item.Type == ilink.ItemTypeImage && item.ImageItem != nil:
		kind, media = "图片", item.ImageItem.Media
		if media != nil {
			aesKey = media.AESKey
		}
		if item.ImageItem.AESKey != "" {
			aesKey = HexKeyToBase64(item.ImageItem.AESKey)
		}
	case item.Type == ilink.ItemTypeFile && item.FileItem != nil:
		kind, media, name = "文件", item.FileItem.Media, item.FileItem.FileName
		declared, _ = strconv.ParseInt(item.FileItem.Len, 10, 64)
	case item.Type == ilink.ItemTypeVideo && item.VideoItem != nil:
		kind, media = "视频", item.VideoItem.Media
	default:
		return nil, nil
	}

	if media == nil || media.EncryptQueryParam == "" {
		return nil, errors.New("no CDN media info")
	}
	if aesKey == "" {
		aesKey = media.AESKey
	}
	if declared > maxInboundFileSize {
		return nil, fmt.Errorf("%w: %d bytes", errInboundTooLarge, declared)
	}

	data, err := DownloadFileFromCDN(ctx, media.EncryptQueryParam, aesKey)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	fileName := sanitizeInboundName(name)
	switch {
	case fileName != "":
	case kind == "图片":
		fileName = "image-" + now.Format("150405") + detectImageExt(data)
	case kind == "视频":
		fileName = "video-" + now.Format("150405") + ".mp4"
	default:
		fileName = "file-" + now.Format("150405")
	}

	dir := filepath.Join(InboxDir(), now.Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create inbox: %w", err)
	}
	path := uniquePath(filepath.Join(dir, fileName))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return &inboundAttachment{Kind: kind, Name: name, Path: path, Size: int64(len(data))}, nil
}

// queueInbound adds an attachment to the user's pending list and (re)arms a
// single acknowledgement covering everything received in a short burst.
func (h *Handler) queueInbound(ctx context.Context, client *ilink.Client, msg ilink.WeixinMessage, att inboundAttachment) {
	h.inboundMu.Lock()
	defer h.inboundMu.Unlock()
	if h.inbound == nil {
		h.inbound = make(map[string]*pendingInbound)
	}
	p := h.inbound[msg.FromUserID]
	if p == nil {
		p = &pendingInbound{}
		h.inbound[msg.FromUserID] = p
	}
	p.items = append(p.items, att)
	if p.ack != nil {
		p.ack.Stop()
	}
	if client == nil {
		return
	}
	userID, token := msg.FromUserID, msg.ContextToken
	p.ack = time.AfterFunc(inboundAckDelay, func() {
		h.inboundMu.Lock()
		var items []inboundAttachment
		if cur := h.inbound[userID]; cur != nil {
			items = append(items, cur.items...)
			cur.ack = nil
		}
		h.inboundMu.Unlock()
		if len(items) == 0 {
			return // already consumed by a text message
		}
		if err := SendTextReply(ctx, client, userID, inboundAckText(items), token, NewClientID()); err != nil {
			log.Printf("[handler] failed to send reply to %s: %v", userID, err)
		}
	})
}

// takePendingInbound prefixes message with the attachments the user sent
// since their last message, and clears them.
func (h *Handler) takePendingInbound(userID, message string) string {
	h.inboundMu.Lock()
	p := h.inbound[userID]
	delete(h.inbound, userID)
	h.inboundMu.Unlock()
	if p == nil || len(p.items) == 0 {
		return message
	}
	if p.ack != nil {
		p.ack.Stop()
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[用户通过微信发来 %d 个附件，已保存到本机，可直接读取：\n", len(p.items))
	for _, it := range p.items {
		b.WriteString("- " + attachmentLabel(it) + "：" + it.Path + "\n")
	}
	b.WriteString("]\n\n")
	b.WriteString(message)
	return b.String()
}

func attachmentLabel(it inboundAttachment) string {
	if it.Name != "" {
		return fmt.Sprintf("%s「%s」（%s）", it.Kind, it.Name, humanSize(it.Size))
	}
	return fmt.Sprintf("%s（%s）", it.Kind, humanSize(it.Size))
}

func inboundAckText(items []inboundAttachment) string {
	if len(items) == 1 {
		it := items[0]
		if it.Name != "" {
			return fmt.Sprintf("已收到文件「%s」（%s）。接着说说要我做什么。", it.Name, humanSize(it.Size))
		}
		return fmt.Sprintf("已收到%s。接着说说要我做什么。", it.Kind)
	}
	return fmt.Sprintf("已收到 %d 个附件。接着说说要我做什么。", len(items))
}

func inboundFailureText(item ilink.MessageItem, err error) string {
	what := "这个" + itemKind(item)
	if item.FileItem != nil && item.FileItem.FileName != "" {
		what = "文件「" + item.FileItem.FileName + "」"
	}
	switch {
	case errors.Is(err, errInboundTooLarge):
		return fmt.Sprintf("%s超过 %s 上限，没有接收。可以压缩后再发，或直接放到电脑上告诉我路径。", what, humanSize(maxInboundFileSize))
	case strings.Contains(err.Error(), "no CDN media info"):
		return what + "没能接收：微信没有提供下载地址。请重新发送一次。"
	case strings.Contains(err.Error(), "write file"), strings.Contains(err.Error(), "create inbox"):
		return what + "下载成功但保存到电脑失败（可能磁盘已满）。请检查电脑后重发。"
	}
	return what + "下载失败（可能是网络问题），Claude 没有收到。请重新发送。"
}

func itemKind(item ilink.MessageItem) string {
	switch item.Type {
	case ilink.ItemTypeImage:
		return "图片"
	case ilink.ItemTypeFile:
		return "文件"
	case ilink.ItemTypeVideo:
		return "视频"
	case ilink.ItemTypeVoice:
		return "语音"
	}
	return "消息"
}

func describeItem(item ilink.MessageItem) string {
	if item.FileItem != nil && item.FileItem.FileName != "" {
		return fmt.Sprintf("file %q", item.FileItem.FileName)
	}
	return fmt.Sprintf("item type=%d", item.Type)
}

// unsupportedMessageText explains a message weclaw cannot forward. It returns
// "" when there is nothing to tell the user.
func unsupportedMessageText(msg ilink.WeixinMessage) string {
	for _, item := range msg.ItemList {
		if item.Type == ilink.ItemTypeVoice {
			return "这条语音没能转成文字，Claude 没有收到。请重说一遍，或改发文字。"
		}
	}
	if len(msg.ItemList) == 0 {
		return ""
	}
	return "暂不支持这种消息（比如表情、位置、名片），Claude 没有收到。请改发文字、图片或文件。"
}

func sanitizeInboundName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == "/" || name == ".." {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == ':' {
			return '_'
		}
		return r
	}, name)
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		p := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
	}
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
