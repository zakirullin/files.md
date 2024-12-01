<img src="https://github.com/zakirullin/files.md/raw/main/docs/go.svg" alt="Files.md Bot logo" title="Files.md Bot" align="right" height="60" />

# files.md

An application for your personal stuff:
- ✅ Tasks, Checklists
- 📝 Notes, Files
- 💚 Journal, Habits
- 🍅 Pomodoro

**Everything is stored as plain local `*.md` files.**  

When we are focused and distracting information comes in, we want to get rid of it as quickly as possible. To do that, just send whatever is distracting you to the bot. Then choose how you want to save it - as a task, a note, or a journal entry. By default, it will be saved as today's task.  

It works like a regular chat, so it's easier to use because there's less resistance. We're used to sending messages to friends, now we're going to send stuff to the bot.  

[files.md](https://files.md)  
[Tasks management via bot](https://club.mnogosdelal.ru/post/180/)  
[Notes taking via bot](https://vas3k.club/post/18815/)

## Repository structure
`/cmd/tgbot` - entrypoint for telegram bot (stable release)  
`/cmd/bot` - entrypoint for local standalone bot (beta version)  
`/internal` - bot's code (reused for both telegram/local bots)  
`/pkg` - various packages   
`/web` - standalone PWA application for viewing/editing files (alpha version)   

## Telegram Bot 🤖
1) Install [Go](https://go.dev/doc/install)
2) Register new telegram bot via [@BotFather](https://t.me/BotFather)
3) Copy your bot token to `.env` file (see `.env.example`)
4) Run the bot:
```bash
$ go run ./cmd/tgbot
```

Bot's artifacts can be seen in `./storage/<USER_ID>` folder

## Local Standalone Bot 🤖
1) Install [Go](https://go.dev/doc/install) and [Wails](https://wails.io/docs/gettingstarted/installation)
2) Run the bot:
```bash
$ make bot
```

## App 📝
[app.files.md](https://app.files.md), a standalone application for viewing/editing files, alpha version. Works offline.
1) Open `/web/app.html`

`cmd + k` for command palette.  
`cmd + [` to move back in history, `cmd + ]` to move forward.  
`cmd + enter` to hide/show sidebar.  
`[` to create a link.  
`ctrl + cmd + space` to show emoji dialog.  

## Storage file structure
We differentiate the following types of files (with `/` denoting your root folder):
- Tasks: `/today/Pay the bills.md` (`/today/*.md`, `/later/*.md`)
- Notes: `/brain/Brain is the most complex object.md` (`/*/*.md`)
- Files: `/My project.md` (`/*.md`)
- Checklists: `/_read_/How to Take Smart Notes.md` (`/_[read|watch|shop]_/*.md`)
- Journal: `/Journal/2024.08 August.md` (`/journal/<YEAR>.<MONTH> <MONTH NAME>.md`)
- Habits: `/habits/2 minute morning workout.md` (`/habits/*.md`)
- Insights: `/insights/2024 Habits.md` (`/insights/<YEAR> Habits.md`)
- Images: `/img/*`
- Pomodoro: `/today/Finished a break.md`
- Archive: `/archive/*`

## How we contribute
- No long-lived branches except `main`
- Feature branches are [short-lived](https://trunkbaseddevelopment.com/short-lived-feature-branches/)
- **We commit often, so pull `main` every once in a while**
- Once your feature is ready, open a PR to `main`

How to start a feature branch:
```bash
$ git checkout main
$ git pull
$ git checkout -b feature_name
```

## Glossary
- `filename` - a filename with extension, like "note.md" (USE THIS AS ID)
- `title` - an extension-stripped and capitalized filename, like "Note"
- `content` - note's content (body/text)
- `dir` - a dir that is meant to store notes under some category, like "happiness"
- `userID` - chatID. For the most part we're only using chatID as userID (PM with the bot)
- `ctime` for file - data blocks or metadata change time: file's ownership, location, file type and permission settings changed time.  Parent folder renaming won't affect, moving the file does affect, renaming the file does affect. We need this to track file's location changes, like to understand when it was moved to archive
- `ctime` for dir - adding or removing files or subdirectories (similar to `mtime`)

Any file can be uniquely identified by filename and dir. We only support one level of nesting.

## Performance
The app is  blazing fast :) If you're afraid of using files or mutexes unnecessarily for performance reasons, take a look at this:  
```
Mutex lock/unlock = 25 ns
Read 4K randomly from SSD = 150,000 ns
1 ms = 1,000,000 ns
```

