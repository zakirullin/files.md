// CodeMirror, copyright (c) by laobubu
// Distributed under an MIT license: http://codemirror.net/LICENSE
//
// This is a patch to markdown mode. Supports lots of new features
//
var __assign = (this && this.__assign) || Object.assign || function(t) {
    for (var s, i = 1, n = arguments.length; i < n; i++) {
        s = arguments[i];
        for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p))
            t[p] = s[p];
    }
    return t;
};

(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("codemirror/mode/markdown/markdown"), require("./theme-flexoki.css")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","codemirror/mode/markdown/markdown","./theme-flexoki.css"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.Mode = this.HyperMD.Mode || {}), CodeMirror);
})(function (require, exports, CodeMirror) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });
    var _a;
    /**
     * Markdown Extension Tokens
     *
     * - `$` Maybe a LaTeX
     * - `|` Maybe a Table Col Separator
     */
    // PATCHED
    // var tokenBreakRE = /[^\\][$|]/;
    var tokenBreakRE = /[^\\][$]/;
    var listRE = /^(?:[*\-+]|^[0-9]+([.)]))\s+/;
    var urlRE = /^((?:(?:aaas?|about|acap|adiumxtra|af[ps]|aim|apt|attachment|aw|beshare|bitcoin|bolo|callto|cap|chrome(?:-extension)?|cid|coap|com-eventbrite-attendee|content|crid|cvs|data|dav|dict|dlna-(?:playcontainer|playsingle)|dns|doi|dtn|dvb|ed2k|facetime|feed|file|finger|fish|ftp|geo|gg|git|gizmoproject|go|gopher|gtalk|h323|hcp|https?|iax|icap|icon|im|imap|info|ipn|ipp|irc[6s]?|iris(?:\.beep|\.lwz|\.xpc|\.xpcs)?|itms|jar|javascript|jms|keyparc|lastfm|ldaps?|magnet|mailto|maps|market|message|mid|mms|ms-help|msnim|msrps?|mtqp|mumble|mupdate|mvn|news|nfs|nih?|nntp|notes|oid|opaquelocktoken|palm|paparazzi|platform|pop|pres|proxy|psyc|query|res(?:ource)?|rmi|rsync|rtmp|rtsp|secondlife|service|session|sftp|sgn|shttp|sieve|sips?|skype|sm[bs]|snmp|soap\.beeps?|soldat|spotify|ssh|steam|svn|tag|teamspeak|tel(?:net)?|tftp|things|thismessage|tip|tn3270|tv|udp|unreal|urn|ut2004|vemmi|ventrilo|view-source|webcal|wss?|wtai|wyciwyg|xcon(?:-userid)?|xfire|xmlrpc\.beeps?|xmpp|xri|ymsgr|z39\.50[rs]?):(?:\/{1,3}|[a-z0-9%])|www\d{0,3}[.]|[a-z0-9.\-]+[.][a-z]{2,4}\/)(?:[^\s()<>]|\([^\s()<>]*\))+(?:\([^\s()<>]*\)|[^\s`*!()\[\]{};:'".,<>?«»“”‘’]))/i; // from CodeMirror/mode/gfm
    var emailRE = /^(([^<>()[\]\\.,;:\s@\"]+(\.[^<>()[\]\\.,;:\s@\"]+)*)|(\".+\"))@((\[[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\])|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$/;
    var url2RE = /^\.{0,2}\/[^\>\s]+/;
    var hashtagRE = /^(?:[-()/a-zA-Z0-9ァ-ヺー-ヾｦ-ﾟｰ０-９Ａ-Ｚａ-ｚぁ-ゖ゙-ゞー々ぁ-んァ-ヾ一-\u9FEF㐀-䶵﨎﨏﨑﨓﨔﨟﨡﨣﨤﨧-﨩]|[\ud840-\ud868][\udc00-\udfff]|\ud869[\udc00-\uded6\udf00-\udfff]|[\ud86a-\ud86c][\udc00-\udfff]|\ud86d[\udc00-\udf34\udf40-\udfff]|\ud86e[\udc00-\udc1d])+/;
    var SimpleTableRE = /^\s*[^\|].*?\|.*[^|]\s*$/;
    var SimpleTableLooseRE = /^\s*[^\|].*\|/; // unfinished | row
    var NormalTableRE = /^\s*\|[^\|]+\|.+\|\s*$/;
    var NormalTableLooseRE = /^\s*\|/; // | unfinished row
    var linkStyle = (_a = {},
        _a[1 /* BARELINK */] = "hmd-barelink",
        _a[6 /* BARELINK2 */] = "hmd-barelink2",
        _a[2 /* FOOTREF */] = "hmd-barelink hmd-footref",
        _a[4 /* FOOTNOTE */] = "hmd-footnote line-HyperMD-footnote",
        _a[7 /* FOOTREF2 */] = "hmd-footref2",
        _a);
    function resetTable(state) {
        state.hmdTable = 0 /* NONE */;
        state.hmdTableColumns = [];
        state.hmdTableID = null;
        state.hmdTableCol = state.hmdTableRow = 0;
    }
    var listInQuoteRE = /^\s+((\d+[).]|[-*+])\s+)?/;
    var defaultTokenTypeOverrides = {
        hr: "line-HyperMD-hr line-background-HyperMD-hr-bg hr",
        // HyperMD needs to know the level of header/indent. using tokenTypeOverrides is not enough
        // header: "line-HyperMD-header header",
        // quote: "line-HyperMD-quote quote",
        // Note: there are some list related process below
        list1: "list-1",
        list2: "list-2",
        list3: "list-3",
        code: "inline-code",
        hashtag: "hashtag meta",
    };
    CodeMirror.defineMode("hypermd", function (cmCfg, modeCfgUser) {
        var modeCfg = {
            front_matter: true,
            math: true,
            table: true,
            toc: true,
            orgModeMarkup: true,
            hashtag: false,
            fencedCodeBlockHighlighting: true,
            name: "markdown",
            highlightFormatting: true,
            taskLists: true,
            strikethrough: true,
            emoji: true,
            /** @see defaultTokenTypeOverrides */
            tokenTypeOverrides: defaultTokenTypeOverrides,
        };
        Object.assign(modeCfg, modeCfgUser);
        if (modeCfg.tokenTypeOverrides !== defaultTokenTypeOverrides) {
            modeCfg.tokenTypeOverrides = Object.assign({}, defaultTokenTypeOverrides, modeCfg.tokenTypeOverrides);
        }
        modeCfg["name"] = "markdown";
        /** functions from CodeMirror Markdown mode closure. Only for status checking */
        var rawClosure = {
            htmlBlock: null,
        };
        var rawMode = CodeMirror.getMode(cmCfg, modeCfg);
        var newMode = __assign({}, rawMode);
        newMode.startState = function () {
            var ans = rawMode.startState();
            resetTable(ans);
            ans.hmdOverride = null;
            ans.hmdInnerExitChecker = null;
            ans.hmdInnerMode = null;
            ans.hmdLinkType = 0 /* NONE */;
            ans.hmdNextMaybe = modeCfg.front_matter ? 1 /* FRONT_MATTER */ : 0 /* NONE */;
            ans.hmdNextState = null;
            ans.hmdNextStyle = null;
            ans.hmdNextPos = null;
            ans.hmdHashtag = 0 /* NONE */;
            return ans;
        };
        newMode.copyState = function (s) {
            var ans = rawMode.copyState(s);
            var keys = [
                "hmdLinkType", "hmdNextMaybe",
                "hmdTable", "hmdTableID", "hmdTableCol", "hmdTableRow",
                "hmdOverride",
                "hmdInnerMode", "hmdInnerStyle", "hmdInnerExitChecker",
                "hmdNextPos", "hmdNextState", "hmdNextStyle",
                "hmdHashtag",
            ];
            for (var _i = 0, keys_1 = keys; _i < keys_1.length; _i++) {
                var key = keys_1[_i];
                ans[key] = s[key];
            }
            ans.hmdTableColumns = s.hmdTableColumns.slice(0);
            if (s.hmdInnerMode)
                ans.hmdInnerState = CodeMirror.copyState(s.hmdInnerMode, s.hmdInnerState);
            return ans;
        };
        newMode.blankLine = function (state) {
            var ans;
            var innerMode = state.hmdInnerMode;
            if (innerMode) {
                if (innerMode.blankLine)
                    ans = innerMode.blankLine(state.hmdInnerState);
            }
            else {
                ans = rawMode.blankLine(state);
            }
            if (!ans)
                ans = "";
            if (state.code === -1) {
                ans += " line-HyperMD-codeblock line-background-HyperMD-codeblock-bg";
            }
            resetTable(state);
            return ans.trim() || null;
        };
        newMode.indent = function (state, textAfter) {
            var mode = state.hmdInnerMode || rawMode;
            var f = mode.indent;
            if (typeof f === 'function')
                return f.apply(mode, arguments);
            return CodeMirror.Pass;
        };
        newMode.innerMode = function (state) {
            if (state.hmdInnerMode)
                return { mode: state.hmdInnerMode, state: state.hmdInnerState };
            return rawMode.innerMode(state);
        };
        newMode.token = function (stream, state) {
            if (state.hmdOverride)
                return state.hmdOverride(stream, state);
            if (state.hmdNextMaybe === 1 /* FRONT_MATTER */) { // Only appears once for each Doc
                if (stream.string === '---') {
                    state.hmdNextMaybe = 2 /* FRONT_MATTER_END */;
                    return enterMode(stream, state, "yaml", {
                        style: "hmd-frontmatter",
                        fallbackMode: function () { return createDummyMode("---"); },
                        exitChecker: function (stream, state) {
                            if (stream.string === '---') {
                                // found the endline of front_matter
                                state.hmdNextMaybe = 0 /* NONE */;
                                return { endPos: 3 };
                            }
                            else {
                                return null;
                            }
                        }
                    });
                }
                else {
                    state.hmdNextMaybe = 0 /* NONE */;
                }
            }
            var wasInHTML = (state.f === rawClosure.htmlBlock);
            var wasInCodeFence = state.code === -1;
            var bol = stream.start === 0;
            // PATCHED: inline backtick code (any count) must not span lines.
            // markdown.js keeps state.code = <count> across newlines if it
            // never saw a matching closer, which paints the rest of the doc
            // as code. At SOL, clear any positive state.code so the next
            // line tokenizes as normal markdown. state.code === -1 means
            // we're inside a fenced ``` block and IS allowed to span - leave
            // that alone.
            if (bol && state.code > 0) {
                state.code = 0;
            }
            var wasLinkText = state.linkText;
            var wasLinkHref = state.linkHref;
            var inMarkdown = !(wasInCodeFence || wasInHTML);
            var inMarkdownInline = inMarkdown && !(state.code || state.indentedCode || state.linkHref);
            var ans = "";
            var tmp;
            if (inMarkdown) {
                // now implement some extra features that require higher priority than CodeMirror's markdown
                //#region Math
                // PATCHED: in addition to $...$ and $$...$$, also recognize LaTeX-style
                // \(...\) (inline) and \[...\] (display). These are emitted by ChatGPT/Claude
                // and present in pasted LaTeX sources, so accepting them avoids a lossy
                // format conversion step.
                if (modeCfg.math && inMarkdownInline) {
                    var openTag_1, endTag_1, mathLevel;
                    if ((tmp = stream.match(/^\${1,2}/, false))) {
                        var lvl = tmp[0].length;
                        // PATCHED: inline `$...$` must not have whitespace right
                        // after the opening `$` ("$ x") nor right before the
                        // closing `$` ("x $"), matching the common rule - so
                        // prose like "I have $5 and $10" or "$asdfsdf $" stays
                        // plain text. The `[^\\\s]\$` requires the closing `$` to
                        // be preceded by a non-space, non-backslash char. $$
                        // (display math) is unaffected.
                        var spaceAfter = /\s/.test(stream.string.charAt(stream.pos + lvl));
                        if (lvl === 2 || (!spaceAfter && stream.string.slice(stream.pos).match(/[^\\\s]\$/))) {
                            // $$ may span lines, $ must be paired on same line
                            openTag_1 = tmp[0];
                            endTag_1 = tmp[0];
                            mathLevel = lvl;
                        }
                    } else if (stream.match(/^\\\(/, false) && stream.string.slice(stream.pos + 2).indexOf('\\)') >= 0) {
                        // \( only counts as math if there's a matching \) on the same line.
                        // Otherwise fall through so the lone \( is rendered as a normal escape.
                        openTag_1 = '\\(';
                        endTag_1 = '\\)';
                        mathLevel = 1;
                    } else if (stream.match(/^\\\[/, false)) {
                        // PATCHED: \[ is math when either:
                        //   - the line contains a matching \] (single-line block), or
                        //   - the rest of the line is empty AND a \] exists somewhere in
                        //     the doc (multi-line block, the common case for AI-pasted math).
                        // The lookahead uses window.editor (set at boot in index.html) since
                        // CodeMirror modes don't have a `cm` reference. If no \] exists at
                        // all, fall through and render the \[ as escaped [.
                        var restAfterOpen = stream.string.slice(stream.pos + 2);
                        var sameLineClose = restAfterOpen.indexOf('\\]') >= 0;
                        var maybeMultiline = restAfterOpen.trim() === ''
                            && typeof window !== 'undefined' && window.editor
                            && window.editor.getValue().indexOf('\\]') >= 0;
                        if (sameLineClose || maybeMultiline) {
                            openTag_1 = '\\[';
                            endTag_1 = '\\]';
                            mathLevel = 2;
                        }
                    }
                    if (openTag_1) {
                        var texMode = CodeMirror.getMode(cmCfg, {
                            name: "stex",
                        });
                        var noTexMode = texMode['name'] !== 'stex';
                        ans += enterMode(stream, state, texMode, {
                            style: "math",
                            skipFirstToken: noTexMode,
                            fallbackMode: function () { return createDummyMode(endTag_1); },
                            exitChecker: createSimpleInnerModeExitChecker(endTag_1, {
                                style: "formatting formatting-math formatting-math-end math-" + mathLevel
                            })
                        });
                        if (noTexMode)
                            stream.pos += openTag_1.length;
                        ans += " formatting formatting-math formatting-math-begin math-" + mathLevel;
                        return ans;
                    }
                }
                //#endregion
                //#region [OrgMode] markup
                if (bol && modeCfg.orgModeMarkup && (tmp = stream.match(/^\#\+(\w+\:?)\s*/))) {
                    // Support #+TITLE: This is the title of the document
                    if (!stream.eol()) {
                        state.hmdOverride = function (stream, state) {
                            stream.skipToEnd();
                            state.hmdOverride = null;
                            return "string hmd-orgmode-markup";
                        };
                    }
                    return "meta formatting-hmd-orgmode-markup hmd-orgmode-markup line-HyperMD-orgmode-markup";
                }
                //#endregion
                //#region [TOC] in a single line
                if (bol && modeCfg.toc && stream.match(/^\[TOCM?\]\s*$/i)) {
                    return "meta line-HyperMD-toc hmd-toc";
                }
                //#endregion
                //#region Extra markdown inline extenson
                if (inMarkdownInline) {
                    // PATCHED: `// ` line-comment support. Emits two tokens:
                    //   1. `formatting hmd-comment-marker` for `// ` (3 chars)
                    //   2. `hmd-comment` for the rest of the line
                    // so themes can color both and hide just the marker on
                    // inactive lines. Header/quote classes are inherited so
                    // font/size styling carries through the comment span and
                    // the cursor doesn't jump between mismatched metrics.
                    //
                    // At SOL we mirror markdown.js's per-line state reset
                    // ourselves: if our checks intercept and return early,
                    // rawMode.token never runs and state.header/state.quote
                    // would otherwise stay stale from the previous line.
                    if (bol) {
                        state.hmdInComment = false;
                        state.header = 0;
                        state.quote = 0;
                    }

                    if (state.hmdInComment) {
                        stream.skipToEnd();
                        state.hmdInComment = false;
                        var contClasses = 'hmd-comment';
                        if (state.header) contClasses += ' header header-' + state.header;
                        if (state.quote) contClasses += ' quote quote-' + state.quote;
                        return contClasses;
                    }

                    // transform unformatted URL into link
                    if (!state.hmdLinkType && (stream.match(urlRE) || stream.match(emailRE))) {
                        return "url";
                    }

                    if (stream.peek() === '/' &&
                        stream.string.charAt(stream.pos + 1) === '/' &&
                        stream.string.charAt(stream.pos + 2) === ' ' &&
                        (stream.pos === 0 || /\s/.test(stream.string.charAt(stream.pos - 1)))) {
                        stream.pos += 3; // consume '// '
                        state.hmdInComment = true;
                        var markerClasses = 'formatting hmd-comment-marker';
                        if (state.header) markerClasses += ' header header-' + state.header;
                        if (state.quote) markerClasses += ' quote quote-' + state.quote;
                        return markerClasses;
                    }
                }
                //#endregion
            }
            // now enter markdown
            if (state.hmdNextState) {
                stream.pos = state.hmdNextPos;
                ans += " " + (state.hmdNextStyle || "");
                Object.assign(state, state.hmdNextState);
                state.hmdNextState = null;
                state.hmdNextStyle = null;
                state.hmdNextPos = null;
            }
            else {
                ans += " " + (rawMode.token(stream, state) || "");
            }
            // add extra styles
            if (state.hmdHashtag !== 0 /* NONE */) {
                ans += " " + modeCfg.tokenTypeOverrides.hashtag;
            }
            // PATCHED: mark lines containing an image so CSS can scope url-hiding
            // without a per-span :has(.cm-image) probe.
            if (/image-marker/.test(ans)) {
                ans += " line-hmd-image-line";
            }
            /** Try to capture some internal functions from CodeMirror Markdown mode closure! */
            if (!rawClosure.htmlBlock && state.htmlState)
                rawClosure.htmlBlock = state.f;
            var inHTML = (state.f === rawClosure.htmlBlock);
            var inCodeFence = state.code === -1;
            inMarkdown = inMarkdown && !(inHTML || inCodeFence);
            inMarkdownInline = inMarkdownInline && inMarkdown && !(state.code || state.indentedCode || state.linkHref);
            // If find a markdown extension token (which is not escaped),
            // break current parsed string into two parts and the first char of next part is the markdown extension token
            if (inMarkdownInline && (tmp = stream.current().match(tokenBreakRE))) {
                stream.pos = stream.start + tmp.index + 1; // rewind
            }
            var current = stream.current();
            if (inHTML != wasInHTML) {
                if (inHTML) {
                    ans += " hmd-html-begin";
                    rawClosure.htmlBlock = state.f;
                }
                else {
                    ans += " hmd-html-end";
                }
            }
            if (wasInCodeFence || inCodeFence) {
                if (!state.localMode || !wasInCodeFence)
                    ans = ans.replace("inline-code", "");
                ans += " line-HyperMD-codeblock line-background-HyperMD-codeblock-bg";
                if (inCodeFence !== wasInCodeFence) {
                    if (!inCodeFence)
                        ans += " line-HyperMD-codeblock-end line-background-HyperMD-codeblock-end-bg";
                    else if (!wasInCodeFence)
                        ans += " line-HyperMD-codeblock-begin line-background-HyperMD-codeblock-begin-bg";
                }
            }
            if (inMarkdown) {
                var tableType = state.hmdTable;
                //#region [Table] Reset
                if (bol && tableType) {
                    var rowRE = (tableType == 1 /* SIMPLE */) ? SimpleTableLooseRE : NormalTableLooseRE;
                    if (rowRE.test(stream.string)) {
                        // still in table
                        state.hmdTableCol = 0;
                        state.hmdTableRow++;
                    }
                    else {
                        // end of a table
                        resetTable(state);
                    }
                }
                //#endregion
                //#region Header, indentedCode, quote
                if (bol && state.header) {
                    if (/^(?:---+|===+)\s*$/.test(stream.string) && state.prevLine && state.prevLine.header) {
                        ans += " line-HyperMD-header-line line-HyperMD-header-line-" + state.header;
                    }
                    else {
                        ans += " line-HyperMD-header line-HyperMD-header-" + state.header;
                    }
                }
                if (state.indentedCode) {
                    ans += " hmd-indented-code";
                }
                if (state.quote) {
                    // mess up as less as possible
                    if (stream.eol()) {
                        ans += " line-HyperMD-quote line-HyperMD-quote-" + state.quote;
                        if (!/^ {0,3}\>/.test(stream.string))
                            ans += " line-HyperMD-quote-lazy"; // ">" is omitted
                    }
                    if (bol && (tmp = current.match(/^\s+/))) {
                        stream.pos = tmp[0].length; // rewind
                        ans += " hmd-indent-in-quote";
                        return ans.trim();
                    }
                    // make indentation (and potential list bullet) monospaced
                    if (/^>\s+$/.test(current) && stream.peek() != ">") {
                        // PATCHED, bad indentation for word-wrapped lines
                        stream.pos = stream.start + 1; // rewind!
                        current = ">";
                        state.hmdOverride = function (stream, state) {
                            stream.match(listInQuoteRE);
                            state.hmdOverride = null;
                            return "hmd-indent-in-quote line-HyperMD-quote line-HyperMD-quote-" + state.quote;
                        };
                    }
                }
                //#endregion
                //#region List
                var maxNonCodeIndentation = (state.listStack[state.listStack.length - 1] || 0) + 3;
                var tokenIsIndent = bol && /^\s+$/.test(current) && (state.list !== false || stream.indentation() <= maxNonCodeIndentation);
                var tokenIsListBullet = state.list && /formatting-list/.test(ans);
                if (tokenIsListBullet || (tokenIsIndent && (state.list !== false || stream.match(listRE, false)))) {
                    var listLevel = state.listStack && state.listStack.length || 0;
                    if (tokenIsIndent) {
                        if (stream.match(listRE, false)) { // next token is 1. 2. or bullet
                            if (state.list === false)
                                listLevel++;
                        }
                        else {
                            while (listLevel > 0 && stream.pos < state.listStack[listLevel - 1]) {
                                listLevel--; // find the real indent level
                            }
                            if (!listLevel) { // not even a list
                                return ans.trim() || null;
                            }
                            ans += " line-HyperMD-list-line-nobullet line-HyperMD-list-line line-HyperMD-list-line-" + listLevel;
                        }
                        ans += " hmd-list-indent hmd-list-indent-" + listLevel;
                    }
                    else if (tokenIsListBullet) {
                        // no space before bullet!
                        ans += " line-HyperMD-list-line line-HyperMD-list-line-" + listLevel;
                        // PATCHED: line-level marks for task/ordered items so CSS can
                        // use plain classes instead of per-line :has() probes.
                        // state.taskList is set by the markdown mode while it consumes
                        // this very bullet token, before the task token is reached.
                        if (state.taskList)
                            ans += " line-HyperMD-task-line";
                        if (/formatting-list-ol/.test(ans))
                            ans += " line-HyperMD-list-line-ol";
                    }
                }
                //#endregion
                //#region Link, BareLink, Footnote etc
                if (wasLinkText !== state.linkText) {
                    if (!wasLinkText) {
                        // entering a link
                        tmp = stream.match(/^([^\]]+)\](\(| ?\[|\:)?/, false);
                        if (!tmp) { // maybe met a line-break in link text?
                            state.hmdLinkType = 1 /* BARELINK */;
                        }
                        else if (!tmp[2]) { // barelink
                            if (tmp[1].charAt(0) === "^") {
                                state.hmdLinkType = 2 /* FOOTREF */;
                            }
                            else {
                                state.hmdLinkType = 1 /* BARELINK */;
                            }
                        }
                        else if (tmp[2] === ":") { // footnote
                            state.hmdLinkType = 4 /* FOOTNOTE */;
                        }
                        else if ((tmp[2] === "[" || tmp[2] === " [") && stream.string.charAt(stream.pos + tmp[0].length) === "]") { // [barelink2][]
                            state.hmdLinkType = 6 /* BARELINK2 */;
                        }
                        else {
                            state.hmdLinkType = 3 /* NORMAL */;
                        }
                    }
                    else {
                        // leaving a link
                        if (state.hmdLinkType in linkStyle)
                            ans += " " + linkStyle[state.hmdLinkType];
                        if (state.hmdLinkType === 4 /* FOOTNOTE */) {
                            state.hmdLinkType = 5 /* MAYBE_FOOTNOTE_URL */;
                        }
                        else {
                            state.hmdLinkType = 0 /* NONE */;
                        }
                    }
                }
                if (wasLinkHref !== state.linkHref) {
                    if (!wasLinkHref) {
                        // [link][doc] the [doc] part
                        if (current === "[" && stream.peek() !== "]") {
                            state.hmdLinkType = 7 /* FOOTREF2 */;
                        }
                    }
                    else if (state.hmdLinkType) { // leaving a Href
                        ans += " " + linkStyle[state.hmdLinkType];
                        state.hmdLinkType = 0 /* NONE */;
                    }
                }
                if (state.hmdLinkType !== 0 /* NONE */) {
                    if (state.hmdLinkType in linkStyle)
                        ans += " " + linkStyle[state.hmdLinkType];
                    if (state.hmdLinkType === 5 /* MAYBE_FOOTNOTE_URL */) {
                        if (!/^(?:\]\:)?\s*$/.test(current)) { // not spaces
                            if (urlRE.test(current) || url2RE.test(current))
                                ans += " hmd-footnote-url";
                            else
                                ans = ans.replace("string url", "");
                            state.hmdLinkType = 0 /* NONE */; // since then, can't be url anymore
                        }
                    }
                }
                //#endregion
                //#region start of an escaped char
                if (/formatting-escape/.test(ans) && current.length > 1) {
                    // CodeMirror merge backslash and escaped char into one token, which is not good
                    // Use hmdOverride to separate them
                    var escapedLength_1 = current.length - 1;
                    var escapedCharStyle_1 = ans.replace("formatting-escape", "escape") + " hmd-escape-char";
                    state.hmdOverride = function (stream, state) {
                        stream.pos += escapedLength_1;
                        state.hmdOverride = null;
                        return escapedCharStyle_1.trim();
                    };
                    ans += " hmd-escape-backslash";
                    stream.pos -= escapedLength_1;
                    return ans;
                }
                //#endregion
                //#region [Table] Creating Table and style Table Separators
                if (!ans.trim() && modeCfg.table) {
                    // string is unformatted
                    var isTableSep = false;
                    if (current.charAt(0) === "|") {
                        // is "|xxxxxx", separate "|" and "xxxxxx"
                        stream.pos = stream.start + 1; // rewind to end of "|"
                        current = "|";
                        isTableSep = true;
                    }
                    if (isTableSep) {
                        // if not inside a table, try to construct one
                        if (!tableType) {
                            // check 1: current line meet the table format
                            if (SimpleTableRE.test(stream.string))
                                tableType = 1 /* SIMPLE */;
                            else if (NormalTableRE.test(stream.string))
                                tableType = 2 /* NORMAL */;
                            // check 2: check every column's alignment style
                            var rowStyles = void 0;
                            if (tableType) {
                                var nextLine = stream.lookAhead(1);
                                if (tableType === 2 /* NORMAL */) {
                                    if (!NormalTableRE.test(nextLine)) {
                                        tableType = 0 /* NONE */;
                                    }
                                    else {
                                        // remove leading / tailing pipe char
                                        nextLine = nextLine.replace(/^\s*\|/, '').replace(/\|\s*$/, '');
                                    }
                                }
                                else if (tableType === 1 /* SIMPLE */) {
                                    if (!SimpleTableRE.test(nextLine)) {
                                        tableType = 0 /* NONE */;
                                    }
                                }
                                if (tableType) {
                                    rowStyles = nextLine.split("|");
                                    for (var i = 0; i < rowStyles.length; i++) {
                                        var row = rowStyles[i];
                                        if (/^\s*--+\s*:\s*$/.test(row))
                                            row = "right";
                                        else if (/^\s*:\s*--+\s*$/.test(row))
                                            row = "left";
                                        else if (/^\s*:\s*--+\s*:\s*$/.test(row))
                                            row = "center";
                                        else if (/^\s*--+\s*$/.test(row))
                                            row = "default";
                                        else {
                                            // ouch, can't be a table
                                            tableType = 0 /* NONE */;
                                            break;
                                        }
                                        rowStyles[i] = row;
                                    }
                                }
                            }
                            // step 3: made it
                            if (tableType) {
                                // successfully made one
                                state.hmdTable = tableType;
                                state.hmdTableColumns = rowStyles;
                                state.hmdTableID = "T" + stream.lineOracle.line;
                                state.hmdTableRow = state.hmdTableCol = 0;
                            }
                        }
                        // then
                        if (tableType) {
                            var colUbound = state.hmdTableColumns.length - 1;
                            if (tableType === 2 /* NORMAL */ && ((state.hmdTableCol === 0 && /^\s*\|$/.test(stream.string.slice(0, stream.pos))) || stream.match(/^\s*$/, false))) {
                                ans += " hmd-table-sep hmd-table-sep-dummy";
                            }
                            else if (state.hmdTableCol < colUbound) {
                                var row = state.hmdTableRow;
                                var col = state.hmdTableCol++;
                                if (col == 0) {
                                    ans += " line-HyperMD-table_" + state.hmdTableID + " line-HyperMD-table-" + tableType + " line-HyperMD-table-row line-HyperMD-table-row-" + row;
                                }
                                ans += " hmd-table-sep hmd-table-sep-" + col;
                            }
                        }
                    }
                }
                //#endregion
                if (tableType && state.hmdTableRow === 1) {
                    // fix a stupid problem:    :------: is not emoji
                    if (/emoji/.test(ans))
                        ans = "";
                }
                //#region HTML Block
                //
                // See https://github.github.com/gfm/#html-blocks type3-5
                if (inMarkdownInline && current === '<') {
                    var endTag = null;
                    if (stream.match(/^\![A-Z]+/))
                        endTag = ">";
                    else if (stream.match("?"))
                        endTag = "?>";
                    else if (stream.match("![CDATA["))
                        endTag = "]]>";
                    if (endTag != null) {
                        return enterMode(stream, state, null, {
                            endTag: endTag,
                            style: (ans + " comment hmd-cdata-html").trim(),
                        });
                    }
                }
                //#endregion
                //#region Hashtag
                if (modeCfg.hashtag && inMarkdownInline) {
                    switch (state.hmdHashtag) {
                        case 0 /* NONE */:
                            if (current === '#' &&
                                !state.linkText && !state.image &&
                                (bol || /^\s*$/.test(stream.string.charAt(stream.start - 1)))) {
                                var escape_removed_str = stream.string.slice(stream.pos).replace(/\\./g, '');
                                if ((tmp = hashtagRE.exec(escape_removed_str))) {
                                    if (/^\d+$/.test(tmp[0]))
                                        state.hmdHashtag = 0 /* NONE */;
                                    else
                                        state.hmdHashtag = 1 /* NORMAL */;
                                    escape_removed_str = escape_removed_str.slice(tmp[0].length);
                                    do {
                                        // found tailing #
                                        if (escape_removed_str[0] === '#' &&
                                            (escape_removed_str.length === 1 || !hashtagRE.test(escape_removed_str[1]))) {
                                            state.hmdHashtag = 2 /* WITH_SPACE */;
                                            break;
                                        }
                                        if (tmp = escape_removed_str.match(/^\s+/)) {
                                            escape_removed_str = escape_removed_str.slice(tmp[0].length);
                                            if (tmp = escape_removed_str.match(hashtagRE)) {
                                                // found a space + valid tag text parts
                                                // continue this loop until tailing # is found
                                                escape_removed_str = escape_removed_str.slice(tmp[0].length);
                                                continue;
                                            }
                                        }
                                        // can't establish a Hashtag WITH_SPACE. stop
                                        break;
                                    } while (true);
                                }
                                if (state.hmdHashtag) {
                                    ans += " formatting formatting-hashtag hashtag-begin " + modeCfg.tokenTypeOverrides.hashtag;
                                }
                            }
                            break;
                        case 1 /* NORMAL */:
                            var endHashTag = false;
                            if (!/formatting/.test(ans) && !/^\s*$/.test(current)) {
                                // if invalid hashtag char found, break current parsed text part
                                tmp = current.match(hashtagRE);
                                var backUpChars = current.length - (tmp ? tmp[0].length : 0);
                                if (backUpChars > 0) {
                                    stream.backUp(backUpChars);
                                    endHashTag = true;
                                }
                            }
                            if (!endHashTag)
                                endHashTag = stream.eol(); // end of line
                            if (!endHashTag)
                                endHashTag = !hashtagRE.test(stream.peek()); // or next char is invalid to hashtag name
                            // escaped char is always invisible to stream. no worry
                            if (endHashTag) {
                                ans += " hashtag-end";
                                state.hmdHashtag = 0 /* NONE */;
                            }
                            break;
                        case 2 /* WITH_SPACE */:
                            // escaped char is always invisible to stream. no worry
                            if (current === '#') {
                                // end the hashtag if meet unescaped #
                                ans = ans.replace(/\sformatting-header(?:-\d+)?/g, '');
                                ans += " formatting formatting-hashtag hashtag-end";
                                state.hmdHashtag = 0 /* NONE */;
                            }
                            break;
                    }
                }
                //#endregion
            }
            return ans.trim() || null;
        };
        function modeOverride(stream, state) {
            var exit = state.hmdInnerExitChecker(stream, state);
            var extraStyle = state.hmdInnerStyle;
            var ans = (!exit || !exit.skipInnerMode) && state.hmdInnerMode.token(stream, state.hmdInnerState) || "";
            if (extraStyle)
                ans += " " + extraStyle;
            if (exit) {
                if (exit.style)
                    ans += " " + exit.style;
                if (exit.endPos)
                    stream.pos = exit.endPos;
                state.hmdInnerExitChecker = null;
                state.hmdInnerMode = null;
                state.hmdInnerState = null;
                state.hmdOverride = null;
            }
            return ans.trim() || null;
        }
        /**
         * advance Markdown tokenizing stream
         *
         * @returns true if success, then `state.hmdNextState` & `state.hmdNextStyle` will be set
         */
        function advanceMarkdown(stream, state) {
            if (stream.eol() || state.hmdNextState)
                return false;
            var oldStart = stream.start;
            var oldPos = stream.pos;
            stream.start = oldPos;
            var newState = __assign({}, state);
            var newStyle = rawMode.token(stream, newState);
            state.hmdNextPos = stream.pos;
            state.hmdNextState = newState;
            state.hmdNextStyle = newStyle;
            // console.log("ADVANCED!", oldStart, oldPos, stream.start, stream.pos)
            // console.log("ADV", newStyle, newState)
            stream.start = oldStart;
            stream.pos = oldPos;
            return true;
        }
        function createDummyMode(endTag) {
            return {
                token: function (stream) {
                    var endTagSince = stream.string.indexOf(endTag, stream.start);
                    if (endTagSince === -1)
                        stream.skipToEnd(); // endTag not in this line
                    else if (endTagSince === 0)
                        stream.pos += endTag.length; // current token is endTag
                    else {
                        stream.pos = endTagSince;
                        if (stream.string.charAt(endTagSince - 1) === "\\")
                            stream.pos++;
                    }
                    return null;
                }
            };
        }
        function createSimpleInnerModeExitChecker(endTag, retInfo) {
            if (!retInfo)
                retInfo = {};
            return function (stream, state) {
                if (stream.string.substr(stream.start, endTag.length) === endTag) {
                    retInfo.endPos = stream.start + endTag.length;
                    return retInfo;
                }
                return null;
            };
        }
        /**
         * switch to another mode
         *
         * After entering a mode, you can then set `hmdInnerExitStyle` and `hmdInnerState` of `state`
         *
         * @returns if `skipFirstToken` not set, returns `innerMode.token(stream, innerState)`, meanwhile, stream advances
         */
        function enterMode(stream, state, mode, opt) {
            if (typeof mode === "string")
                mode = CodeMirror.getMode(cmCfg, mode);
            if (!mode || mode["name"] === "null") {
                if ('endTag' in opt)
                    mode = createDummyMode(opt.endTag);
                else
                    mode = (typeof opt.fallbackMode === 'function') && opt.fallbackMode();
                if (!mode)
                    throw new Error("no mode");
            }
            state.hmdInnerExitChecker = ('endTag' in opt) ? createSimpleInnerModeExitChecker(opt.endTag) : opt.exitChecker;
            state.hmdInnerStyle = opt.style;
            state.hmdInnerMode = mode;
            state.hmdOverride = modeOverride;
            state.hmdInnerState = CodeMirror.startState(mode);
            var ans = opt.style || "";
            if (!opt.skipFirstToken) {
                ans += " " + mode.token(stream, state.hmdInnerState);
            }
            return ans.trim();
        }
        return newMode;
    }, "hypermd");
    CodeMirror.defineMIME("text/x-hypermd", "hypermd");
});
