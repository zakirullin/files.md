// HyperMD, copyright (c) by laobubu
// Distributed under an MIT license: http://laobubu.net/HyperMD/LICENSE
//
// powerful keymap for HyperMD and Markdown modes
//

(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("codemirror"), require("../core")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","codemirror","../core"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.KeyMap = this.HyperMD.KeyMap || {}), CodeMirror, CodeMirror, HyperMD);
})(function (require, exports, CodeMirror, codemirror_1, core_1) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });
    var _a;
    /**
     Some codes in this files are from CodeMirror's source code.

     CodeMirror, copyright (c) by Marijn Haverbeke and others
     MIT license: http://codemirror.net/LICENSE

     @see codemirror\addon\edit\continuelist.js
     */
        // loq = List Or Quote
    var LoQRE = /^(\s*)(>[> ]*|[*+-] \[[x ]\]\s|[*+-]\s|(\d+)([.)]))(\s*)/, emptyLoQRE = /^(\s*)(>[> ]*|[*+-] \[[x ]\]|[*+-]|(\d+)[.)])(\s*)$/, unorderedListRE = /[*+-]\s/;
    var ListRE = /^(\s*)([*+-]\s|(\d+)([.)]))(\s*)/;
    var isRealTableSep = function (token) { return /hmd-table-sep/.test(token.type) && !/hmd-table-sep-dummy/.test(token.type); };
    /**
     * continue list / quote / insert table row
     * start a table
     */
    function newlineAndContinue(cm) {
        if (cm.getOption("disableInput"))
            return CodeMirror.Pass;
        var selections = cm.listSelections();
        var replacements = [];
        for (var _i = 0, selections_1 = selections; _i < selections_1.length; _i++) {
            var range = selections_1[_i];
            var pos = range.head;
            var rangeEmpty = range.empty();
            var eolState = cm.getStateAfter(pos.line);
            var line = cm.getLine(pos.line);
            var handled = false;
            if (!handled) {
                var inList = eolState.list !== false;
                var inQuote = eolState.quote;
                var match = LoQRE.exec(line);
                var cursorBeforeBullet = /^\s*$/.test(line.slice(0, pos.ch));
                if (rangeEmpty && (inList || inQuote) && match && !cursorBeforeBullet) {
                    handled = true;
                    if (emptyLoQRE.test(line)) {
                        if (!/>\s*$/.test(line))
                            cm.replaceRange("", { line: pos.line, ch: 0 }, { line: pos.line, ch: pos.ch + 1 });
                        replacements.push("\n");
                    }
                    else {
                        var indent = match[1], after = match[5];
                        var numbered = !(unorderedListRE.test(match[2]) || match[2].indexOf(">") >= 0);
                        var bullet = numbered ? (parseInt(match[3], 10) + 1) + match[4] : match[2].replace("x", " ");
                        replacements.push("\n" + indent + bullet + after);
                        if (numbered)
                            incrementRemainingMarkdownListNumbers(cm, pos);
                    }
                }
            }
            if (!handled) {
                var table = rangeEmpty ? eolState.hmdTable : 0 /* NONE */;
                if (table != 0 /* NONE */) {
                    if (/^[\s\|]+$/.test(line) && (pos.line === cm.lastLine() || (cm.getStateAfter(pos.line + 1).hmdTable !== table))) {
                        // if this is last row and is empty
                        // remove this row and insert a new line
                        cm.setCursor({ line: pos.line, ch: 0 });
                        cm.replaceRange("\n", { line: pos.line, ch: 0 }, { line: pos.line, ch: line.length });
                    }
                    else {
                        // insert a row below
                        var columns = eolState.hmdTableColumns;
                        var newline_1 = core_1.repeatStr("  |  ", columns.length - 1);
                        var leading = "\n";
                        if (table === 2 /* NORMAL */) {
                            leading += "| ";
                            newline_1 += " |";
                        }
                        // There are always nut users!
                        if (eolState.hmdTableRow == 0) {
                            cm.setCursor({ line: pos.line + 1, ch: cm.getLine(pos.line + 1).length });
                        }
                        else {
                            cm.setCursor({ line: pos.line, ch: line.length });
                        }
                        cm.replaceSelection(leading);
                        cm.replaceSelection(newline_1, "start");
                    }
                    handled = true;
                    return;
                }
                else if (rangeEmpty && pos.ch >= line.length && !eolState.code && !eolState.hmdInnerMode && /^\|.+\|.+\|$/.test(line)) {
                    // current line is   | this | format |
                    // let's make a table
                    var lineTokens = cm.getLineTokens(pos.line);
                    var ans = "|", ans2 = "|";
                    for (var i = 1; i < lineTokens.length; i++) { // first token must be "|"
                        var token = lineTokens[i];
                        if (token.string === "|" && (!token.type || !token.type.trim().length)) {
                            ans += " ------- |";
                            ans2 += "   |";
                        }
                    }
                    // multi-cursor is meanless for this
                    // replacements.push("\n" + ans + "\n" + ans2 + "\n")
                    cm.setCursor({ line: pos.line, ch: line.length });
                    cm.replaceSelection("\n" + ans + "\n| ");
                    cm.replaceSelection(ans2.slice(1) + "\n", "start");
                    handled = true;
                    return;
                }
            }
            if (!handled) {
                if (rangeEmpty && line.slice(pos.ch - 2, pos.ch) == "$$" && /math-end/.test(cm.getTokenTypeAt(pos))) {
                    // ignore indentations of MathBlock Tex lines
                    replacements.push("\n");
                    handled = true;
                }
            }
            if (!handled) {
                cm.execCommand("newlineAndIndent");
                return;
            }
        }
        cm.replaceSelections(replacements);
    }
    exports.newlineAndContinue = newlineAndContinue;
    /** insert "\n" , or if in list, insert "\n" + indentation */
    function newline(cm) {
        if (cm.getOption("disableInput"))
            return CodeMirror.Pass;
        var selections = cm.listSelections();
        var replacements = core_1.repeat("\n", selections.length);
        for (var i = 0; i < selections.length; i++) {
            var range = selections[i];
            var pos = range.head;
            var eolState = cm.getStateAfter(pos.line);
            if (eolState.list !== false) {
                replacements[i] += core_1.repeatStr(" ", eolState.listStack.slice(-1)[0]);
            }
        }
        cm.replaceSelections(replacements);
    }
    exports.newline = newline;
    function killIndent(cm, lineNo, spaces) {
        if (!spaces || spaces < 0)
            return;
        var oldSpaces = /^ */.exec(cm.getLine(lineNo))[0].length;
        if (oldSpaces < spaces)
            spaces = oldSpaces;
        if (spaces > 0)
            cm.replaceRange("", { line: lineNo, ch: 0 }, { line: lineNo, ch: spaces });
    }
    /** unindent or move cursor into prev table cell */
    function shiftTab(cm) {
        var _a;
        var selections = cm.listSelections();
        var replacements = [];
        var tokenSeeker = new core_1.TokenSeeker(cm);
        for (var i = 0; i < selections.length; i++) {
            var range = selections[i];
            var left = range.head;
            var right = range.anchor;
            var rangeEmpty = range.empty();
            if (!rangeEmpty && codemirror_1.cmpPos(left, right) > 0)
                _a = [left, right], right = _a[0], left = _a[1];
            else if (right === left) {
                right = range.anchor = { ch: left.ch, line: left.line };
            }
            var eolState = cm.getStateAfter(left.line);
            if (eolState.hmdTable) {
                tokenSeeker.setPos(left.line, left.ch);
                var isNormalTable = eolState.hmdTable === 2 /* NORMAL */; // leading and ending | is not omitted
                var line = left.line;
                var lineText = cm.getLine(line);
                var chStart = 0, chEnd = 0;
                var rightPipe = tokenSeeker.findPrev(isRealTableSep);
                if (rightPipe) { // prev cell is in this line
                    var leftPipe = tokenSeeker.findPrev(isRealTableSep, rightPipe.i_token - 1);
                    chStart = leftPipe ? leftPipe.token.end : 0;
                    chEnd = rightPipe.token.start;
                    if (chStart == 0 && isNormalTable)
                        chStart += lineText.match(/^\s*\|/)[0].length;
                }
                else { // jump to prev line, last cell
                    if (eolState.hmdTableRow == 0)
                        return; // no more row before
                    if (eolState.hmdTableRow == 2)
                        line--; // skip row #1 (| ----- | ----- |)
                    line--;
                    lineText = cm.getLine(line);
                    tokenSeeker.setPos(line, lineText.length);
                    var leftPipe = tokenSeeker.findPrev(isRealTableSep);
                    chStart = leftPipe.token.end;
                    chEnd = lineText.length;
                    if (isNormalTable)
                        chEnd -= lineText.match(/\|\s*$/)[0].length;
                }
                if (lineText.charAt(chStart) === " ")
                    chStart += 1;
                if (chStart > 0 && lineText.substr(chStart - 1, 2) === ' |')
                    chStart--;
                if (lineText.charAt(chEnd - 1) === " ")
                    chEnd -= 1;
                cm.setSelection({ line: line, ch: chStart }, { line: line, ch: chEnd });
                return;
            }
            else if (eolState.listStack.length > 0) {
                var lineNo = left.line;
                while (!ListRE.test(cm.getLine(lineNo))) { // beginning line has no bullet? go up
                    lineNo--;
                    var isList = cm.getStateAfter(lineNo).listStack.length > 0;
                    if (!isList) {
                        lineNo++;
                        break;
                    }
                }
                var lastLine = cm.lastLine();
                var tmp = void 0;
                for (; lineNo <= right.line && (tmp = ListRE.exec(cm.getLine(lineNo))); lineNo++) {
                    var listStack = cm.getStateAfter(lineNo).listStack;
                    var listLevel = listStack.length;
                    var spaces = 0;
                    if (listLevel == 1) {
                        // maybe user wants to trimLeft?
                        spaces = tmp[1].length;
                    }
                    else {
                        // make bullets right-aligned
                        spaces = (listStack[listLevel - 1] - (listStack[listLevel - 2] || 0));
                    }
                    killIndent(cm, lineNo, spaces);
                    // if current list item is multi-line...
                    while (++lineNo <= lastLine) {
                        if ( /*corrupted */cm.getStateAfter(lineNo).listStack.length !== listLevel) {
                            lineNo = Infinity;
                            break;
                        }
                        if ( /*has bullet*/ListRE.test(cm.getLine(lineNo))) {
                            lineNo--;
                            break;
                        }
                        killIndent(cm, lineNo, spaces);
                    }
                }
                return;
            }
        }
        cm.execCommand("indentLess");
    }
    exports.shiftTab = shiftTab;
    /**
     * 1. for tables, move cursor into next table cell, and maybe insert a cell
     * 2.
     */
    function tab(cm) {
        var _a;
        var selections = cm.listSelections();
        var beforeCur = [];
        var afterCur = [];
        var selected = [];
        var addIndentTo = {}; // {lineNo: stringIndent}
        var tokenSeeker = new core_1.TokenSeeker(cm);
        /** indicate previous 4 variable changed or not */
        var flag0 = false, flag1 = false, flag2 = false, flag3 = true;
        function setBeforeCur(text) { beforeCur[i] = text; if (text)
            flag1 = true; }
        function setAfterCur(text) { afterCur[i] = text; if (text)
            flag2 = true; }
        function setSelected(text) { selected[i] = text; if (text)
            flag3 = true; }
        for (var i = 0; i < selections.length; i++) {
            beforeCur[i] = afterCur[i] = selected[i] = "";
            var range = selections[i];
            var left = range.head;
            var right = range.anchor;
            var rangeEmpty = range.empty();
            if (!rangeEmpty && codemirror_1.cmpPos(left, right) > 0)
                _a = [left, right], right = _a[0], left = _a[1];
            else if (right === left) {
                right = range.anchor = { ch: left.ch, line: left.line };
            }
            var eolState = cm.getStateAfter(left.line);
            var line = cm.getLine(left.line);
            if (eolState.hmdTable) {
                // yeah, we are inside a table
                flag0 = true; // cursor will move
                var isNormalTable = eolState.hmdTable === 2 /* NORMAL */;
                var columns = eolState.hmdTableColumns;
                tokenSeeker.setPos(left.line, left.ch);
                var nextCellLeft = tokenSeeker.findNext(isRealTableSep, tokenSeeker.i_token);
                if (!nextCellLeft) { // already last cell
                    var lineSpan = eolState.hmdTableRow === 0 ? 2 : 1; // skip |---|---| line
                    if ((left.line + lineSpan) > cm.lastLine() || cm.getStateAfter(left.line + lineSpan).hmdTable != eolState.hmdTable) {
                        // insert a row after this line
                        left.ch = right.ch = line.length;
                        var newline_2 = core_1.repeatStr("  |  ", columns.length - 1);
                        // There are always nut users!
                        if (eolState.hmdTableRow === 0) {
                            right.line = left.line += 1;
                            right.ch = left.ch = cm.getLine(left.line).length;
                        }
                        if (isNormalTable) {
                            setBeforeCur("\n| ");
                            setAfterCur(newline_2 + " |");
                        }
                        else {
                            setBeforeCur("\n");
                            setAfterCur(newline_2.trimRight());
                        }
                        setSelected("");
                    }
                    else {
                        // move cursor to next line, first cell
                        right.line = left.line += lineSpan;
                        tokenSeeker.setPos(left.line, 0);
                        var line_1 = tokenSeeker.line.text;
                        var dummySep = isNormalTable && tokenSeeker.findNext(/hmd-table-sep-dummy/, 0);
                        var nextCellRight = tokenSeeker.findNext(/hmd-table-sep/, dummySep ? dummySep.i_token + 1 : 1);
                        left.ch = dummySep ? dummySep.token.end : 0;
                        right.ch = nextCellRight ? nextCellRight.token.start : line_1.length;
                        if (right.ch > left.ch && line_1.charAt(left.ch) === " ")
                            left.ch++;
                        if (right.ch > left.ch && line_1.charAt(right.ch - 1) === " ")
                            right.ch--;
                        setSelected(right.ch > left.ch ? cm.getRange(left, right) : "");
                    }
                }
                else {
                    var nextCellRight = tokenSeeker.findNext(/hmd-table-sep/, nextCellLeft.i_token + 1);
                    left.ch = nextCellLeft.token.end;
                    right.ch = nextCellRight ? nextCellRight.token.start : line.length;
                    if (right.ch > left.ch && line.charAt(left.ch) === " ")
                        left.ch++;
                    if (right.ch > left.ch && line.charAt(right.ch - 1) === " ")
                        right.ch--;
                    setSelected(right.ch > left.ch ? cm.getRange(left, right) : "");
                }
                // console.log("selected cell", left.ch, right.ch, selected[i])
            }
            else if (eolState.listStack.length > 0) {
                // add indent to current line
                var lineNo = left.line;
                var tmp = void 0; // ["  * ", "  ", "* "]
                while (!(tmp = ListRE.exec(cm.getLine(lineNo)))) { // beginning line has no bullet? go up
                    lineNo--;
                    var isList = cm.getStateAfter(lineNo).listStack.length > 0;
                    if (!isList) {
                        lineNo++;
                        break;
                    }
                }
                var firstLine = cm.firstLine();
                var lastLine = cm.lastLine();
                for (; lineNo <= right.line && (tmp = ListRE.exec(cm.getLine(lineNo))); lineNo++) {
                    var eolState_1 = cm.getStateAfter(lineNo);
                    var listStack = eolState_1.listStack;
                    var listStackOfPrevLine = cm.getStateAfter(lineNo - 1).listStack;
                    var listLevel = listStack.length;
                    var spaces = "";
                    // avoid uncontinuous list levels
                    if (lineNo > firstLine && listLevel <= listStackOfPrevLine.length) {
                        if (listLevel == listStackOfPrevLine.length) {
                            // tmp[1] is existed leading spaces
                            // listStackOfPrevLine[listLevel-1] is desired indentation
                            spaces = core_1.repeatStr(" ", listStackOfPrevLine[listLevel - 1] - tmp[1].length);
                        }
                        else {
                            // make bullets right-aligned
                            // tmp[0].length is end pos of current bullet
                            spaces = core_1.repeatStr(" ", listStackOfPrevLine[listLevel] - tmp[0].length);
                        }
                    }
                    addIndentTo[lineNo] = spaces;
                    // if current list item is multi-line...
                    while (++lineNo <= lastLine) {
                        if ( /*corrupted */cm.getStateAfter(lineNo).listStack.length !== listLevel) {
                            lineNo = Infinity;
                            break;
                        }
                        if ( /*has bullet*/ListRE.test(cm.getLine(lineNo))) {
                            lineNo--;
                            break;
                        }
                        addIndentTo[lineNo] = spaces;
                    }
                }
                if (!rangeEmpty) {
                    flag3 = false;
                    break; // f**k
                }
            }
            else {
                // emulate Tab
                if (rangeEmpty) {
                    setBeforeCur("    ");
                }
                else {
                    setSelected(cm.getRange(left, right));
                    for (var lineNo = left.line; lineNo <= right.line; lineNo++) {
                        if (!(lineNo in addIndentTo))
                            addIndentTo[lineNo] = "    ";
                    }
                }
            }
        }
        // if (!(flag0 || flag1 || flag2 || flag3)) return cm.execCommand("defaultTab")
        // console.log(flag0, flag1, flag2, flag3)
        for (var lineNo in addIndentTo) {
            if (addIndentTo[lineNo])
                cm.replaceRange(addIndentTo[lineNo], { line: ~~lineNo, ch: 0 });
        }
        if (flag0)
            cm.setSelections(selections);
        if (flag1)
            cm.replaceSelections(beforeCur);
        if (flag2)
            cm.replaceSelections(afterCur, "start");
        if (flag3)
            cm.replaceSelections(selected, "around");
    }
    exports.tab = tab;
    /**
     * add / delete bracket pair to every selections,
     * or just add left bracket to cursor if nothing selected.
     *
     * This provides a `createStyleToggler`-like feature,
     * but don't rely on HyperMD mode
     *
     * @example
     *     When brackets are "(" and ")" :
     *     (Hello) => Hello   (Selected "(Hello)" or just "Hello")
     *     Hello   => (Hello)
     *
     * @param rightBracket if null, will use leftBracket
     */
    function wrapTexts(cm, leftBracket, rightBracket) {
        var _a;
        if (cm.getOption("disableInput"))
            return CodeMirror.Pass;
        var selections = cm.listSelections();
        var replacements = new Array(selections.length);
        var insertBeforeCursor = new Array(selections.length);
        var flag0 = false; // replacements changed
        var flag1 = false; // insertBeforeCursor changed
        var flag2 = false; // selections changed
        if (!rightBracket)
            rightBracket = leftBracket;
        var lb_len = leftBracket.length;
        var rb_len = rightBracket.length;
        for (var i = 0; i < selections.length; i++) {
            replacements[i] = insertBeforeCursor[i] = "";
            var range = selections[i];
            var left = range.head;
            var right = range.anchor;
            var line = cm.getLine(left.line);
            if (range.empty()) {
                if (left.ch >= lb_len && line.substr(left.ch - lb_len, lb_len) === leftBracket) {
                    // wipe out the left bracket
                    flag2 = true;
                    left.ch -= lb_len;
                }
                else {
                    // insert left bracket
                    flag1 = true;
                    insertBeforeCursor[i] = leftBracket;
                }
                continue;
            }
            flag0 = true;
            var headAfterAnchor = codemirror_1.cmpPos(left, right) > 0;
            if (headAfterAnchor)
                _a = [left, right], right = _a[0], left = _a[1];
            var text = cm.getRange(left, right);
            if (left.ch >= lb_len && left.line === right.line) {
                if (line.substr(left.ch - lb_len, lb_len) === leftBracket && line.substr(right.ch, rb_len) === rightBracket) {
                    flag2 = true;
                    right.ch += rb_len;
                    left.ch -= lb_len;
                    text = leftBracket + text + rightBracket;
                }
            }
            if (text.slice(0, lb_len) === leftBracket && text.slice(-rb_len) === rightBracket) {
                replacements[i] = text.slice(lb_len, -rb_len);
            }
            else {
                replacements[i] = leftBracket + text + rightBracket;
            }
        }
        if (flag2)
            cm.setSelections(selections);
        if (flag1)
            cm.replaceSelections(insertBeforeCursor);
        if (flag0)
            cm.replaceSelections(replacements, "around");
    }
    exports.wrapTexts = wrapTexts;
    function createStyleToggler(isStyled, isFormattingToken, getFormattingText) {
        return function (cm) {
            var _a;
            if (cm.getOption("disableInput"))
                return CodeMirror.Pass;
            var ts = new core_1.TokenSeeker(cm);
            var selections = cm.listSelections();
            var replacements = new Array(selections.length);
            for (var i = 0; i < selections.length; i++) {
                var range = selections[i];
                var left = range.head;
                var right = range.anchor;
                var eolState = cm.getStateAfter(left.line);
                var rangeEmpty = range.empty();
                if (codemirror_1.cmpPos(left, right) > 0)
                    _a = [left, right], right = _a[0], left = _a[1];
                var rangeText = replacements[i] = rangeEmpty ? "" : cm.getRange(left, right);
                if (rangeEmpty || isStyled(cm.getTokenAt(left).state)) { // nothing selected
                    var line = left.line;
                    ts.setPos(line, left.ch, true);
                    var token_1 = ts.lineTokens[ts.i_token];
                    var state_1 = token_1 ? token_1.state : eolState;
                    if (!token_1 || /^\s*$/.test(token_1.string)) {
                        token_1 = ts.lineTokens[--ts.i_token]; // maybe eol, or current token is space
                    }
                    var _b = ts.expandRange(function (token) { return token && (isStyled(token.state) || isFormattingToken(token)); }), from = _b.from, to = _b.to;
                    if (to.i_token === from.i_token) { // current token "word" is not formatted
                        var f = getFormattingText();
                        if (token_1 && !/^\s*$/.test(token_1.string)) { // not empty line, not spaces
                            var pos1 = { line: line, ch: token_1.start }, pos2 = { line: line, ch: token_1.end };
                            token_1 = from.token;
                            cm.replaceRange(f + token_1.string + f, pos1, pos2);
                            pos2.ch += f.length;
                            cm.setCursor(pos2);
                            return;
                        }
                        else {
                            replacements[i] = f;
                        }
                    }
                    else { // **wor|d**    **|**   **word|  **|
                        if (isFormattingToken(to.token)) {
                            cm.replaceRange("", { line: line, ch: to.token.start }, { line: line, ch: to.token.end });
                        }
                        if (from.i_token !== to.i_token && isFormattingToken(from.token)) {
                            cm.replaceRange("", { line: line, ch: from.token.start }, { line: line, ch: from.token.end });
                        }
                    }
                    continue;
                }
                var token = cm.getTokenAt(left);
                var state = token ? token.state : eolState;
                var formatter = getFormattingText(state);
                replacements[i] = formatter + rangeText + formatter;
            }
            cm.replaceSelections(replacements);
        };
    }
    exports.createStyleToggler = createStyleToggler;
    // Auto-updating Markdown list numbers when a new item is added to the
    // middle of a list
    function incrementRemainingMarkdownListNumbers(cm, pos) {
        var listRE = LoQRE;
        var startLine = pos.line, lookAhead = 0, skipCount = 0;
        var startItem = listRE.exec(cm.getLine(startLine)), startIndent = startItem[1];
        do {
            lookAhead += 1;
            var nextLineNumber = startLine + lookAhead;
            var nextLine = cm.getLine(nextLineNumber), nextItem = listRE.exec(nextLine);
            if (nextItem) {
                var nextIndent = nextItem[1];
                var newNumber = (parseInt(startItem[3], 10) + lookAhead - skipCount);
                var nextNumber = (parseInt(nextItem[3], 10)), itemNumber = nextNumber;
                if (startIndent === nextIndent && !isNaN(nextNumber)) {
                    if (newNumber === nextNumber)
                        itemNumber = nextNumber + 1;
                    if (newNumber > nextNumber)
                        itemNumber = newNumber + 1;
                    cm.replaceRange(nextLine.replace(listRE, nextIndent + itemNumber + nextItem[4] + nextItem[5]), {
                        line: nextLineNumber, ch: 0
                    }, {
                        line: nextLineNumber, ch: nextLine.length
                    });
                }
                else {
                    if (startIndent.length > nextIndent.length)
                        return;
                    // This doesn't run if the next line immediatley indents, as it is
                    // not clear of the users intention (new indented item or same level)
                    if ((startIndent.length < nextIndent.length) && (lookAhead === 1))
                        return;
                    skipCount += 1;
                }
            }
        } while (nextItem);
    }
    Object.assign(CodeMirror.commands, {
        hmdNewlineAndContinue: newlineAndContinue,
        hmdNewline: newline,
        hmdShiftTab: shiftTab,
        hmdTab: tab,
    });
    var defaultKeyMap = CodeMirror.keyMap["default"];
    var modPrefix = defaultKeyMap === CodeMirror.keyMap["macDefault"] ? "Cmd" : "Ctrl";
    exports.keyMap = (_a = {
        "Shift-Tab": "hmdShiftTab",
        "Tab": "hmdTab",
        "Enter": "hmdNewlineAndContinue",
        "Shift-Enter": "hmdNewline"
    },
        _a[modPrefix + "-K"] = createStyleToggler(function (state) { return state.strong; }, function (token) { return / formatting-strong /.test(token.type); }, function (state) { return core_1.repeatStr(state && state.strong || "*", 2); } // ** or __
        ),
        _a[modPrefix + "-B"] = createStyleToggler(function (state) { return state.strong; }, function (token) { return / formatting-strong /.test(token.type); }, function (state) { return core_1.repeatStr(state && state.strong || "*", 2); } // ** or __
        ),
        _a[modPrefix + "-I"] = createStyleToggler(function (state) { return state.em; }, function (token) { return / formatting-em /.test(token.type); }, function (state) { return (state && state.em || "*"); }),
        _a[modPrefix + "-D"] = createStyleToggler(function (state) { return state.strikethrough; }, function (token) { return / formatting-strikethrough /.test(token.type); }, function (state) { return "~~"; }),
        _a.fallthrough = "default",
        _a);
    exports.keyMap = CodeMirror.normalizeKeyMap(exports.keyMap);
    CodeMirror.keyMap["hypermd"] = exports.keyMap;
    core_1.suggestedEditorConfig.keyMap = "hypermd";
});