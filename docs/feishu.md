# Feishu

`tmux-connect` can run as a Feishu bot over the official websocket event stream, so the first stage does not require a public callback URL.

## What v1 supports

- inbound bot messages over Feishu websocket long connection
- App ID + App Secret authentication
- private chats and group chats
- group commands only when the bot is mentioned
- plain-text relay in private chats
- `/snapshot` image delivery
- static cards for `/help` and pane-selection prompts
- follow mode as new text messages instead of in-place message updates

## What v1 does not support

- interactive card callbacks or buttons
- message editing for follow output
- webhook callback mode

## Config

```toml
[daemon]
platform = "feishu"
db = "/home/user/.tmux-connect/tmux-connect.db"
# optional allowlist; remove this line to allow any reachable private chat or group
allow_chats = ["feishu:oc_xxx"]

[daemon.feishu]
app_id = "cli_xxx"
app_secret = "secret_xxx"
bot_open_id = "ou_bot"
# optional fallback if you prefer other ID types:
# bot_user_id = "cli_xxx"
# bot_union_id = "on_xxx"
```

Or use environment variables:

```bash
export TMUXCONN_PLATFORM=feishu
export TMUXCONN_FEISHU_APP_ID=cli_xxx
export TMUXCONN_FEISHU_APP_SECRET=secret_xxx
export TMUXCONN_FEISHU_BOT_OPEN_ID=ou_bot
```

## Choosing `allow_chats` Values

`--allow-chat` and `[daemon].allow_chats` are optional. If you omit them, any
Feishu private chat or group that can reach the bot may use it.

- recommended format: `feishu:<chat_id>`
- use the exact Feishu `chat_id` delivered by inbound events; examples often
  look like `oc_xxx`
- this allowlist matches the daemon's chat identifier, not `bot_open_id`,
  `bot_user_id`, or `bot_union_id`
- if you are unsure, start once without an allowlist, send a test message, then
  inspect the latest `platform = feishu` row in `message_log` and copy the
  exact `chat_id`

## Run

```bash
./tmux-connect daemon run \
  --platform feishu \
  --feishu-app-id "$TMUXCONN_FEISHU_APP_ID" \
  --feishu-app-secret "$TMUXCONN_FEISHU_APP_SECRET" \
  --db ~/.tmux-connect/tmux-connect.db
```

## Chat behavior

- In private chats, plain text targets the current pane. With `--plain-text-mode execute`, plain text becomes input + Enter.
- In groups, only `@bot` commands are handled. Plain text without `@bot` is ignored.
- For precise group mention matching, set one of `bot_open_id`, `bot_user_id`, or `bot_union_id`. Otherwise `tmux-connect` falls back to treating any mention as a potential bot command for compatibility.
- Use `/send <text>` when the text itself starts with `/`.
- When no pane is selected, the bot replies with a static card listing available panes. Reply with a pane number like `1` or a pane id like `%5`.

## Commands

- `/help`
- `/panes`
- `/select <pane>`
- `/current`
- `/snapshot [lines] [image|text]`
- `/send <text>`
- `/enter [text]`
- `/keys <key...>`
- `/ctrlc`
- `/follow on [interval]|off`

## Permissions and event subscription

The Feishu app needs bot capability plus the event subscription for `im.message.receive_v1`. In groups, the exact events you receive still depend on the Feishu permissions you grant to the app.
