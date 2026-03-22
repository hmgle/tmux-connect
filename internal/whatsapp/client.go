package whatsapp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type MessageEvent struct {
	ChatID          string
	SenderID        string
	MessageID       string
	Text            string
	QuotedMessageID string
	QuotedSenderID  string
	Timestamp       time.Time
	IsFromMe        bool
	IsGroup         bool
}

type Client struct {
	client             *whatsmeow.Client
	stderr             io.Writer
	deviceName         string
	allowSelfChat      bool
	recentOutboundMu   sync.Mutex
	recentOutboundByID map[string]time.Time
}

const outboundSuppressionTTL = 10 * time.Minute

func NewClient(ctx context.Context, sessionDBPath string, deviceName string, allowSelfChat bool, stderr io.Writer) (*Client, error) {
	sessionDBPath = strings.TrimSpace(sessionDBPath)
	if sessionDBPath == "" {
		return nil, fmt.Errorf("whatsapp session db path is required")
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if strings.TrimSpace(deviceName) == "" {
		deviceName = "tmux-connect"
	}

	dbLog := waLog.Stdout("WhatsApp/DB", "WARN", true)
	container, err := sqlstore.New(ctx, "sqlite3", "file:"+sessionDBPath+"?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("open whatsapp session store %s: %w", sessionDBPath, err)
	}
	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, fmt.Errorf("get whatsapp device store: %w", err)
	}
	clientLog := waLog.Stdout("WhatsApp", "WARN", true)
	cli := whatsmeow.NewClient(deviceStore, clientLog)
	cli.SetForceActiveDeliveryReceipts(true)

	return &Client{
		client:             cli,
		stderr:             stderr,
		deviceName:         deviceName,
		allowSelfChat:      allowSelfChat,
		recentOutboundByID: make(map[string]time.Time),
	}, nil
}

func (c *Client) Close() error {
	if c.client == nil {
		return nil
	}
	c.client.Disconnect()
	return nil
}
