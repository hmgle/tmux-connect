# WhatsApp

`tmux-connect` can run over WhatsApp private chats by using [`go.mau.fi/whatsmeow`](https://pkg.go.dev/go.mau.fi/whatsmeow) as a local multi-device client.

This connector is different from the official Meta Cloud API:

- no public webhook is required
- first login happens by scanning a QR code in the terminal
- the daemon keeps a local WhatsApp device session database
- v1 supports private chats only, not group chats
- self-chat with the paired account is off by default; an experimental self-chat mode is available

## Example Config

```toml
[daemon]
platform = "whatsapp"
db = "/home/user/.tmux-connect/tmux-connect.db"
# optional allowlist; remove this line to allow any reachable private chat
allow_chats = ["whatsapp:8613800000000@s.whatsapp.net"]
follow_lines = 80
follow_min_interval = "2s"

[daemon.whatsapp]
session_db = "/home/user/.tmux-connect/whatsapp-device.db"
device_name = "tmux-connect"
auto_mark_read = true
allow_self_chat = false
```

## Run

```bash
./tmux-connect daemon run \
  --platform whatsapp \
  --whatsapp-session-db ~/.tmux-connect/whatsapp-device.db \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat whatsapp:8613800000000@s.whatsapp.net
```

On the first run, the daemon prints a QR code. Open WhatsApp on your phone, go to `Linked Devices`, and scan it.

## Account Model

The WhatsApp connector currently works like this:

- pair your main WhatsApp account as the local multi-device client
- by default, send commands to that paired account from a different WhatsApp account in a one-to-one chat
- optionally enable experimental self-chat mode; in that mode only self-chat messages sent from another linked device are accepted, and messages echoed back by the daemon itself are suppressed

In practice, this means the WhatsApp operator is usually a second account. If you only have one WhatsApp account, enable `allow_self_chat` or pass `--whatsapp-allow-self-chat`, then set `--allow-chat` to the paired account's own JID.

## Setup Patterns

### Default mode: second WhatsApp account

Use this when you have a separate operator account.

```bash
./tmux-connect daemon run \
  --platform whatsapp \
  --whatsapp-session-db ~/.tmux-connect/whatsapp-device.db \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat whatsapp:<operator-jid>
```

In this mode:

- pair your main account by scanning the QR code
- send commands from a different WhatsApp account
- plain text works normally

### Experimental self-chat mode: one account, multiple linked devices

Use this when you only have one WhatsApp account and want to send commands from another linked device of the same account.

```bash
./tmux-connect daemon run \
  --platform whatsapp \
  --whatsapp-session-db ~/.tmux-connect/whatsapp-device.db \
  --db ~/.tmux-connect/tmux-connect.db \
  --whatsapp-allow-self-chat \
  --allow-chat whatsapp:<your-own-chat-jid>
```

In this mode:

- `--allow-chat` must point to the paired account's own chat JID
- only self-chat messages sent from another linked device are accepted
- plain text is intentionally disabled to avoid reply loops
- use explicit slash commands such as `/panes`, `/select`, `/send <text>`, `/enter <text>`, `/keys <key...>`, or `/follow on`

## Flags

- `--platform whatsapp`
- `--whatsapp-session-db PATH`
- `--whatsapp-device-name NAME`
- `--whatsapp-auto-mark-read`
- `--whatsapp-allow-self-chat`
- `--allow-chat whatsapp:<jid>` for the remote operator account, or for the paired account itself when self-chat mode is enabled

`--allow-chat` and `[daemon].allow_chats` are optional. If you omit them, any
private chat that can reach the paired account may use the bot. In practice,
most operators should still set an explicit allowlist on WhatsApp because chat
IDs can differ between normal JIDs and `@lid` JIDs.

## Commands And Behavior

- by default, plain text targets the current pane, like Telegram private chat mode
- explicit commands still use slash commands such as `/panes`, `/select`, `/snapshot`, `/follow on`, or `/clear`
- when `/select` or `/unmanage` is sent without an argument, the bot replies with a numbered pane list; replying with `1` or `2` is supported
- when `/send`, `/keys`, or `/follow` is missing input, reply in the same chat with the missing value
- snapshot and follow output are formatted as monospace code blocks
- follow defaults to a more conservative `2s` minimum interval on WhatsApp to reduce message spam
- when self-chat mode is enabled, plain text is disabled to avoid reply loops; use explicit commands such as `/send <text>` or `/enter <text>`

## Notes

- only private chats are accepted; group messages are ignored
- in self-chat mode, only self-chat messages sent from another linked device are accepted; messages sent by the daemon itself are ignored
- allowlists should use full IDs such as `whatsapp:8613800000000@s.whatsapp.net`; use the remote operator account by default, or the paired account itself in self-chat mode
- the WhatsApp device session database is separate from the daemon SQLite state database
- `daemon doctor` validates the session DB path and reminds you that first login prints a QR code

## Common Pitfalls

### `chat is not allowed to use this bot`

This means the chat that sent the message doesn't match the effective allowlist.

Check these first:

- command-line flags override environment variables, and environment variables override `config.toml`
- if you don't pass `--allow-chat`, an existing `[daemon].allow_chats` entry from `config.toml` may still be active
- `--allow-chat` must exactly match the actual WhatsApp chat ID that the daemon receives

In self-chat mode, the most common mistake is allowing the phone-number JID while WhatsApp actually delivers the chat as a `@lid` JID.

For example, this may be wrong:

```text
whatsapp:8613800000000@s.whatsapp.net
```

while the real incoming chat ID is:

```text
whatsapp:158274578075808@lid
```

### How to find the real WhatsApp chat ID

If you're unsure what to put in `--allow-chat`, send a command once, then inspect the daemon SQLite log:

```bash
sqlite3 ~/.tmux-connect/tmux-connect.db \
  'select platform, chat_id, kind, body_preview, created_at from message_log order by id desc limit 10;'
```

Use the `chat_id` from the most recent `platform = whatsapp` row exactly as shown.

### Self-chat mode ignores bare text

This is expected. In self-chat mode, plain text is disabled to avoid reply loops caused by the daemon seeing its own replies.

Use:

- `/send ls`
- `/enter pwd`
- `/keys C-c`
- `/panes`

instead of sending bare text like `ls` or `pwd`.
