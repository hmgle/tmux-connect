# WhatsApp

`tmux-connect` can run over WhatsApp private chats by using [`go.mau.fi/whatsmeow`](https://pkg.go.dev/go.mau.fi/whatsmeow) as a local multi-device client.

This connector is different from the official Meta Cloud API:

- no public webhook is required
- first login happens by scanning a QR code in the terminal
- the daemon keeps a local WhatsApp device session database
- v1 supports private chats only, not group chats

## Example Config

```toml
[daemon]
platform = "whatsapp"
db = "/home/user/.tmux-connect/tmux-connect.db"
allow_chats = ["whatsapp:8613800000000@s.whatsapp.net"]
follow_lines = 80
follow_min_interval = "2s"

[daemon.whatsapp]
session_db = "/home/user/.tmux-connect/whatsapp-device.db"
device_name = "tmux-connect"
auto_mark_read = true
```

## Run

```bash
go run ./cmd/tmux-connect daemon run \
  --platform whatsapp \
  --whatsapp-session-db ~/.tmux-connect/whatsapp-device.db \
  --db ~/.tmux-connect/tmux-connect.db \
  --allow-chat whatsapp:8613800000000@s.whatsapp.net
```

On the first run, the daemon prints a QR code. Open WhatsApp on your phone, go to `Linked Devices`, and scan it.

## Flags

- `--platform whatsapp`
- `--whatsapp-session-db PATH`
- `--whatsapp-device-name NAME`
- `--whatsapp-auto-mark-read`
- `--allow-chat whatsapp:<jid>`

## Commands And Behavior

- plain text targets the current pane, like Telegram private chat mode
- explicit commands still use slash commands such as `/panes`, `/select`, `/snapshot`, `/follow on`, or `/clear`
- when `/select` or `/unmanage` is sent without an argument, the bot replies with a numbered pane list; replying with `1` or `2` is supported
- when `/send`, `/keys`, or `/follow` is missing input, reply in the same chat with the missing value
- snapshot and follow output are formatted as monospace code blocks
- follow defaults to a more conservative `2s` minimum interval on WhatsApp to reduce message spam

## Notes

- only private chats are accepted; group messages are ignored
- allowlists should use full IDs such as `whatsapp:8613800000000@s.whatsapp.net`
- the WhatsApp device session database is separate from the daemon SQLite state database
- `daemon doctor` validates the session DB path and reminds you that first login prints a QR code
