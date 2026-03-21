// HyperMD, copyright (c) by laobubu
// Distributed under an MIT license: http://laobubu.net/HyperMD/LICENSE
//
// DESCRIPTION: Auto show/hide markdown tokens like `##` or `*`
//
// Only works with `hypermd` mode, require special CSS rules
//

(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("../core"), require("../core"), require("../core")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","../core","../core","../core"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.HideToken = this.HyperMD.HideToken || {}), CodeMirror, HyperMD, HyperMD, HyperMD);
})(function (require, exports, CodeMirror, core_1, cm_utils_1, line_spans_1) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });
    var DEBUG = false;
    //#region Internal Function...
    /** check if has the class and remove it */
    function rmClass(el, className) {
        var c = ' ' + el.className + ' ', cnp = ' ' + className + ' ';
        if (c.indexOf(cnp) === -1)
            return false;
        el.className = c.replace(cnp, '').trim();
        return true;
    }
    /** check if NOT has the class and add it */
    function addClass(el, className) {
        var c = ' ' + el.className + ' ', cnp = ' ' + className + ' ';
        if (c.indexOf(cnp) !== -1)
            return false;
        el.className = (el.className + ' ' + className);
        return true;
    }
    exports.defaultOption = {
        enabled: false,
        line: true,
        tokenTypes: "em|strong|strikethrough|code|linkText|linkHref|task".split("|"),
    };
    exports.suggestedOption = {
        enabled: true,
    };
    core_1.suggestedEditorConfig.hmdHideToken = exports.suggestedOption;
    core_1.normalVisualConfig.hmdHideToken = false;
    CodeMirror.defineOption("hmdHideToken", exports.defaultOption, function (cm, newVal) {
        ///// convert newVal's type to `Partial<Options>`, if it is not.
        if (!newVal || typeof newVal === "boolean") {
            newVal = { enabled: !!newVal };
        }
        else if (typeof newVal === "string") {
            newVal = { enabled: true, tokenTypes: newVal.split("|") };
        }
        else if (newVal instanceof Array) {
            newVal = { enabled: true, tokenTypes: newVal };
        }
        ///// apply config and write new values into cm
        var inst = exports.getAddon(cm);
        for (var k in exports.defaultOption) {
            inst[k] = (k in newVal) ? newVal[k] : exports.defaultOption[k];
        }
    });
    //#endregion
    /********************************************************************************** */
        //#region Addon Class
    var hideClassName = "hmd-hidden-token";
    var lineInactiveClassName = "hmd-inactive-line";
    var HideToken = /** @class */ (function () {
        function HideToken(cm) {
            var _this = this;
            this.cm = cm;
            this.renderLineHandler = function (cm, line, el) {
                // TODO: if we procLine now, we can only get the outdated lineView, lineViewMeasure and lineViewMap. Calling procLine will be wasteful!
                var changed = _this.procLine(line, el);
                if (DEBUG)
                    console.log("renderLine return " + changed);
            };
            this.cursorActivityHandler = function (doc) {
                // PATCHED, if we don't do this, autoscroll is not working.
                if (cm.somethingSelected()) return;

                // PATCHED, prevent blinking
                // Actually that works bad when we show/hide a header with 2 lines of text (expanded) and 1 line of text hidden.
                // Cursor jumps to wrong positions
                // _this.updateImmediately();
                _this.update();
            };
            this.update = core_1.debounce(function () { return _this.updateImmediately(); }, 100);
            /** Current user's selections, in each line */
            this._rangesInLine = {};
            new core_1.FlipFlop(
                /* ON  */ function () {
                    cm.on("cursorActivity", _this.cursorActivityHandler);
                    cm.on("renderLine", _this.renderLineHandler);
                    cm.on("update", _this.update);
                    _this.update();
                    cm.refresh();
                },
                /* OFF */ function () {
                    cm.off("cursorActivity", _this.cursorActivityHandler);
                    cm.off("renderLine", _this.renderLineHandler);
                    cm.off("update", _this.update);
                    _this.update.stop();
                    cm.refresh();
                }).bind(this, "enabled", true);
        }
        /**
         * hide/show <span>s in one line, based on `this._rangesInLine`
         * @returns line changed or not
         */
        HideToken.prototype.procLine = function (line, pre) {
            var cm = this.cm;
            var lineNo = typeof line === 'number' ? line : line.lineNo();
            if (typeof line === 'number')
                line = cm.getLineHandle(line);
            var rangesInLine = this._rangesInLine[lineNo] || [];
            var lv = core_1.cm_internal.findViewForLine(cm, lineNo);
            if (!lv || lv.hidden || !lv.measure)
                return false;
            if (!pre)
                pre = lv.text;
            if (!pre)
                return false;
            if (DEBUG)
                if (!pre.isSameNode(lv.text))
                    console.warn("procLine got different node... " + lineNo);
            var mapInfo = core_1.cm_internal.mapFromLineView(lv, line, lineNo);
            var map = mapInfo.map;
            var nodeCount = map.length / 3;
            var changed = false;
            // change line status
            if (rangesInLine.length === 0) { // inactiveLine
                if (addClass(pre, lineInactiveClassName))
                    changed = true;
            }
            else { // activeLine
                if (rmClass(pre, lineInactiveClassName))
                    changed = true;
            }
            // show or hide tokens
            /**
             * @returns if there are Span Nodes changed
             */
            function changeVisibilityForSpan(span, shallHideTokens, iNodeHint) {
                var changed = false;
                iNodeHint = iNodeHint || 0;
                // iterate the map
                for (var i = iNodeHint; i < nodeCount; i++) {
                    var begin = map[i * 3], end = map[i * 3 + 1];
                    var domNode = map[i * 3 + 2];
                    if (begin === span.head.start) {
                        // find the leading token!
                        if (/formatting-/.test(span.head.type) && domNode.nodeType === Node.TEXT_NODE) {
                            // if (DEBUG) console.log("DOMNODE", shallHideTokens, domNode, begin, span)
                            // good. this token can be changed
                            var domParent = domNode.parentElement;
                            if (domNode.textContent === "(") {
                                if (shallHideTokens ? addClass(domNode.parentElement.nextSibling, hideClassName) : rmClass(domNode.parentElement.nextSibling, hideClassName)) {
                                    // if (DEBUG) console.log("HEAD DOM PATCHED")
                                    changed = true;
                                }
                            }
                            if (shallHideTokens ? addClass(domParent, hideClassName) : rmClass(domParent, hideClassName)) {
                                // if (DEBUG) console.log("HEAD DOM PATCHED")
                                changed = true;
                            }
                        }
                        //FIXME: if leading formatting token is separated into two, the latter will not be hidden/shown!
                        // search for the tailing token
                        if (span.tail && /formatting-/.test(span.tail.type)) {
                            for (var j = i + 1; j < nodeCount; j++) {
                                var begin_1 = map[j * 3], end_1 = map[j * 3 + 1];
                                var domNode_1 = map[j * 3 + 2];
                                if (begin_1 == span.tail.start) {
                                    // if (DEBUG) console.log("TAIL DOM PATCHED", domNode)
                                    if (domNode_1.nodeType === Node.TEXT_NODE) {
                                        // good. this token can be changed
                                        var domParent = domNode_1.parentElement;
                                        if (domNode.textContent === "```") {
                                            if (shallHideTokens ? addClass(domParent, hideClassName) : rmClass(domParent, hideClassName)) {
                                                changed = true;
                                            }
                                        }
                                        if (shallHideTokens ? addClass(domParent, hideClassName) : rmClass(domParent, hideClassName)) {
                                            changed = true;
                                        }
                                    }
                                }
                                if (begin_1 >= span.tail.end)
                                    break;
                            }
                        }
                    }
                    // whoops, next time we can start searching since here
                    // return the hint value
                    if (begin >= span.begin)
                        break;
                }
                return changed;
            }
            var spans = line_spans_1.getLineSpanExtractor(cm).extract(lineNo);
            var iNodeHint = 0;
            for (var iSpan = 0; iSpan < spans.length; iSpan++) {
                var span = spans[iSpan];
                if (this.tokenTypes.indexOf(span.type) === -1)
                    continue; // not-interested span type
                /* TODO: Use AST, instead of crafted Position */
                var spanRange = [{ line: lineNo, ch: span.begin }, { line: lineNo, ch: span.end }];
                /* TODO: If use AST, compute `spanBeginCharInCurrentLine` in another way */
                var spanBeginCharInCurrentLine = span.begin;
                while (iNodeHint < nodeCount && map[iNodeHint * 3 + 1] < spanBeginCharInCurrentLine)
                    iNodeHint++;
                var shallHideTokens = true;
                for (var iLineRange = 0; iLineRange < rangesInLine.length; iLineRange++) {
                    var userRange = rangesInLine[iLineRange];
                    if (cm_utils_1.rangesIntersect(spanRange, userRange)) {
                        shallHideTokens = false;
                        break;
                    }
                }

                // PATCHED, if we are at the task line and before checkbox, don't hide tokens.
                if (span.type === 'task') {
                    var cursorPos = cm.getCursor();
                    if (cursorPos.line === lineNo) {
                        var tokens = cm.getLineTokens(lineNo);
                        // Check if cursor is on list formatting that comes before this task
                        for (var i = 0; i < tokens.length; i++) {
                            var token = tokens[i];
                            if (token.type && token.type.indexOf('formatting-list') !== -1) {
                                // If cursor is on the list formatting token
                                if (cursorPos.ch >= token.start && cursorPos.ch <= token.end) {
                                    shallHideTokens = false; // Don't hide task when cursor is on adjacent list
                                    break;
                                }
                            }
                        }
                    }
                }
                // PATCHED

                if (changeVisibilityForSpan(span, shallHideTokens, iNodeHint))
                    changed = true;
            }
            // finally clean the cache (if needed) and report the result
            if (changed) {
                // clean CodeMirror measure cache
                delete lv.measure.heights;
                lv.measure.cache = {};
            }
            return changed;
        };
        HideToken.prototype.updateImmediately = function () {
            var _this = this;
            this.update.stop();
            var cm = this.cm;
            var selections = cm.listSelections();
            var caretAtLines = {};
            var activedLines = {};
            var lastActivedLines = this._rangesInLine;

            // update this._activedLines and caretAtLines
            for (var _i = 0, selections_1 = selections; _i < selections_1.length; _i++) {
                var selection = selections_1[_i];
                var oRange = cm_utils_1.orderedRange(selection);
                var line0 = oRange[0].line, line1 = oRange[1].line;
                caretAtLines[line0] = caretAtLines[line1] = true;
                for (var line = line0; line <= line1; line++) {
                    if (!activedLines[line])
                        activedLines[line] = [oRange];
                    else
                        activedLines[line].push(oRange);
                }
            }
            this._rangesInLine = activedLines;
            if (DEBUG)
                console.log("======= OP START " + Object.keys(activedLines));
            cm.operation(function () {
                // adding "inactive" class
                for (var line in lastActivedLines) {
                    if (activedLines[line])
                        continue; // line is still active. do nothing
                    _this.procLine(~~line); // or, try adding "inactive" class to the <pre>s
                }
                var caretLineChanged = false;
                // process active lines
                for (var line in activedLines) {
                    var lineChanged = _this.procLine(~~line);
                    if (lineChanged && caretAtLines[line])
                        caretLineChanged = true;
                }


                // refresh cursor position if needed
                if (caretLineChanged) {
                    if (DEBUG)
                        console.log("caretLineChanged");
                    cm.refreshCursor();
                    // legacy unstable way to update display and caret position:
                    // updateCursorDisplay(cm, true)
                    // if (cm.hmd.TableAlign && cm.hmd.TableAlign.enabled) cm.hmd.TableAlign.updateStyle()
                }
            });
            if (DEBUG)
                console.log("======= OP END ");
        };
        return HideToken;
    }());
    exports.HideToken = HideToken;
    //#endregion
    /** ADDON GETTER (Singleton Pattern): a editor can have only one HideToken instance */
    exports.getAddon = core_1.Addon.Getter("HideToken", HideToken, exports.defaultOption /** if has options */);
});
