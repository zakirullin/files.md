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
                this.updateStyle = core_1.debounce(function () {
                    if (!_this.enabled)
                        return;
                    var cm = _this.cm;
                    var measures = _this.measure();
                    var css = _this.makeCSS(measures);
                    if (css === _this._lastCSS)
                        return;
                    _this.styleEl.textContent = _this._lastCSS = css;
                    cm.refresh();
                }, 100);
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
                return ans;
            };
            /** Generate CSS */
            TableAlign.prototype.makeCSS = function (measures) {
                var rules = [];
                for (var tableID in measures) {
                    var columnWidths = measures[tableID];
                    var rulePrefix = "pre.HyperMD-table-row.HyperMD-table_" + tableID + " .hmd-table-column-";
                    for (var columnIdx = 0; columnIdx < columnWidths.length; columnIdx++) {
                        var width = columnWidths[columnIdx];
                        rules.push("" + rulePrefix + columnIdx + " { min-width: " + (width + .5) + "px }");
                    }
                }
                return rules.join("\n");
            };
            return TableAlign;
        }());
    exports.TableAlign = TableAlign;
    //#endregion
    /** ADDON GETTER (Singleton Pattern): a editor can have only one TableAlign instance */
    exports.getAddon = core_1.Addon.Getter("TableAlign", TableAlign, exports.defaultOption);
});