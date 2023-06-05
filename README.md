![Gopher](https://github.com/zakirullin/stuff-bot/raw/main/assets/_img_/gopherv3.png)

### Spin it up 🌱
```bash
make install && make run
```

### Overarching principles
- Clarity: The code’s purpose and rationale is clear to the reader.
- Simplicity: The code accomplishes its goal in the simplest way possible.
- Concision: The code has a high signal-to-noise ratio.
- Maintainability: The code is written such that it can be easily maintained.
- Consistency: The code is consistent across the database  
Refer to [developer's handbook](https://github.com/zakirullin/cognitive-load) for more comprehensive guiding rules.

### Decisions
- Write tests!
- Don't use get* prefix for methods
- Panics are part of business logic, don't use them
- If you're ignoring an error - leave a WHY comment
- Wrap errors all the time, we should add method's context
- Prefer fakes/real implementations over mocks and stubs
- Imports should only be renamed to avoid a name collision with other imports

### Glossary
- filename - a filename with extension, like "note.md" (USE THIS AS ID)
- title - an extension-stripped and capitalized filename, like "Note"
- content - note's content (body/text)
- dir - a dir that is meant to store notes under some category, like "happiness"
- userID - chatID. For the most part we're only using chatID as userID (PM with the bot)
- mtime - modification time (content only, renaming doesn't affect)
- ctime - change time (parent folder rename won't affect)
- atime - access time

### ADRs (Architecture Decision Records)
- everywhere where we have user input - we should use fs.hash, otherwise we get long filenames, and tg returns INVALID_DATA error (callbackData max 64 bytes)
- introduced db.go. We had to abstract away Redis anyway (otherwise it's hard to write tests)
- db.go doesn't store userID (we often use it separately...) Do we?
- we can't ucfist filename in fs.Put - what if that was user-created file (outside the bot), i.e. it comes with lowercase

### Why schedule is stored at once
To lessen roundrip to redis (is it tangible at all?)
- in PHP get/set takes 1,2,9 ms sometimes. Connection included
- do we need to lock?

### TODO 
- recreate checklists folder instead of coding source dir in name?