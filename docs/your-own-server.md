# Run your own server

## Containerized deployment (Docker/Podman)

Build and start:
```bash
$ docker compose up
```

### Enable HTTPS
In `compose.yaml`: set `CERT_DIR` to a persistent path, uncomment the `"443:443"` port.


## Deploy on your own server (manual)

Install [Go](https://go.dev/doc/install) on your host machine.  

Initialize server with folders and systemd service. Tested on Debian-based systems:
```bash
$ make init_server host=user@example.com salt=$(head -c 32 /dev/urandom | base64)
```

Configure the `/app/.env` file:
```
BOT_API_TOKEN=<TELEGRAM_API_TOKEN_IF_NEEDED>
STORAGE_DIR=/app/storage
CERT_DIR=/opt/files.md
TOKENS_DIR=/opt/files.md/tokens
LOG_FILE=server.log
API_URL=https://api.yourdomain.com
APP_URL=https://app.yourdomain.com
```

### Optional LLM assistant
LLM assistant is disabled unless you configure a provider on your server. The server calls an OpenAI-compatible chat completions API and keeps the provider key out of the browser.

Add placeholder values like these to `/app/.env`:
```
LLM_PROVIDER_BASE_URL=https://api.example.com/v1
LLM_MODEL=<MODEL_NAME>
LLM_API_KEY=<PROVIDER_API_KEY>

# Optional timeout and limits
LLM_TIMEOUT_SECONDS=30
LLM_MAX_BODY_BYTES=262144
LLM_MAX_PROMPT_CHARS=8000
LLM_MAX_CONTEXT_BLOCKS=8
LLM_MAX_CONTEXT_BYTES=131072
LLM_MAX_RESPONSE_BYTES=262144
LLM_MAX_OUTPUT_TOKENS=1024
LLM_USER_RATE_PER_MINUTE=10
LLM_IP_RATE_PER_MINUTE=60
LLM_USER_CONCURRENCY=2
LLM_USER_DAILY_QUOTA=100
```

Use your provider's real base URL, model name, and API key only in your private server environment. Prefer `https` provider URLs; use `http` only for an intentionally configured local or private gateway.

Privacy behavior:
- Ordinary chat capture still writes to local Markdown and does not call the LLM provider.
- Only content you explicitly select or request for an assistant action is sent to the configured provider.
- Files.md does not send whole folders, unrelated sync data, or your whole vault as background LLM context.
- The external provider's own retention policy applies to anything you choose to send.

Current LLM limitations:
- Responses are non-streaming.
- There is no whole-vault context or retrieval index.
- Model output is shown as a draft until you choose to copy, insert, append to `Chat.md`, or discard it.
- Files.md does not automatically mutate Markdown after an LLM response.
- Files.md does not automatically create a separate AI transcript file.

Deploy a systemd service:
```bash
$ make deploy_systemd host=<YOUR_SSH_HOST>
```

That's all :)  

## Run your own Telegram Bot
1) Install [Go](https://go.dev/doc/install)
2) Register new telegram bot via [@BotFather](https://t.me/BotFather)
3) Add `BOT_API_TOKEN=<YOUR_TELEGRAM_API_TOKEN>` line to `.env` file
4) Redeploy/relaunch the server

Bot's artifacts can be seen in `./storage/<USER_ID>` folder.  

## Linking a new device
1) Open telegram bot
2) Open `/app`
3) Open the link in your browser
4) Device is now linked

### Additional bot's settings
1) For search functionality, enable `Inline Mode` for your bot in [@BotFather](https://t.me/BotFather)
2) Press "Edit Commands", and send the following list:
```
chat - 🏠 Home
files - 📄 Files
dirs - 🗂 Dirs
checklists - ☑️ Checklists
schedule - 📆 Schedule
postpone - 🦥 Postpone
rename - ✏️ Rename
move - ➡️ Move
app - 🔗 Open in app
settings - ⚙️ Settings
help - 📕 Help
```

## Hosting the bot on you local computer
You can host the bot locally, because it doesn't expose any ports to the outside world (if you don't use habits functionality).  
It communicates with Telegram using pull API.

Create a symlink to your local folder with `.md` files for convenience:  
`ln -s <YOUR_EXISTING_DIR_WITH_MD_FILES> storage/<USER_ID>`

## Transfer files to another server

1) Backup your data (`/app/storage`)
2) Be sure that all client app fully synced with the server (bring the app in the focus)
3) Stop bot on old server, so no new files would be created.
4) Compress all the files on one server: `tar -czvf storage.tar.gz storage`
5) `scp` the file to your host machine: `scp SSH_HOST:/app/storage.tar.gz .`
6) `scp` the file to your target machine

Synchronization is relying on `mtime`, so after compressing/decompressing the flag wouldn't be lost.

1) `cd /opt/files.md`
2) `tar -czvf tokens.tar.gz tokens`
3) `scp` to same dir on target machine

We don't need to transfer fslog (renames), if we're certain that all clients read the log.

1) Extract all files on new server
2) Transfer `BOT_API_TOKEN`
3) Launch server
4) Execute `localStorage.setItem('ApiHost', 'YOUR_NEW_API_HOST');` in your PWA applications
5) Make sure that all files are available
6) Cleanup the oldserver

## Maintenance notes
Add this to your crontab (`crontab -e`) for daily git backups:
`0 0 * * * cd /app/storage/<YOUR_TELEGRAM_ID> && git add . && git commit -m "$(date +\%d.\%m.\%Y)"`

Execute `git init` in your folder before that, to init a git repository.

If you have non-ASCI character in filenames, disable quoting:
`git config --global core.quotePath false`

Systemd journal:  
`sudo journalctl -u filesmd`

Find forbidden character in filenames (can be executed in user's storage folder):
`find . -name '*[<>:"|\?*]*'`

Remove forbidden filename characters:
```bash
find . -type f -name '*[<>:"|\?*]*' -print0 | while IFS= read -r -d '' f; do
  dir=$(dirname "$f")
  base=$(basename "$f")
  newbase="${base//[<>:\"|\\?*]/}"
  [ "$base" != "$newbase" ] && [ -n "$newbase" ] && mv -n -- "$f" "$dir/$newbase"
done
```
