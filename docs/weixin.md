# Weixin (iLink)

`tmux-connect` can run over personal Weixin by using the Tencent iLink bot HTTP gateway.

This connector differs from WhatsApp and Telegram in two important ways:

- login is out of scope for `tmux-connect` v1; you must already have an iLink bearer token
- outbound messages require a valid per-user `context_token`, so the Weixin operator must message the bot first

## Example Config

```toml
[daemon]
platform = "weixin"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["weixin:user@im.wechat"]

[daemon.weixin]
token = "ilink-bearer-token"
base_url = "https://ilinkai.weixin.qq.com"
cdn_base_url = "https://novac2c.cdn.weixin.qq.com/c2c"
route_tag = ""
```

## Run

```bash
./tmux-connect daemon run \
  --platform weixin \
  --weixin-token "$TMUXCONN_WEIXIN_TOKEN" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat weixin:user@im.wechat
```

If your iLink provider requires a route tag:

```bash
./tmux-connect daemon run \
  --platform weixin \
  --weixin-token "$TMUXCONN_WEIXIN_TOKEN" \
  --weixin-route-tag "$TMUXCONN_WEIXIN_ROUTE_TAG" \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat weixin:user@im.wechat
```

## First Message Requirement

The Weixin operator must send one message to the bot before `tmux-connect` can reply.

Why this is required:

- each inbound iLink message carries a `context_token`
- replies must send that token back unchanged
- `tmux-connect` caches the latest `context_token` per Weixin user in its SQLite store

If no token is cached yet, replies fail with an error that tells you the user must message the bot first.

## Allowlist Format

Use platform-scoped allowlist entries:

```text
weixin:user@im.wechat
```

The `chat_id` is the iLink `from_user_id`. Use the full value exactly as delivered by the gateway.

## Current Scope

Weixin v1 supports:

- inbound text commands
- outbound text replies
- outbound images for `/snapshot`
- persisted iLink cursor and per-user `context_token` in the daemon SQLite store

Weixin v1 does not yet support:

- QR login or token binding CLI
- inbound media attachments into the router
- proactive sends to users who have never messaged the bot

## Notes

- only private user messages are handled
- the connector ignores bot-originated messages from iLink
- long polling uses `getupdates` with a persisted cursor to avoid replay after restart
- large text replies are split into chunks before sending

## Common Pitfalls

### `context_token missing`

This means the Weixin user has not sent an inbound message since the daemon started with the current SQLite store.

Fix:

1. start `tmux-connect daemon run`
2. send any message from the allowed Weixin account to the bot
3. retry the command

### `chat is not allowed to use this bot`

Your `--allow-chat` or `[daemon].allow_chats` entry does not match the actual iLink user ID.

Use the exact platform-scoped value:

```text
weixin:user@im.wechat
```

### Images fail to send

Check:

- `cdn_base_url` matches your iLink environment
- the token is still valid
- the operator has an active cached `context_token`
