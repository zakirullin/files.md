// HyperMD, copyright (c) by laobubu
// Distributed under an MIT license: http://laobubu.net/HyperMD/LICENSE
//
// DESCRIPTION: Align Table Columns
//

(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("../core")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","../core"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.TableAlign = this.HyperMD.TableAlign || {}), CodeMirror, HyperMD);
})(function (require, exports, CodeMirror, core_1) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });
    exports.defaultOption = {
        enabled: false,
    };
    exports.suggestedOption = {
        enabled: true,
    };
    core_1.suggestedEditorConfig.hmdTableAlign = exports.suggestedOption;
    core_1.normalVisualConfig.hmdTableAlign = false;
    CodeMirror.defineOption("hmdTableAlign", exports.defaultOption, function (cm, newVal) {
        var enabled = !!newVal;
        ///// convert newVal's type to `Partial<Options>`, if it is not.
        if (!enabled || typeof newVal === "boolean") {
            newVal = { enabled: enabled };
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
    var TableAlign = /** @class */ (function () {
            function TableAlign(cm) {
                // options will be initialized to defaultOption (if exists)
                // add your code here
                var _this = this;
                this.cm = cm;
                this.styleEl = document.createElement("style");
                /**
                 * Remeasure visible columns, update CSS style to make columns aligned
                 *
                 * (This is a debounced function)
                 */
                // PATCHED: Request-Animation-Frame-throttle instead of timed debounce. tableID is
                // line-number-based (hypermd.js: "T" + line), so any line shift
                // above a table changes the CSS string and the table line is
                // re-rendered with a new id. With a 100ms debounce the new column
                // DOM lived for ~100ms without matching CSS — visible flicker.
                // requestAnimationFrame batches bursty updates (many `update`
                // events collapse into one) AND runs before the browser paints,
                // so the new <style> is applied in the same frame as the render.
                var rafScheduled = false;
                this.updateStyle = function () {
                    if (rafScheduled) return;
                    rafScheduled = true;
                    requestAnimationFrame(function () {
                        rafScheduled = false;
                        if (!_this.enabled) return;
                        var measures = _this.measure();
                        var css = _this.makeCSS(measures);
                        if (css === _this._lastCSS) return;
                        _this.styleEl.textContent = _this._lastCSS = css;
                        _this.refreshHeights();
                    });
                };
                /** CodeMirror renderLine event handler */
                this._procLine = function (cm, line, el) {
                    if (!el.querySelector('.cm-hmd-table-sep'))
                        return;
                    var lineSpan = el.firstElementChild;
                    var lineSpanChildren = Array.prototype.slice.call(lineSpan.childNodes, 0);
                    var eolState = cm.getStateAfter(line.lineNo());
                    var columnStyles = eolState.hmdTableColumns;
                    var tableID = eolState.hmdTableID;
                    var columnIdx = eolState.hmdTable === 2 /* NORMAL */ ? -1 : 0;
                    var columnSpan = _this.makeColumn(columnIdx, columnStyles[columnIdx] || "dummy", tableID);
                    var columnContentSpan = columnSpan.firstElementChild;
                    for (var _i = 0, lineSpanChildren_1 = lineSpanChildren; _i < lineSpanChildren_1.length; _i++) {
                        var el_1 = lineSpanChildren_1[_i];
                        var elClass = el_1.nodeType === Node.ELEMENT_NODE && el_1.className || "";
                        if (/cm-hmd-table-sep/.test(elClass)) {
                            // found a "|", and a column is finished
                            columnIdx++;
                            columnSpan.appendChild(columnContentSpan);
                            lineSpan.appendChild(columnSpan);
                            lineSpan.appendChild(el_1);
                            columnSpan = _this.makeColumn(columnIdx, columnStyles[columnIdx] || "dummy", tableID);
                            columnContentSpan = columnSpan.firstElementChild;
                        }
                        else {
                            columnContentSpan.appendChild(el_1);
                        }
                    }
                    columnSpan.appendChild(columnContentSpan);
                    lineSpan.appendChild(columnSpan);
                };
                new core_1.FlipFlop(
                    /* ON  */ function () {
                        cm.on("renderLine", _this._procLine);
                        cm.on("update", _this.updateStyle);
                        cm.refresh();
                        document.head.appendChild(_this.styleEl);
                    },
                    /* OFF */ function () {
                        cm.off("renderLine", _this._procLine);
                        cm.off("update", _this.updateStyle);
                        document.head.removeChild(_this.styleEl);
                    }).bind(this, "enabled", true);
            }
            /**
             * create a <span> container as column,
             * note that put content into column.firstElementChild
             */
            TableAlign.prototype.makeColumn = function (index, style, tableID) {
                var span = document.createElement("span");
                span.className = "hmd-table-column hmd-table-column-" + index + " hmd-table-column-" + style;
                span.setAttribute("data-column", "" + index);
                span.setAttribute("data-table-id", tableID);
                var span2 = document.createElement("span");
                span2.className = "hmd-table-column-content";
                span2.setAttribute("data-column", "" + index);
                span.appendChild(span2);
                return span;
            };
            /** Measure all visible tables and columns */
            TableAlign.prototype.measure = function () {
                var cm = this.cm;
                var lineDiv = cm.display.lineDiv; // contains every <pre> line
                // PATCHED: cells word-wrap under the generated max-width caps, so a
                // plain offsetWidth read would return the capped width and the
                // natural width would be lost. The hmd-table-measure class lifts
                // the caps (max-width: none, white-space: pre) for the duration of
                // the read. We run inside requestAnimationFrame, before paint, so
                // the temporary layout is never visible.
                lineDiv.classList.add("hmd-table-measure");
                // The editor centers content via large side paddings on the
                // scroller, so derive the width available to a table from a
                // rendered row's own line box rather than from the scroller.
                this._availWidth = 0;
                var rowPre = lineDiv.querySelector("pre.HyperMD-table-row");
                if (rowPre) {
                    var preStyle = window.getComputedStyle(rowPre);
                    this._availWidth = rowPre.clientWidth
                        - (parseFloat(preStyle.paddingLeft) || 0)
                        - (parseFloat(preStyle.paddingRight) || 0);
                }
                var contentSpans = lineDiv.querySelectorAll(".hmd-table-column-content");
                /** every table's every column's width in px */
                var ans = {};
                for (var i = 0; i < contentSpans.length; i++) {
                    var contentSpan = contentSpans[i];
                    var column = contentSpan.parentElement;
                    var tableID = column.getAttribute("data-table-id");
                    var columnIdx = ~~column.getAttribute("data-column");
                    var width = contentSpan.offsetWidth + 1; // +1 because browsers turn 311.3 into 312
                    if (!(tableID in ans))
                        ans[tableID] = [];
                    var columnWidths = ans[tableID];
                    while (columnWidths.length <= columnIdx)
                        columnWidths.push(0);
                    if (columnWidths[columnIdx] < width)
                        columnWidths[columnIdx] = width;
                }
                lineDiv.classList.remove("hmd-table-measure");
                return ans;
            };
            /**
             * PATCHED: cap column widths so a table fits the editor width and
             * its cells word-wrap instead of growing horizontally (issue #43).
             * Columns narrower than their fair share keep their natural width;
             * the remaining space is split evenly among the wider ones.
             */
            TableAlign.prototype.capWidths = function (widths, available) {
                var minCap = 48;
                var total = 0;
                for (var i = 0; i < widths.length; i++)
                    total += widths[i];
                if (available <= 0 || total <= available)
                    return widths;
                var sorted = widths.slice().sort(function (a, b) { return a - b; });
                var remaining = available;
                var cap = minCap;
                for (var i = 0; i < sorted.length; i++) {
                    cap = remaining / (sorted.length - i);
                    if (sorted[i] > cap)
                        break;
                    remaining -= sorted[i];
                }
                cap = Math.max(Math.floor(cap), minCap);
                return widths.map(function (w) { return Math.min(w, cap); });
            };
            /** Generate CSS */
            TableAlign.prototype.makeCSS = function (measures) {
                var rules = [];
                for (var tableID in measures) {
                    // Column borders and rounding eat a few px per column.
                    var overhead = measures[tableID].length * 2 + 8;
                    var columnWidths = this.capWidths(measures[tableID], this._availWidth - overhead);
                    var rulePrefix = "pre.HyperMD-table-row.HyperMD-table_" + tableID + " .hmd-table-column-";
                    for (var columnIdx = 0; columnIdx < columnWidths.length; columnIdx++) {
                        var width = columnWidths[columnIdx] + .5;
                        rules.push("" + rulePrefix + columnIdx + " { min-width: " + width + "px }");
                        rules.push("" + rulePrefix + columnIdx + " .hmd-table-column-content { max-width: " + width + "px }");
                    }
                }
                return rules.join("\n");
            };
            /**
             * PATCHED: a width change can now change a table row's height (cells
             * word-wrap), but CodeMirror re-measures line heights only during its
             * own update cycle, which has already run by the time our CSS lands.
             * Stale heights break cursor placement and scrolling for everything
             * below the table, so detect the mismatch and refresh.
             */
            TableAlign.prototype.refreshHeights = function () {
                var cm = this.cm;
                var view = cm.display.view;
                for (var i = 0; i < view.length; i++) {
                    var lineView = view[i];
                    if (lineView.hidden || lineView.rest || !lineView.node)
                        continue;
                    if (lineView.line.widgets && lineView.line.widgets.length)
                        continue;
                    if (!lineView.node.querySelector(".hmd-table-column"))
                        continue;
                    var rect = lineView.node.getBoundingClientRect();
                    if (Math.abs((rect.bottom - rect.top) - lineView.line.height) > 1) {
                        cm.refresh();
                        return;
                    }
                }
            };
            return TableAlign;
        }());
    exports.TableAlign = TableAlign;
    //#endregion
    /** ADDON GETTER (Singleton Pattern): a editor can have only one TableAlign instance */
    exports.getAddon = core_1.Addon.Getter("TableAlign", TableAlign, exports.defaultOption);
});
// App-level table UX helpers (not part of the HyperMD table-align addon).

// Hovering a table's right edge shows a thin
// vertical strip with a plus that adds a column, the bottom edge shows a
// horizontal one that adds a row. With Cmd/Ctrl held the planks flip to
// minus and remove the last column / row instead.
function initTablePlus(cm) {
    const wrapper = cm.getWrapperElement();
    const colBtn = document.createElement('div');
    colBtn.className = 'table-plus table-plus-col';
    colBtn.textContent = '+';
    const rowBtn = document.createElement('div');
    rowBtn.className = 'table-plus table-plus-row';
    rowBtn.textContent = '+';
    wrapper.appendChild(colBtn);
    wrapper.appendChild(rowBtn);

    const THICK = 16; // plank thickness, also its hover band, px

    function hide() {
        colBtn.classList.remove('show');
        rowBtn.classList.remove('show');
    }

    function setMinusMode(on) {
        colBtn.textContent = rowBtn.textContent = on ? '−' : '+';
    }

    // Cmd/Ctrl pressed while the mouse rests over a plank; mousemove below
    // keeps the glyph in sync when the mouse enters with the key held.
    document.addEventListener('keydown', function (e) {
        if (e.key === 'Meta' || e.key === 'Control') setMinusMode(true);
    });
    document.addEventListener('keyup', function (e) {
        if (e.key === 'Meta' || e.key === 'Control') setMinusMode(false);
    });

    // Visible tables as client-rect blocks, keyed by table id
    function tableBlocks() {
        const blocks = [];
        for (const pre of cm.display.lineDiv.querySelectorAll('pre.HyperMD-table-row')) {
            const id = (pre.className.match(/HyperMD-table_(T\d+)/) || [])[1];
            if (!id) continue;
            let last = blocks[blocks.length - 1];
            if (!last || last.id !== id) {
                last = {id, top: Infinity, bottom: 0, left: Infinity, right: 0};
                blocks.push(last);
            }
            // The bordered cells, not the pre, define the table box - the
            // pre includes line padding above and below the borders. The
            // |---| separator row is collapsed (no padding, no borders), its
            // cells sit outside the visible table box, so skip it.
            if (/HyperMD-table-row-1\b/.test(pre.className)) continue;
            for (const col of pre.querySelectorAll('.hmd-table-column:not(.hmd-table-column-dummy)')) {
                const r = col.getBoundingClientRect();
                if (r.width === 0) continue;
                last.left = Math.min(last.left, r.left);
                last.right = Math.max(last.right, r.right);
                last.top = Math.min(last.top, r.top);
                last.bottom = Math.max(last.bottom, r.bottom);
            }
        }
        return blocks.filter(b => b.right > b.left);
    }

    function update(mx, my) {
        const wrapRect = wrapper.getBoundingClientRect();
        let col = null, row = null;
        // The hover bands match the planks' footprints exactly (planks sit
        // 1px over the table border), so a shown plank is always clickable.
        for (const b of tableBlocks()) {
            if (my >= b.top && my <= b.bottom && mx >= b.right - 1 && mx <= b.right - 1 + THICK) col = b;
            if (mx >= b.left && mx <= b.right && my >= b.bottom - 1 && my <= b.bottom - 1 + THICK) row = b;
        }
        if (col) {
            colBtn.classList.add('show');
            colBtn.style.left = (col.right - wrapRect.left - 1) + 'px';
            colBtn.style.top = (col.top - wrapRect.top) + 'px';
            colBtn.style.height = (col.bottom - col.top) + 'px';
            colBtn._table = col;
        } else {
            colBtn.classList.remove('show');
        }
        if (row) {
            rowBtn.classList.add('show');
            rowBtn.style.left = (row.left - wrapRect.left) + 'px';
            rowBtn.style.top = (row.bottom - wrapRect.top - 1) + 'px';
            rowBtn.style.width = (row.right - row.left) + 'px';
            rowBtn._table = row;
        } else {
            rowBtn.classList.remove('show');
        }
    }

    let raf = 0;
    wrapper.addEventListener('mousemove', function (e) {
        if (raf) return;
        raf = requestAnimationFrame(function () {
            raf = 0;
            setMinusMode(e.metaKey || e.ctrlKey);
            update(e.clientX, e.clientY);
        });
    });
    wrapper.addEventListener('mouseleave', hide);
    cm.on('scroll', hide);
    cm.on('change', hide);
    hide();

    colBtn.addEventListener('mousedown', function (e) {
        e.preventDefault();
        e.stopPropagation();
        const range = tableLineRange(cm, cm.lineAtHeight(colBtn._table.top + 1, 'window'));
        if (!range) return;
        if (e.metaKey || e.ctrlKey) tableRemoveColumn(cm, range);
        else tableAddColumn(cm, range);
    });
    rowBtn.addEventListener('mousedown', function (e) {
        e.preventDefault();
        e.stopPropagation();
        const range = tableLineRange(cm, cm.lineAtHeight(rowBtn._table.top + 1, 'window'));
        if (!range) return;
        if (e.metaKey || e.ctrlKey) tableRemoveRow(cm, range);
        else tableAddRow(cm, range);
    });
}

// Cmd/Ctrl+A inside a table cell selects the cell's content first;
// an empty cell consumes the key without selecting anything, and when the
// cell is already selected it returns false so the handler falls through
// to the usual select-all.
function tableSelectCell(cm) {
    const cursor = cm.getCursor();
    if (!cm.getStateAfter(cursor.line).hmdTable) return false;
    const text = cm.getLine(cursor.line);
    let start = text.lastIndexOf('|', Math.max(cursor.ch - 1, 0)) + 1;
    let end = text.indexOf('|', cursor.ch);
    if (end < 0) end = text.length;
    while (start < end && text[start] === ' ') start++;
    while (end > start && text[end - 1] === ' ') end--;
    if (start === end) return true;
    const from = {line: cursor.line, ch: start};
    const to = {line: cursor.line, ch: end};
    const sel = cm.listSelections()[0];
    if (sel && CodeMirror.cmpPos(sel.from(), from) === 0 && CodeMirror.cmpPos(sel.to(), to) === 0) {
        return false;
    }
    cm.setSelection(from, to);
    return true;
}

// Insert an empty 2x2 table at the cursor and focus its first cell
function tableInsert(cm) {
    const cursor = cm.getCursor();
    const line = cm.getLine(cursor.line);
    const table = '|  |  |\n| --- | --- |\n|  |  |';
    cm.operation(function () {
        if (line.trim() === '') {
            cm.replaceRange(table, {line: cursor.line, ch: 0}, {line: cursor.line, ch: line.length});
            cm.setCursor({line: cursor.line, ch: 2});
        } else {
            cm.replaceRange('\n' + table, {line: cursor.line, ch: line.length});
            cm.setCursor({line: cursor.line + 1, ch: 2});
        }
    });
}

// Enter inside a table cell moves focus to the same column's cell one row
// below (skipping the |---| separator), selecting its content if there is
// any. Returns false on the last row so the default Enter applies.
function tableEnterCell(cm) {
    const cursor = cm.getCursor();
    if (!cm.getStateAfter(cursor.line).hmdTable) return false;
    const range = tableLineRange(cm, cursor.line);
    if (!range) return false;
    let target = cursor.line + 1;
    if (target === range.from + 1) target++;
    if (target > range.to) return false;
    const pipes = (cm.getLine(cursor.line).slice(0, cursor.ch).match(/\|/g) || []).length;
    const text = cm.getLine(target);
    let start = 0;
    for (let i = 0, seen = 0; i < text.length && seen < pipes; i++) {
        if (text[i] === '|') {
            seen++;
            start = i + 1;
        }
    }
    const cellStart = start; // right after the opening pipe
    let cellEnd = text.indexOf('|', start);
    if (cellEnd < 0) cellEnd = text.length;
    let from = cellStart, to = cellEnd;
    while (from < to && text[from] === ' ') from++;
    while (to > from && text[to - 1] === ' ') to--;
    if (from === to) {
        // Empty cell: land the cursor between the padding spaces, not glued
        // to the pipe. Normalise to " cursor " (add spaces if missing).
        if (text.slice(cellStart, cellEnd) !== '  ') {
            cm.replaceRange('  ', {line: target, ch: cellStart}, {line: target, ch: cellEnd});
        }
        cm.setCursor({line: target, ch: cellStart + 1});
    } else {
        cm.setSelection({line: target, ch: from}, {line: target, ch: to});
    }
    return true;
}

// Expand a line known to be inside a table to the table's full line range
function tableLineRange(cm, line) {
    const state = cm.getStateAfter(line);
    if (!state.hmdTable) return null;
    const id = state.hmdTableID;
    const inTable = (l) => {
        const st = cm.getStateAfter(l);
        return st.hmdTable && st.hmdTableID === id;
    };
    let from = line, to = line;
    while (from > cm.firstLine() && inTable(from - 1)) from--;
    while (to < cm.lastLine() && inTable(to + 1)) to++;
    return {from, to};
}

// Append an empty column to every row of the table
function tableAddColumn(cm, range) {
    cm.operation(function () {
        for (let l = range.from; l <= range.to; l++) {
            const text = cm.getLine(l);
            const isSep = l === range.from + 1; // the |---|---| row
            if (/\|\s*$/.test(text)) {
                cm.replaceRange(isSep ? ' --- |' : '  |', {line: l, ch: text.lastIndexOf('|') + 1});
            } else {
                cm.replaceRange(isSep ? ' | ---' : ' |  ', {line: l, ch: text.length});
            }
        }
        const header = cm.getLine(range.from);
        const inNewCell = /\|\s*$/.test(header) ? header.lastIndexOf('|') - 1 : header.length;
        cm.setCursor({line: range.from, ch: inNewCell});
    });
    cm.focus();
}

// Append an empty row after the table's last row
function tableAddRow(cm, range) {
    const state = cm.getStateAfter(range.from);
    const cols = (state.hmdTableColumns || []).length || 1;
    const normal = state.hmdTable === 2; // table with outer pipes
    const row = normal ? '|' + '  |'.repeat(cols) : Array(cols).fill('  ').join('|');
    cm.operation(function () {
        cm.replaceRange('\n' + row, {line: range.to, ch: cm.getLine(range.to).length});
        cm.setCursor({line: range.to + 1, ch: normal ? 2 : 1});
    });
    cm.focus();
}

// Remove the last column from every row of the table. Cursors
// inside the removed cells get clipped to the line end by CodeMirror.
function tableRemoveColumn(cm, range) {
    const state = cm.getStateAfter(range.from);
    if ((state.hmdTableColumns || []).length <= 1) return;
    cm.operation(function () {
        for (let l = range.from; l <= range.to; l++) {
            const text = cm.getLine(l);
            const lastPipe = text.lastIndexOf('|');
            if (lastPipe < 0) continue;
            if (/\|\s*$/.test(text)) {
                const prevPipe = text.lastIndexOf('|', lastPipe - 1);
                if (prevPipe < 0) continue;
                cm.replaceRange('|', {line: l, ch: prevPipe}, {line: l, ch: text.length});
            } else {
                const cut = text.slice(0, lastPipe).replace(/\s+$/, '').length;
                cm.replaceRange('', {line: l, ch: cut}, {line: l, ch: text.length});
            }
        }
    });
    cm.focus();
}

// Remove the table's last row, never the header or the separator
function tableRemoveRow(cm, range) {
    if (range.to <= range.from + 1) return;
    cm.operation(function () {
        cm.replaceRange('',
            {line: range.to - 1, ch: cm.getLine(range.to - 1).length},
            {line: range.to, ch: cm.getLine(range.to).length});
    });
    cm.focus();
}
