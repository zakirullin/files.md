(function (mod) {
    mod(
        (this.CompleteEmoji = {}),
        CodeMirror,
        HyperMD.FoldEmoji
    );
})(function (exports, CodeMirror, FoldEmoji) {
  /**
   * Create a hint function for codemirror/addon/hint/show-hint
   * and a HyperMD editor with fold-emoji actived
   *
   * @see https://codemirror.net/doc/manual.html#addon_show-hint
   */
  exports.createHintFunc = function () {
    var editor = null
    var defaultDict = {}

    var previewShown = false
    var previewContainer = document.createElement('div')
    previewContainer.setAttribute('class', 'CodeMirror-hints HyperMD-complete-preview')

    return function (cm, options) {
      let cursor = cm.getCursor(), line = cm.getLine(cursor.line);
      // PATCHED don't mixup with checkboxse
      if (/^\s*-\s\[\s/.test(line)) {
        hidePreview()
        return null
      }

      let start = cursor.ch, end = cursor.ch;

      // while (start && /[-\w:]/.test(line.charAt(start - 1)))--start
      // while (end < line.length && /[-\w:]/.test(line.charAt(end)))++end

      // PATCHED (removed \s, allowed us too see links dialog always)
      const unicodeWordRegex = /[\p{L}\p{N}_\s:-]/u; // \p{L} matches any letter, \p{N} matches any number

      while (start && unicodeWordRegex.test(line.charAt(start - 1))) --start;
      while (end < line.length && unicodeWordRegex.test(line.charAt(end))) ++end;

      // if (start === end) {
      //   hidePreview()
      //   return null
      // }

      let word = line.slice(start, cursor.ch).toLowerCase()
      let wordEmpty = word.length === 0

      /** @type {Array<Record<string,string>>} */
      // var dicts = [defaultDict] // for now we only use links autocompletion
      var dicts = []
      var myEmojiDict = (cm.getOption('hmdFoldEmoji') || {}).myEmoji
      if (myEmojiDict) dicts.push(myEmojiDict())

      var result = {
        list: [],
        from: CodeMirror.Pos(cursor.line, start),
        to: CodeMirror.Pos(cursor.line, end)
      }

      var list = result.list
      for (var i = 0; i < dicts.length; i++) {
        var dict = dicts[i]
        if (!dict) continue
        for (var key in dict) {
          if (wordEmpty || key.slice(0, word.length) === word || key.toLowerCase().includes(word.toLowerCase())) {
            list.push({text: dict[key], displayText: key})
          }
        }
      }

      CodeMirror.on(result, "select", showPreview)
      CodeMirror.on(result, "close", hidePreview)

      return result
    }

    function hidePreview() {
      if (previewShown) {
        document.body.removeChild(previewContainer)
        previewShown = false
      }
    }

    /**
     *
     * @param {string} completion
     * @param {HTMLElement} element
     */
    function showPreview(completion, element) {
      return;

      var foldEmoji = FoldEmoji.getAddon(editor)

      /** @type {Node} */
      var newNode =
          ((completion in foldEmoji.myEmoji) && foldEmoji.myEmoji[completion](completion))
          || foldEmoji.emojiRenderer(completion)
          || document.createTextNode(defaultDict[completion])
          || null

      if (newNode) { // yes, this is a emoji
        if (!previewShown) {
          previewShown = true
          document.body.appendChild(previewContainer)
        }

        var oldNode = previewContainer.firstChild
        if (oldNode) previewContainer.removeChild(oldNode)
        previewContainer.appendChild(newNode)

        var loc = element.parentElement.style
        var pcStyle = previewContainer.style
        pcStyle.left = Number.parseFloat(loc.left) + element.parentElement.offsetWidth + 'px'
        pcStyle.top = loc.top
      } else {
        hidePreview()
      }
    }
  }
})