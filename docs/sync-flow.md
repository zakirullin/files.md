# Sync flow

How `openFile`, `syncCurrentEditor`, `syncFilesWithServer`, and `syncLocalFileWithServer` hand off work, and how the server knows what to send or receive.

## Triggers and who calls whom

```mermaid
flowchart TD
    U[User: sidebar click, link click, popstate] -->|openFile| OF[openFile]
    Timer1[setInterval 1000ms saver] -->|syncCurrentEditor| SCE[syncCurrentEditor]
    Focus[focusin / focus event] -->|syncCurrentEditor| SCE
    Timer2[setInterval syncFilesWithServer + syncMediaFiles] -->|syncFilesWithServer| STS[syncFilesWithServer]

    OF -->|save previous editor before swapping| SCE
    SCE -->|switchAwayEditor=false: end of fn| SLF[syncLocalFileWithServer]
    STS -->|per-file path| POST1[POST /syncFile per file]
    SLF --> POST2[POST /syncFile]

    STS -->|batch modified/deleted| POST3[POST /syncFilenames]
```

The red node is the drift-seal line (files.js:1028). If `currentEditor` was rotated by anything during the yellow await above, this write lands on the wrong editor instance.

## syncCurrentEditor - both flag branches

```mermaid
flowchart TD
    Enter([syncCurrentEditor switchAwayEditor]) --> G1{files undef, debug,<br/>or path undefined?}
    G1 -->|yes| Ret1([return])
    G1 -->|no| G2{isSaving OR<br/>isMessingWithCurrentEditor?}
    G2 -->|yes| Ret2([return])
    G2 -->|no| SetFlag[isMessingWithCurrentEditor = true;<br/>path = currentEditor.path]

    SetFlag --> IsInbox{path == INBOX_PATH?}
    IsInbox -->|yes| InboxBranch[chat sync logic]
    InboxBranch --> Ret3([return])

    IsInbox -->|no| RenameCheck{firstLine's filename<br/>!= current filename?}
    RenameCheck -->|yes| Rename[await remove path<br/>await getFileHandle newPath<br/>await writeIfContentIsDifferent<br/>await renderSidebar]
    Rename --> Ret4([return])
    RenameCheck -->|no| DiffCheck[await isContentEqual<br/>path, getCurrentContent]

    DiffCheck --> SameGuard{isCurrentEditorSame?}
    SameGuard -->|no| Ret5([return])
    SameGuard -->|yes| ModCheck{contentWasModifiedLocally<br/>AND editor.isClean?}

    ModCheck -->|yes| SwitchGate{switchAwayEditor?}
    SwitchGate -->|true: skip reload| Clear[isMessingWithCurrentEditor = false]
    SwitchGate -->|false: reload from disk| Reload[await openFile path, false]
    Reload --> Clear

    ModCheck -->|editor dirty| Save[write editor content to disk;<br/>editor.markClean]
    Save --> Clear
    ModCheck -->|neither| Clear

    Clear --> ServerGate{switchAwayEditor?}
    ServerGate -->|true: skip server sync| Done([return])
    ServerGate -->|false| PushServer[await syncLocalFileWithServer path]
    PushServer --> Done

    style Rename fill:#faa,stroke:#900,color:#000
    style Reload fill:#fca,stroke:#c80,color:#000
    style SwitchGate fill:#cfc,stroke:#070,color:#000
    style ServerGate fill:#cfc,stroke:#070,color:#000
```

The two green gates are the `switchAwayEditor` branches. The orange `Reload` is the one we neutralised (it used to recurse into `openFile` without an `el` arg and clobber the main editor). The red `Rename` block is the executioner that actually deletes and creates files on disk - still live, fires whenever first-line header disagrees with filename.

## Sync with the server - batch vs per-file

```mermaid
sequenceDiagram
    participant Client
    participant Server

    Note over Client: syncFilesWithServer fires
    Client->>Client: collect modified and deleted files (skip editor and editor2 paths)
    Client->>Server: POST /syncFilenames with modified, deleted, timestamps
    Server-->>Client: files, timestamps, renames
    Client->>Client: write non-current files to disk and update server.files snapshot
    Client->>Client: advance per-dir timestamp pointers

    Note over Client: syncCurrentEditor finishes the switchAwayEditor=false branch
    Client->>Client: syncLocalFileWithServer for the active editor
    Client->>Server: POST /syncFile with path, lastModified, clientLastModified, clientLastSynced, content
    alt notModified
        Server-->>Client: notModified
        Client->>Client: advance lastClientModified only
    else updatedOnServer
        Server-->>Client: updatedOnServer with new lastModified
        Client->>Client: record the server lastModified, no disk write
    else merged or ok
        Server-->>Client: content and lastModified
        Client->>Client: writeIfContentIsDifferent, then openFile if path matches editor.path
    end
```

### How the server knows there's something to sync

Two mechanisms, running in parallel:

