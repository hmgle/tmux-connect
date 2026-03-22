package whatsapp

import (
	"context"
	"fmt"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func (c *Client) Run(ctx context.Context, autoMarkRead bool, handler func(context.Context, MessageEvent) error) error {
	if handler == nil {
		return fmt.Errorf("whatsapp handler is required")
	}

	c.client.AddEventHandler(func(evt interface{}) {
		switch event := evt.(type) {
		case *events.Message:
			msg, ok := c.parseMessageEvent(event)
			if !ok {
				return
			}
			go c.handleMessageEvent(ctx, autoMarkRead, msg, handler)
		}
	})

	if c.client.Store.ID == nil {
		qrChan, err := c.client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("prepare whatsapp qr channel: %w", err)
		}
		go c.consumeQRChannel(qrChan)
	}

	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("connect whatsapp client: %w", err)
	}
	_ = c.client.SendPresence(ctx, waTypes.PresenceAvailable)

	<-ctx.Done()
	c.client.Disconnect()
	return nil
}

func (c *Client) handleMessageEvent(ctx context.Context, autoMarkRead bool, msg MessageEvent, handler func(context.Context, MessageEvent) error) {
	if err := handler(ctx, msg); err != nil {
		fmt.Fprintf(c.stderr, "whatsapp message error: %v\n", err)
		return
	}
	if autoMarkRead {
		if err := c.markRead(ctx, msg); err != nil {
			fmt.Fprintf(c.stderr, "whatsapp mark read error: %v\n", err)
		}
	}
}

func (c *Client) consumeQRChannel(ch <-chan whatsmeow.QRChannelItem) {
	for evt := range ch {
		switch evt.Event {
		case "code":
			fmt.Fprintf(c.stderr, "whatsapp login required for %s; scan this QR code from Linked Devices:\n", c.deviceName)
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, c.stderr)
			fmt.Fprintln(c.stderr)
		case "success":
			fmt.Fprintf(c.stderr, "whatsapp pairing completed for %s\n", c.deviceName)
		default:
			fmt.Fprintf(c.stderr, "whatsapp login event: %s\n", evt.Event)
		}
	}
}

func (c *Client) markRead(ctx context.Context, msg MessageEvent) error {
	chatJID, err := waTypes.ParseJID(msg.ChatID)
	if err != nil {
		return err
	}
	sender := waTypes.EmptyJID
	if msg.IsGroup && msg.SenderID != "" {
		sender, err = waTypes.ParseJID(msg.SenderID)
		if err != nil {
			return err
		}
	}
	return c.client.MarkRead(ctx, []waTypes.MessageID{waTypes.MessageID(msg.MessageID)}, msg.Timestamp, chatJID, sender)
}
