// Package feishu implements the Feishu/Lark channel using native HTTP + WebSocket.
// Supports: DM + Group, WebSocket + Webhook, mentions, media, streaming cards.
// Default domain: Lark Global (open.larksuite.com).
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/channels"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	defaultTextChunkLimit = 4000
	defaultMediaMaxMB     = 30
	defaultWebhookPort    = 3000
	defaultWebhookPath    = "/feishu/events"
	senderCacheTTL        = 10 * time.Minute
	pairingDebounceTime   = 60 * time.Second
)

// Channel connects to Feishu/Lark via native HTTP + WebSocket.
type Channel struct {
	*channels.BaseChannel
	cfg             config.FeishuConfig
	client          *LarkClient
	botOpenID       string
	pairingService  store.PairingStore
	senderCache     sync.Map // open_id → *senderCacheEntry
	dedup           sync.Map // message_id → struct{}
	pairingDebounce sync.Map // senderID → time.Time
	groupAllowList  []string
	groupHistory    *channels.PendingHistory
	historyLimit    int
	stopCh          chan struct{}
	httpServer      *http.Server
	wsClient        *WSClient
}

type senderCacheEntry struct {
	name      string
	expiresAt time.Time
}

// New creates a new Feishu/Lark channel.
func New(cfg config.FeishuConfig, msgBus *bus.MessageBus, pairingSvc store.PairingStore) (*Channel, error) {
	if cfg.AppID == "" || cfg.AppSecret == "" {
		return nil, fmt.Errorf("feishu app_id and app_secret are required")
	}

	// Resolve domain
	domain := resolveDomain(cfg.Domain)

	client := NewLarkClient(cfg.AppID, cfg.AppSecret, domain)

	base := channels.NewBaseChannel("feishu", msgBus, cfg.AllowFrom)
	base.ValidatePolicy(cfg.DMPolicy, cfg.GroupPolicy)

	historyLimit := cfg.HistoryLimit
	if historyLimit == 0 {
		historyLimit = channels.DefaultGroupHistoryLimit
	}

	return &Channel{
		BaseChannel:    base,
		cfg:            cfg,
		client:         client,
		pairingService: pairingSvc,
		groupAllowList: cfg.GroupAllowFrom,
		groupHistory:   channels.NewPendingHistory(),
		historyLimit:   historyLimit,
		stopCh:         make(chan struct{}),
	}, nil
}

// Start begins receiving Feishu events via WebSocket or Webhook.
func (c *Channel) Start(ctx context.Context) error {
	slog.Info("starting feishu/lark bot")

	// Probe bot identity
	if err := c.probeBotInfo(ctx); err != nil {
		slog.Warn("feishu bot probe failed (will continue)", "error", err)
	} else {
		slog.Info("feishu bot connected", "bot_open_id", c.botOpenID)
	}

	mode := c.cfg.ConnectionMode
	if mode == "" {
		mode = "websocket"
	}

	c.SetRunning(true)

	switch mode {
	case "webhook":
		return c.startWebhook(ctx)
	default: // "websocket"
		return c.startWebSocket(ctx)
	}
}

// Stop shuts down the Feishu channel.
func (c *Channel) Stop(_ context.Context) error {
	slog.Info("stopping feishu/lark bot")
	close(c.stopCh)

	if c.wsClient != nil {
		c.wsClient.Stop()
	}

	if c.httpServer != nil {
		c.httpServer.Close()
	}

	c.SetRunning(false)
	return nil
}

// Send delivers an outbound message to a Feishu chat.
func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("feishu bot not running")
	}

	chatID := msg.ChatID
	if chatID == "" {
		return fmt.Errorf("empty chat ID for feishu send")
	}

	// Determine receive_id_type
	receiveIDType := resolveReceiveIDType(chatID)

	// Send text content
	text := msg.Content
	if text != "" {
		// Resolve render mode
		renderMode := c.cfg.RenderMode
		if renderMode == "" {
			renderMode = "auto"
		}

		useCard := false
		switch renderMode {
		case "card":
			useCard = true
		case "auto":
			useCard = shouldUseCard(text)
		}

		chunkLimit := c.cfg.TextChunkLimit
		if chunkLimit <= 0 {
			chunkLimit = defaultTextChunkLimit
		}

		if useCard {
			if err := c.sendMarkdownCard(ctx, chatID, receiveIDType, text, nil); err != nil {
				return err
			}
		} else {
			if err := c.sendChunkedText(ctx, chatID, receiveIDType, text, chunkLimit); err != nil {
				return err
			}
		}
	}

	// Send media attachments
	for _, media := range msg.Media {
		if err := c.sendMediaAttachment(ctx, chatID, receiveIDType, media); err != nil {
			slog.Warn("feishu send media failed", "url", media.URL, "error", err)
		}
	}

	return nil
}

// --- Connection modes ---

// wsEventAdapter adapts Channel's event handling to the WSEventHandler interface.
type wsEventAdapter struct {
	ch *Channel
}

