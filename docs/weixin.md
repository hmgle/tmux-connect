# Weixin (iLink)

`tmux-connect` can run over personal Weixin by using the Tencent iLink bot HTTP gateway.

This connector differs from WhatsApp and Telegram in two important ways:

- setup is a config-time flow: use `tmux-connect daemon weixin setup|bind`, then run the daemon separately
- outbound messages still require a valid per-user `context_token`, so the Weixin operator must message the bot first

## Recommended Setup

QR login:

```bash
tmux-connect daemon weixin setup
```

Existing token bind:

```bash
tmux-connect daemon weixin bind --token '<your-ilink-bearer-token>'
```

What this command does:

- writes `[daemon].platform = "weixin"`
- writes `[daemon.weixin].token`
- writes `base_url`, `cdn_base_url`, and optional `route_tag`
- if `[daemon].allow_chats` is empty and QR login returns the scanned Weixin user, fills it with `weixin:<user@im.wechat>`

Force QR mode even when a token is present:

```bash
tmux-connect daemon weixin new
```

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

- QR login and token bind CLI through `tmux-connect daemon weixin`
- inbound text commands
- outbound text replies
- outbound images for `/snapshot`
- persisted iLink cursor and per-user `context_token` in the daemon SQLite store

Weixin v1 does not yet support:

- inbound media attachments into the router
- proactive sends to users who have never messaged the bot

## Notes

- only private user messages are handled
- the connector ignores bot-originated messages from iLink
- long polling uses `getupdates` with a persisted cursor to avoid replay after restart
- large text replies are split into chunks before sending

## Setup Command Flags

- `--token` bind an existing bearer token
- `--api-url` override the iLink API base URL
- `--cdn-url` set the CDN base URL to save in config
- `--timeout` QR wait timeout, default `8m`
- `--route-tag` optional `SKRouteTag`
- `--bot-type` QR `bot_type`, default `3`
- `--set-allow-chat-empty` auto-fill `[daemon].allow_chats` when empty
- `--skip-verify` skip token verification in bind mode

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
