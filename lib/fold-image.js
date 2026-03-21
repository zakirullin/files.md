(function() {
    "use strict";

    if (!window.HyperMD?.Fold) {
        return;
    }
    let fold = window.HyperMD.Fold;

    function ImageFolder(stream, token) {
        let cm = stream.cm;
        let imgRE = /\bimage-marker\b/;
        let urlRE = /\bformatting-link-string\b/; // matches the parentheses
        if (imgRE.test(token.type) && token.string === "!") {
            let lineNo = stream.lineNo;
            // find the begin and end of url part
            let url_begin = stream.findNext(urlRE);
            let url_end = stream.findNext(urlRE, url_begin.i_token + 1);
            let from = {line: lineNo, ch: token.start};
            let to = {line: lineNo, ch: url_end.token.end};

            let rngReq = stream.requestRange(from, to, from, from);
            if (rngReq === fold.RequestRangeResult.OK) {
                // That fixes blinking on select, for some reason range CI is returned even though cursor is outside of our tokens
                if (cm.somethingSelected()) {
                    return null;
                }

                let url;
                let title;
                { // extract the URL
                    let rawurl = cm.getRange(// get the URL or footnote name in the parentheses
                        {line: lineNo, ch: url_begin.token.start + 1}, {line: lineNo, ch: url_end.token.start});
                    if (url_end.token.string === "]") {
                        let tmp = cm.hmdReadLink(rawurl, lineNo);
                        if (!tmp)
                            return null; // Yup! bad URL?!
                        rawurl = tmp.content;
                    }
                    url = cm.hmdResolveURL(rawurl);
                }
                { // extract the title
                    title = cm.getRange({line: lineNo, ch: from.ch + 2}, {line: lineNo, ch: url_begin.token.start - 1});
                }
                let img = document.createElement("img");
                img.style.cursor = "pointer";
                // PATCHED, we don't want blank line with the cursor after image
                let wrapper = document.createElement("span");
                wrapper.style.display = "inline-flex";
                wrapper.style.justifyContent = "center";
                wrapper.style.alignItems = "center";
                wrapper.style.width = "100%";
                wrapper.style.textAlign = "center";
                wrapper.appendChild(img);
                wrapper.addEventListener('click', function () {
                    cm.focus();
                    const lineNo = from.line;
                    const lineLength = cm.getLine(lineNo).length;
                    cm.setCursor({line: lineNo, ch: lineLength});
                });

                let marker = cm.markText(from, to, {
                    clearOnEnter: false,
                    collapsed: true,
                    // PATCHED, was img
                    replacedWith: img,
                });
                img.addEventListener('click', function (e) {
                    e.stopPropagation();
                    let modal = document.createElement("div");
                    modal.style.position = "fixed";
                    modal.style.top = "0";
                    modal.style.left = "0";
                    modal.style.width = "100vw";
                    modal.style.height = "100vh";
                    modal.style.backgroundColor = "rgba(0, 0, 0, 0.8)";
                    modal.style.display = "flex";
                    modal.style.justifyContent = "center";
                    modal.style.alignItems = "center";
                    modal.style.zIndex = "1000";

                    let imgPreview = document.createElement("img");
                    imgPreview.src = img.src;
                    imgPreview.className = "hmd-image-preview";
                    imgPreview.style.maxWidth = "90%";
                    imgPreview.style.maxHeight = "90%";
                    imgPreview.style.borderRadius = "8px";

                    modal.appendChild(imgPreview);

                    const closeModal = () => {
                        document.body.removeChild(modal);
                        document.removeEventListener("keydown", handleKeyDown, true);
                    };

                    modal.addEventListener("click", closeModal,);
                    const handleKeyDown = (event) => {
                        if (event.key === "Escape") {
                            event.stopPropagation();
                            event.preventDefault();
                            closeModal();
                            currentEditor.focus();
                        }
                    };
                    document.addEventListener("keydown", handleKeyDown, true);

                    document.body.appendChild(modal);
                }, false);
                img.addEventListener('load', function () {
                    img.classList.remove("hmd-image-loading");
                    marker.changed();
                }, false);
                img.addEventListener('error', function () {
                    img.classList.remove("hmd-image-loading");
                    img.classList.add("hmd-image-error");
                    marker.changed();
                }, false);
                // img.addEventListener('click', function () { return fold_1.breakMark(cm, marker); }, false);
                img.className = "hmd-image hmd-image-loading";
                img.src = url;
                img.title = title;
                return marker;
            }
            // else {
            //     // if (DEBUG) {
            //     //     console.log("[image]FAILED TO REQUEST RANGE: ", rngReq);
            //     // }
            // }
            // PATCHED, add ![[img/link]] support, TODO it is copypaste from above
        } else if (token.string === "!") {
            // <span >!<span
            // className="cm-formatting cm-formatting-link cm-link cm-hmd-barelink">[[</span><span
            // className="cm-string cm-url cm-hmd-barelink">media/2025-07-06T10-42-31-614Z.png</span><span
            // className="cm-formatting cm-formatting-link cm-link cm-hmd-barelink">]]</span></span>
            let urlRE = /\burl\b/;

            let lineNo = stream.lineNo;
            // find the begin and end of url part
            let url_begin = stream.findNext(urlRE);
            if (url_begin === null) {
                return;
            }
            let url_end = stream.findNext((token) => {return token.string === ']]'});
            // PATCHED, for some reason https://maps.app.goo.gl/HfuMcLjYvTyZTvmM8 in the middle of the text produced
            // an error due to url_end = null.
            if (url_end === null) {
                return;
            }

            let from = {line: lineNo, ch: token.start};
            let to = {line: lineNo, ch: url_end.token.end};


            let rngReq = stream.requestRange(from, to, from, from);
            if (rngReq === fold.RequestRangeResult.OK) {
                // That fixes blinking on select, for some reason range CI is returned even though cursor is outside of our tokens
                if (cm.somethingSelected()) {
                    return null;
                }
                let rawurl = cm.getRange(
                    {line: lineNo, ch: url_begin.token.start},
                    {line: lineNo, ch: url_end.token.start}
                );
                let url = cm.hmdResolveURL(rawurl);

                let img = document.createElement("img");
                img.style.cursor = "pointer";
                // PATCHED, we don't want blank line with the cursor after image
                let wrapper = document.createElement("span");
                wrapper.style.display = "inline-flex";
                wrapper.style.justifyContent = "center";
                wrapper.style.alignItems = "center";
                wrapper.style.width = "100%";
                wrapper.style.textAlign = "center";
                wrapper.appendChild(img);
                wrapper.addEventListener('click', function () {
                    cm.focus();
                    const lineNo = from.line;
                    const lineLength = cm.getLine(lineNo).length;
                    cm.setCursor({line: lineNo, ch: lineLength});
                });

                let marker = cm.markText(from, to, {
                    clearOnEnter: false,
                    collapsed: true,
                    replacedWith: img,
                });
                img.addEventListener('click', function (e) {
                    e.stopPropagation();
                    let modal = document.createElement("div");
                    modal.style.position = "fixed";
                    modal.style.top = "0";
                    modal.style.left = "0";
                    modal.style.width = "100vw";
                    modal.style.height = "100vh";
                    modal.style.backgroundColor = "rgba(0, 0, 0, 0.8)";
                    modal.style.display = "flex";
                    modal.style.justifyContent = "center";
                    modal.style.alignItems = "center";
                    modal.style.zIndex = "1000";

                    let imgPreview = document.createElement("img");
                    imgPreview.src = img.src;
                    imgPreview.className = "hmd-image-preview";
                    imgPreview.style.maxWidth = "90%";
                    imgPreview.style.maxHeight = "90%";
                    imgPreview.style.borderRadius = "8px";

                    modal.appendChild(imgPreview);

                    const closeModal = () => {
                        document.body.removeChild(modal);
                        document.removeEventListener("keydown", handleKeyDown, true);
                    };

                    modal.addEventListener("click", closeModal,);
                    const handleKeyDown = (event) => {
                        if (event.key === "Escape") {
                            event.stopPropagation();
                            event.preventDefault();
                            closeModal();
                            currentEditor.focus();
                        }
                    };
                    document.addEventListener("keydown", handleKeyDown, true);

                    document.body.appendChild(modal);
                }, false);
                img.addEventListener('load', function () {
                    img.classList.remove("hmd-image-loading");
                    marker.changed();
                }, false);
                img.addEventListener('error', function () {
                    img.classList.remove("hmd-image-loading");
                    img.classList.add("hmd-image-error");
                    marker.changed();
                }, false);
                // img.addEventListener('click', function () { return fold_1.breakMark(cm, marker); }, false);
                img.className = "hmd-image hmd-image-loading";
                img.src = url;
                return marker;
            }
        }
        return null;
    }

    // Register with HyperMD
    fold.registerFolder("image", ImageFolder, true);
})();