## ADRs (Architecture Decision Records)
- Removed Wikilinks support. Only plain Markdown links, our knowledge base must be interoperable.
- Updates are now processed sequentially on per-user basis. Because there were some race conditions on concurrent file writings. Also we faced out-of-order forwarded messages processing, and it was impossible to collapse them to one message.
- **Removed fyne.io**. At first, I wanted a lightweight alternative to Electron, and fyne.io seemed to be an ideal candidate. After a few days working with it 80% of bot functionality was implemented, and I was pretty happy with it. The thing is, to implement the rest of the functionality, we would have to apply A TREMENDOUS amount of effort. I am talking tiny details such as scrolling, emojis rendering, text selecting behaviour, links support, etc. And in future we would have to implement image uploading and markdown/html renderer, which would be also painful in such non-webview based toolkit. As much as I hate using the web stack for the desktop applications, it doesn't seem like we have a choice. Let's try wails.io.
- We use vendoring for dependencies. We want all our few dependencies to be in the repo, so we don't care about blocked/removed dependencies. Our repository is the self-sufficient source of truth.
- We use granular locks (in db, journal, userconfig) instead of one global per user lock so to avoid bottlenecks. Workers might use 3rd party API like ChatGPT, and we don't want to hold user's lock all that time. **CHANGED**, we added sequential per-user updates processing, `bot` can't cause RCs on its own, but `bot` & `worker` can, so we should continue using granular locks.
- We read every userconfig value from the config file on every access. We don't need load/save whole config before/after `bot.Answer()` method. We have to reread it every time we need to change it, so we don't write back any stale data. Let's imagine we load config only once before `bot.Answer()`, next, we may have significant networking delays in `bot.Answer()` (let's say 2 seconds when making external requests), there are good changes that during those 2 seconds `worker.MoveDueTasks()` will modify `userconfig.Schedule`, causing data race (after bot's answer we write back stale data). And we don't want our schedule lost.
- Sanitize Early, we gave up sanitizing in Path method. That's an unexpected behaviour - it breaks paths. We should sanitize everything as soon as we received. Most commands work with md5 hashes, for such cases no sanitize is needed
- `gofumpt` for stricter formatting. `gofumpt` is happy with a subset of the formats that gofmt is happy with. The less we have to choose between different formating options, the better
- FS's structure should have userFS name, to reflect the fact it user user-namespaced
- Note term is way too vague. Let's try to use "file" term, without any high level abstraction (like note) 
- Gave up on AST parsing/rendering. We had lots of corner cases via AST and the code was way complex. Markdown isn't that hard to parse, we can do it via good old straigforward code. We have 3x times less code now, and it is far less mentally taxing to understand. We did the same for MD->HTML conversion. Telegram doesn't support whole range of HTML tags, so it was easier to write our own md-to-html converter.
- Adherence to Tolerant Reader principles. If enconunter gibberish during parsing - we skip it, but if we encounter flags of valid data (let's say `###`) but data itself is invalid - we panic. TODO preserve gibberish during read-write cycle.
- Usage of https://github.com/rivo/uniseg. In Go, strings are read-only slices of bytes. They can be turned into Unicode code points using the for loop or by casting: []rune(str). However, multiple code points may be combined into one user-perceived character or what the Unicode specification calls "grapheme cluster". For example, white circle "⚪" has two runes, but one grapheme cluster.
- Markdown to HTML conversion. User can have invalid Markdown in his notes, and TG API would fail to send invalid Markdown directly. So, first we escape HTML, then we convert user's Markdown to HTML and finally send it via Telegram API as HTML.
- File hashing. Everywhere where we have user input - we should use fs.hash, otherwise we get long filenames, and tg returns `INVALID_DATA` error (callbackData max 64 bytes)
- Introduced `db.go`. We had to abstract away Redis anyway (otherwise it's hard to write tests)
- Package db.go doesn't store userID (we often use it separately...) Do we? Maybe we gonna use it without userID (like global bot stats?)
- We can't ucfist filename in fs.Put - what if that was user-created file (outside the bot), i.e. it comes with lowercase

## Notes about Dropbox
- Symlink created on server will be synced on client as is (without resolving)
- To prevent symlinks attack our storage path should be mounted via `nosymfollow` flag

## Overarching design principles
- `Clarity`: The code’s purpose and rationale is clear to the reader.
- `Simplicity`: The code accomplishes its goal in the simplest way possible.
- `Concision`: The code is easy to discern the relevant details, and the naming and structure guide the reader through these details.  
- `Maintainability`: The code is easy for a future programmer to modify correctly.  
- `Consistency`: The code is consistent across the codebase  

Refer to [the following document](https://github.com/zakirullin/cognitive-load) for more comprehensive guiding rules.

## Guidelines
- We write **tests**
- eXtreme Programming and TDD are highly encouraged
- With portability in mind, everything is stored in **plain text files**
- We don't use get* prefix for methods
- No panics, errors are part of business logic
- If we are ignoring an error - we leave a WHY comment
- We wrap errors all the time, we should add method's context
- No iterators for client code
- We prefer real implementations or at least fakes over mocks and stubs
- Imports should only be renamed to avoid a name collision with other imports
