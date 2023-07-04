<img src="https://github.com/zakirullin/stuff-bot/raw/main/docs/go.svg" alt="Stuff Bot logo" title="Stuff Bot" align="right" height="60" />

# Stuff Bot
A telegram bot for your personal stuff.

[Tasks management bot showcase in Dorofeev's club](https://club.mnogosdelal.ru/post/180/)  
[Notes taking via bot](https://vas3k.club/post/18815/)

## Spin it up 🚀
0) Install [Go](https://go.dev/doc/install)
1) Register new telegram bot via [@BotFather](https://t.me/BotFather)
2) Copy your bot token to `.env` file (see `.env.example`)

```bash
$ make install && make run
```
or
```bash
$ go get ./..
$ go run ./cmd
```

Bot's artifacts can be seen in `cmd/testdata`

## How we work
- No long-lived branches except `main`
- Feature branches are [short-lived](https://trunkbaseddevelopment.com/short-lived-feature-branches/)
- **We commit often, so pull `main` every once in a while**
- Once your feature is ready, open a PR to `main`

How to start a feature branch:
```bash
$ git checkout main
$ git pull
$ git checkout -b feature/feature_name
```

## Bot's artifacts are plain files, yet we differentiate the following types:
- Tasks: `/today/pay the bills.md` (`today/*.md`, `later/*.md`, `_archive_/*.md`)
- Notes: `/brain/brain is the most complex object.md` (`/.*/*.md` also `/inbox/*`)
- Documents: `/my big project.md` (`/*.md`)
- Check list items: `/-shop-/cheese.md` (`-*-/*.md`)

## Glossary
- `filename` - a filename with extension, like "note.md" (USE THIS AS ID)
- `title` - an extension-stripped and capitalized filename, like "Note"
- `content` - note's content (body/text)
- `dir` - a dir that is meant to store notes under some category, like "happiness"
- `userID` - chatID. For the most part we're only using chatID as userID (PM with the bot)
- `ctime` - file's ownership, location, file type and permission settings changed time (parent folder rename won't affect). We need this to track file's location changes, like to understand when it was moved to _archive_

Any file can be uniquely identified by filename and dir. We only support one level of nesting.

## ADRs (Architecture Decision Records)
- Markdown to HTML conversion. User can have invalid Markdown in his notes, and TG API would fail to send invalid Markdown directly. So, we need to convert user's Markdown to HTML first and then send it via Telegram as HTML.
- File hashing. Everywhere where we have user input - we should use fs.hash, otherwise we get long filenames, and tg returns `INVALID_DATA` error (callbackData max 64 bytes)
- Introduced `db.go`. We had to abstract away Redis anyway (otherwise it's hard to write tests)
- Package db.go doesn't store userID (we often use it separately...) Do we?
- We can't ucfist filename in fs.Put - what if that was user-created file (outside the bot), i.e. it comes with lowercase

## Why schedule is stored at once
To lessen roundrip to redis (is it tangible at all?)
- in PHP get/set takes 1,2,9 ms sometimes. Connection included
- do we need to lock?

## TODO
- recreate checklists folder instead of coding source dir in name?

## Overarching design principles
- `Clarity`: The code’s purpose and rationale is clear to the reader.
- `Simplicity`: The code accomplishes its goal in the simplest way possible.
- `Concision`: The code is easy to discern the relevant details, and the naming and structure guide the reader through these details.  
- `Maintainability`: The code is easy for a future programmer to modify correctly.  
- `Consistency`: The code is consistent across the codebase  

Refer to [developer's handbook](https://github.com/zakirullin/cognitive-load) for more comprehensive guiding rules.

## Guidelines
- With portability in mind, everything is stored in **plain text files**
- We write **tests**
- We don't use get* prefix for methods
- We don't use panics, errors are part of business logic
- If we are ignoring an error - we leave a WHY comment
- We wrap errors all the time, we should add method's context
- We prefer fakes/real implementations over mocks and stubs
- Imports should only be renamed to avoid a name collision with other imports
- We use Is/Has prefixes for boolean variables