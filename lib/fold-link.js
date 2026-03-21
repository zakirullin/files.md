// HyperMD, copyright (c) by laobubu
// Distributed under an MIT license: http://laobubu.net/HyperMD/LICENSE
//
// DESCRIPTION: Fold and render links
//
// This file is actually for links folding, not emojis.

(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("../core"), require("./fold")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","../core","./fold"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.FoldEmoji = this.HyperMD.FoldEmoji || {}), CodeMirror, HyperMD, HyperMD.Fold);
})(function (require, exports, CodeMirror, core_1, fold_1) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });
    exports.defaultDict = { /* initialized later */};
    exports.defaultChecker = function (text) { return text in exports.defaultDict; };
    exports.defaultRenderer = function (text) {
        var el = document.createElement("span");
        el.textContent = exports.defaultDict[text];
        el.title = text;
        return el;
    };
    /********************************************************************************** */
    //#region Folder
    /**
     * Detect if a token is emoji and fold it
     *
     * @see FolderFunc in ./fold.ts
     */
    exports.EmojiFolder = function (stream, token) {
        if (!token.type || !/ formatting-emoji/.test(token.type))
            return null;
        var cm = stream.cm;
        var from = { line: stream.lineNo, ch: token.start };
        var to = { line: stream.lineNo, ch: token.end };
        var name = token.string; // with ":"
        var addon = exports.getAddon(cm);
        if (!addon.isEmoji(name))
            return null;
        var reqAns = stream.requestRange(from, to);
        if (reqAns !== fold_1.RequestRangeResult.OK)
            return null;
        // now we are ready to fold and render!
        var marker = addon.foldEmoji(name, from, to);
        return marker;
    };
    //#endregion
    fold_1.registerFolder("emoji", exports.EmojiFolder, true);
    exports.defaultOption = {
        myEmoji: {},
        emojiRenderer: exports.defaultRenderer,
        emojiChecker: exports.defaultChecker,
    };
    exports.suggestedOption = {};
    core_1.suggestedEditorConfig.hmdFoldEmoji = exports.suggestedOption;
    CodeMirror.defineOption("hmdFoldEmoji", exports.defaultOption, function (cm, newVal) {
        ///// convert newVal's type to `Partial<Options>`, if it is not.
        if (!newVal) {
            newVal = {};
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
    var FoldEmoji = /** @class */ (function () {
            function FoldEmoji(cm) {
                this.cm = cm;
                // options will be initialized to defaultOption when constructor is finished
            }
            FoldEmoji.prototype.isEmoji = function (text) {
                return text in this.myEmoji || this.emojiChecker(text);
            };
            FoldEmoji.prototype.foldEmoji = function (text, from, to) {
                var cm = this.cm;
                var el = ((text in this.myEmoji) && this.myEmoji[text](text)) || this.emojiRenderer(text);
                if (!el || !el.tagName)
                    return null;
                if (el.className.indexOf('hmd-emoji') === -1)
                    el.className += " hmd-emoji";
                var marker = cm.markText(from, to, {
                    replacedWith: el,
                });
                el.addEventListener("click", fold_1.breakMark.bind(this, cm, marker, 1), false);
                if (el.tagName.toLowerCase() === 'img') {
                    el.addEventListener('load', function () { return marker.changed(); }, false);
                    el.addEventListener('dragstart', function (ev) { return ev.preventDefault(); }, false);
                }
                return marker;
            };
            return FoldEmoji;
        }());
    exports.FoldEmoji = FoldEmoji;
    //#endregion
    /** ADDON GETTER (Singleton Pattern): a editor can have only one FoldEmoji instance */
    exports.getAddon = core_1.Addon.Getter("FoldEmoji", FoldEmoji, exports.defaultOption /** if has options */);
    /********************************************************************************** */
    //#region initialize compact emoji dict
    (function (dest) {
        var parts = [];
        var matRE = /([-\w]+:)([^;]+);/g;
        var t;
        for (var i = 0; i < parts.length; i++) {
            matRE.lastIndex = 0;
            while (t = matRE.exec(parts[i])) {
                dest[':' + t[1]] = t[2];
            }
        }
    })(exports.defaultDict);
});
//#endregion