1. **Batch: `syncFilesWithServer` → `POST /syncFilenames`.** The client sends:
   - `modified`: files whose disk `lastModified` is newer than the `lastClientSynced` pointer recorded in `server.files` for that path.
   - `deleted`: files present in the client's `server.files` snapshot but no longer on disk.
   - `timestamps`: a per-directory pointer telling the server "everything I've seen up to here." The server replies with files newer than each directory's pointer. **The two currently-open editor files are skipped on both send and receive** (`files.js:230` and `files.js:577`) - they're handled by the per-file path instead, to avoid racing with the user's active edits.

2. **Per-file: `syncLocalFileWithServer` → `POST /syncFile`.** Called at the end of each `syncCurrentEditor` (when `switchAwayEditor=false`). Sends the single file's content plus its `lastModified` + `clientLastModified` + `clientLastSynced`. The server compares timestamps and responds with one of four statuses that the client maps to either "advance pointers only" or "write this content to disk."

The client's `server.files` object holds the triple `(content, lastModified, lastClientModified)` per path - this is the client's view of what the server thinks the world looks like, and the basis for deciding which files to include in the next `modified`/`deleted` lists. Persisted to `localStorage` under `SERVER_STORAGE_KEY`.

### Auth gate: `lastServerOk`

The auth token lives in an HttpOnly cookie, so JS can't see it directly. Instead, every successful response from the server stamps `localStorage.lastServerOk` with `Date.now()` via `markServerOk()` (files.js). `hasLastServerOk()` returns true if that key exists - which it only can if the server has previously accepted us. Use this as the gate before kicking off sync work: no stamp ⇒ no token ⇒ skip the request entirely. The flag is set in:

- `app.js` after the `/issuePermanentToken` exchange returns 200
- `post()` after a 2xx response (covers all `/syncFilenames`, `/syncFile`, `/syncMediaFilenames`, `/syncMediaFile` upload calls now that they go through this helper)
- `syncMediaFiles` directly after the raw `POST /syncMediaFile` download (binary blob, can't share `post()`)

If the server later 401s, the stamp stays - but the request will simply fail and no sync state advances, so we don't need to clear it.

## File deletion propogation across clients

A delete on one device has to travel through the server and reach every other device that still holds the file. The mechanism is an append-only `fslog` on disk: every server-side `userFS.Del` writes a `<ts> del <abs-path>` row, and every `/syncFilenames` response carries the deletes a given client hasn't seen yet.

### Why we need this log at all

Without it, the server only knows what currently exists on disk - it has no memory of what *used* to exist. Sync responses only list present files. So Client B, which still holds the deleted file locally, would see "this path is on my disk but not in the server response" and conclude it's a *new* local file → it would re-upload `foo.md` and the file resurrects. The fslog gives the server a memory of deletions, so it can tell B "yes, this used to exist, but it was deleted at time T - drop your stale copy."

```mermaid
sequenceDiagram
    autonumber
    participant A as Client A
    participant S as Server
    participant L as fslog<br/>(append-only file)
    participant B as Client B

    Note over A,B: Steady state: both clients hold foo.md locally,<br/>server has foo.md on disk

    rect rgb(245, 240, 230)
        Note over A: User deletes foo.md in the PWA
        A->>A: moveFile("/foo.md", "/archive/foo.md")<br/>(local FS only)
        A->>S: POST /syncFilenames<br/>{ deleted: ["/foo.md"],<br/>  modified: [{path:"/archive/foo.md", ...}],<br/>  serverTime: <A's cursor> }
        S->>S: userFS.Del("foo.md")<br/>removes from disk
        S->>L: append "<now> del /app/storage/<uid>/foo.md"
        S->>S: deletes = DeletesLog(uid, req.serverTime+1)<br/>→ {"foo.md": <now>}
        S->>S: suppress echo: drop entries that<br/>match request.Deleted
        S->>S: write /archive/foo.md
        S-->>A: response.deleted = {} (A's own delete<br/>was filtered)<br/>response.files = [...]
    end

    Note over A,B: ...time passes, B opens app or hits sync interval...

    rect rgb(230, 240, 245)
        B->>S: POST /syncFilenames<br/>{ deleted: [], modified: [...],<br/>  serverTime: <B's cursor, before A's delete> }
        S->>L: scan fslog for this user
        S->>S: deletes = DeletesLog(uid, req.serverTime+1)<br/>→ {"foo.md": <ts of A's delete>}<br/>(no suppression: B didn't delete it)
        S-->>B: response.deleted = {"foo.md": <ts>}<br/>response.files = [archive/foo.md, ...]
        B->>B: for each (path, deletedAt) in response.deleted:<br/>local = getMemFile(path)<br/>if local && local.lastModified ≤ deletedAt:<br/>  await remove(path)#59; removeServerFile(path)
        B->>B: write archive/foo.md from response.files
    end

    Note over A,B: Both clients converged:<br/>foo.md gone, archive/foo.md present
```
