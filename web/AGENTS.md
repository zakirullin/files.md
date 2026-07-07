# Files.md - Files Structure

This document describes the file structure of a Files.md knowledge base.
All files are plain Markdown (.md) stored locally.

## Directory Layout

```
/
├── *.md                  # Projects, big notes
├── <category>/           # Note directories (brain/, happiness/, ...)
│   └── *.md              # Notes, e.g. brain/Memory.md
│
├── Chat.md               # Unsorted stuff and tasks
├── Later.md              # Postponed tasks (checklist format)
│
├── Read.md               # Reading list (checklist)
├── Watch.md              # Watch list (checklist)
├── Shop.md               # Shopping list (checklist)
│
├── journal/              # Daily journal
│   └── YYYY.MM Month.md  # e.g. 2024.08 August.md
├── habits/               # Habit definitions
│   └── *.md              # e.g. Morning workout.md
├── insights/             # Yearly summaries
│   └── YYYY Habits.md    # e.g. 2024 Habits.md
│
├── media/                # Images (png, jpg, webp, gif)
├── archive/              # Completed/archived items
│   └── Done.md           # Log of completed tasks
└── config.json           # User settings
```

## File Types

### Notes (/<category>/*.md)
One idea per file. The filename is the title. Files can link to each other using standard Markdown: `[title](file.md).
Relations among ideas are far more important than the ideas themselves.

### Chat (Chat.md)
Append-only chat log. New messages are appended with timestamps under daily headers.
```
#### 15 August, Friday
`10:30` First message of the day
`14:22` Another thought
```

### Tasks (Chat.md, Later.md)
Checklist format. Each task is a checklist item.
```
- [ ] Pay the bills
- [ ] Buy groceries
- [x] Send email
```

### Checklists (Read.md, Watch.md, Shop.md)
Same checklist format as tasks, for tracking reading/watching/shopping lists.

### Journal (journal/YYYY.MM Month.md)
Monthly files with daily entries. Each day has a header followed by timestamped records.
```
#### 15 August, Friday
`10:30` Morning walk
`14:00` Deep work session
```

### Habits (habits/*.md)
One file per habit. The filename is the habit name.

### Insights (insights/YYYY Habits.md)
Auto-generated yearly habit tracking summaries.

### Media (media/*)
Images referenced in notes via standard Markdown: `![](media/photo.jpg)`

## Conventions
- One level of directory nesting only
- Filenames are capitalized: `My note.md`, not `my note.md`
- Links between notes use default markdown syntax: `[Note Title](Note path)`
- Supported file extensions: .md, .png, .jpg, .jpeg, .webp, .gif
- System directories: media, archive, journal, habits, insights