func (a *wsEventAdapter) HandleEvent(ctx context.Context, payload []byte) error {
	var event MessageEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		slog.Debug("feishu ws: parse event failed", "error", err)
		return nil
	}
	if event.Header.EventType == "im.message.receive_v1" {
		a.ch.handleMessageEvent(ctx, &event)
	}
	return nil
}

func (c *Channel) startWebSocket(ctx context.Context) error {
	slog.Info("feishu: starting WebSocket connection")

	domain := resolveDomain(c.cfg.Domain)
	c.wsClient = NewWSClient(c.cfg.AppID, c.cfg.AppSecret, domain, &wsEventAdapter{ch: c})

	go func() {
		if err := c.wsClient.Start(ctx); err != nil {
			slog.Error("feishu websocket error", "error", err)
		}
	}()

	slog.Info("feishu WebSocket client started")
	return nil
}

func (c *Channel) startWebhook(ctx context.Context) error {
	port := c.cfg.WebhookPort
	if port <= 0 {
		port = defaultWebhookPort
	}
	path := c.cfg.WebhookPath
	if path == "" {
		path = defaultWebhookPath
	}

	slog.Info("feishu: starting Webhook server", "port", port, "path", path)

	handler := NewWebhookHandler(c.cfg.VerificationToken, c.cfg.EncryptKey, func(event *MessageEvent) {
		c.handleMessageEvent(context.Background(), event)
	})

	mux := http.NewServeMux()
	mux.HandleFunc(path, handler)

	c.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("feishu webhook server error", "error", err)
		}
	}()

	slog.Info("feishu Webhook server listening", "port", port)
	return nil
}

// --- Bot probe ---

func (c *Channel) probeBotInfo(ctx context.Context) error {
	openID, err := c.client.GetBotInfo(ctx)
	if err != nil {
		return fmt.Errorf("fetch bot info: %w", err)
	}
	if openID == "" {
		return fmt.Errorf("bot open_id is empty")
	}
	c.botOpenID = openID
	return nil
}

// --- Send helpers ---

func (c *Channel) sendChunkedText(ctx context.Context, chatID, receiveIDType, text string, chunkLimit int) error {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > chunkLimit {
			cutAt := chunkLimit
			if idx := strings.LastIndex(text[:chunkLimit], "\n"); idx > chunkLimit/2 {
				cutAt = idx + 1
			}
			chunk = text[:cutAt]
			text = text[cutAt:]
		} else {
			text = ""
		}

		if err := c.sendText(ctx, chatID, receiveIDType, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) sendText(ctx context.Context, chatID, receiveIDType, text string) error {
	content := buildPostContent(text)

	_, err := c.client.SendMessage(ctx, receiveIDType, chatID, "post", content)
	if err != nil {
		return fmt.Errorf("feishu send text: %w", err)
	}
	return nil
}

func (c *Channel) sendMarkdownCard(ctx context.Context, chatID, receiveIDType, text string, metadata map[string]string) error {
	card := buildMarkdownCard(text)
	cardJSON, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal card: %w", err)
	}

	_, err = c.client.SendMessage(ctx, receiveIDType, chatID, "interactive", string(cardJSON))
	if err != nil {
		return fmt.Errorf("feishu send card: %w", err)
	}
	return nil
}

// --- Domain resolution ---

func resolveDomain(domain string) string {
	switch domain {
	case "feishu":
		return "https://open.feishu.cn"
	case "", "lark":
		return "https://open.larksuite.com"
	default:
		if !strings.HasPrefix(domain, "http") {
			return "https://" + domain
		}
		return domain
	}
}

func resolveReceiveIDType(id string) string {
	if strings.HasPrefix(id, "oc_") {
		return "chat_id"
	}
	if strings.HasPrefix(id, "ou_") {
		return "open_id"
	}
	if strings.HasPrefix(id, "on_") {
		return "union_id"
	}
	return "chat_id"
}

// --- Content builders ---

func buildPostContent(text string) string {
	content := map[string]interface{}{
		"zh_cn": map[string]interface{}{
			"content": [][]map[string]interface{}{
				{
					{
						"tag":  "md",
						"text": text,
					},
				},
			},
		},
	}
	data, _ := json.Marshal(content)
	return string(data)
}

func buildMarkdownCard(text string) map[string]interface{} {
	return map[string]interface{}{
		"schema": "2.0",
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"body": map[string]interface{}{
			"elements": []map[string]interface{}{
				{
					"tag":     "markdown",
					"content": text,
				},
			},
		},
	}
}

// shouldUseCard detects if content benefits from card rendering (code blocks, tables).
func shouldUseCard(text string) bool {
	return strings.Contains(text, "```") ||
		strings.Contains(text, "| --- ") ||
		strings.Contains(text, "|---|")
}

// isDuplicate returns true if messageID was already processed.
func (c *Channel) isDuplicate(messageID string) bool {
	_, loaded := c.dedup.LoadOrStore(messageID, struct{}{})
	if !loaded {
		go func() {
			time.Sleep(5 * time.Minute)
			c.dedup.Delete(messageID)
		}()
	}
	return loaded
}

// Ensure Channel implements the channels.Channel interface at compile time.
var _ channels.Channel = (*Channel)(nil)
