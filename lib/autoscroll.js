let scrollInterval;
let isSelecting = false;
const resistance = 120; // the more it is, the slower scrolling is
const scrollMargin = 30;
let lastMousePos = null;

function startAutoScroll(direction, editor) {
    if (scrollInterval) return; // Already scrolling

    scrollInterval = setInterval(() => {
        const scrollInfo = editor.getScrollInfo();
        const lineHeight = editor.defaultTextHeight();

        if (direction === 'up') {
            editor.scrollTo(null, Math.max(0, scrollInfo.top - lineHeight));
        } else if (direction === 'down') {
            const maxScroll = scrollInfo.height - scrollInfo.clientHeight;
            editor.scrollTo(null, Math.min(maxScroll, scrollInfo.top + lineHeight));
        }

        // Extend selection to follow auto-scroll
        if (lastMousePos && isSelecting) {
            const pos = editor.coordsChar(lastMousePos);
            if (pos) {
                // const currentSelection = currentEditor.getSelection();
                const anchor = editor.getCursor('anchor');
                editor.setSelection(anchor, pos);
            }
        }
    }, resistance);
}

function stopAutoScroll() {
    if (scrollInterval) {
        clearInterval(scrollInterval);
        scrollInterval = null;
    }
}

function checkAutoScroll(event, editor) {
    if (!isSelecting) return;

    // Store the mouse position for selection extension during auto-scroll
    lastMousePos = {left: event.clientX, top: event.clientY};

    const editorRect = editor.getWrapperElement().getBoundingClientRect();

    const mouseY = event.clientY;

    // Check if mouse is near top or bottom of editor
    if (mouseY < editorRect.top + scrollMargin) {
        startAutoScroll('up', editor);
    } else if (mouseY > editorRect.bottom - scrollMargin) {
        startAutoScroll('down', editor);
    } else {
        stopAutoScroll();
    }
}

function initAutoscroll(editor) {
    editor.getWrapperElement().addEventListener("mousedown", function (e) {
        if (e.target.closest('.CodeMirror')) {
            isSelecting = true;
            // Check immediately on mousedown in case we start at the edge
            setTimeout(() => checkAutoScroll(e, editor), 0);
        }
    }, true);
    editor.getWrapperElement().addEventListener("mouseup", function () {
        isSelecting = false;
        lastMousePos = null;
        stopAutoScroll();
    });
    editor.getWrapperElement().addEventListener("mousemove", (event) => {checkAutoScroll(event, editor)});
    // Stop scrolling when mouse leaves editor
    editor.getWrapperElement().addEventListener("mouseleave", function () {
        lastMousePos = null;
        stopAutoScroll();
    });

    // Additional: Check for auto-scroll during selection changes
    // This catches cases where the selection extends to edges programmatically
    editor.on('beforeSelectionChange', function (cm, obj) {
        if (isSelecting) {
            // Small delay to let the selection update, then check mouse position
            setTimeout(() => {
                const mouseEvent = window.lastMouseEvent;
                if (mouseEvent) {
                    checkAutoScroll(mouseEvent, editor);
                }
            }, 0);
        }
    });
}

// Track the last mouse event for reference
document.addEventListener('mousemove', function (e) {
    window.lastMouseEvent = e;
});
document.addEventListener('mousemove', function (e) {
    window.lastMouseEvent = e;
});

/// NOGLOBAL
// function initAutoscroll(editor) {
//     console.log('initing autoscroll for', editor);
//
//     // Create separate state for each editor instance
//     const autoscrollState = {
//         scrollInterval: null,
//         isSelecting: false,
//         lastMousePos: null,
//         resistance: 120, // the more it is, the slower scrolling is
//         scrollMargin: 30
//     };
//
//     function startAutoScroll(direction) {
//         if (autoscrollState.scrollInterval) return; // Already scrolling
//
//         autoscrollState.scrollInterval = setInterval(() => {
//             const scrollInfo = editor.getScrollInfo();
//             const lineHeight = editor.defaultTextHeight();
//
//             if (direction === 'up') {
//                 editor.scrollTo(null, Math.max(0, scrollInfo.top - lineHeight));
//             } else if (direction === 'down') {
//                 const maxScroll = scrollInfo.height - scrollInfo.clientHeight;
//                 editor.scrollTo(null, Math.min(maxScroll, scrollInfo.top + lineHeight));
//             }
//
//             // Extend selection to follow auto-scroll
//             if (autoscrollState.lastMousePos && autoscrollState.isSelecting) {
//                 const pos = editor.coordsChar(autoscrollState.lastMousePos);
//                 if (pos) {
//                     const anchor = editor.getCursor('anchor');
//                     editor.setSelection(anchor, pos);
//                 }
//             }
//         }, autoscrollState.resistance);
//     }
//
//     function stopAutoScroll() {
//         if (autoscrollState.scrollInterval) {
//             clearInterval(autoscrollState.scrollInterval);
//             autoscrollState.scrollInterval = null;
//         }
//     }
//
//     function checkAutoScroll(event) {
//         if (!autoscrollState.isSelecting) return;
//
//         // Store the mouse position for selection extension during auto-scroll
//         autoscrollState.lastMousePos = {left: event.clientX, top: event.clientY};
//
//         const editorRect = editor.getWrapperElement().getBoundingClientRect();
//         const mouseY = event.clientY;
//
//         // Check if mouse is near top or bottom of editor
//         if (mouseY < editorRect.top + autoscrollState.scrollMargin) {
//             startAutoScroll('up');
//         } else if (mouseY > editorRect.bottom - autoscrollState.scrollMargin) {
//             startAutoScroll('down');
//         } else {
//             stopAutoScroll();
//         }
//     }
//
//     // Event listeners with proper scope
//     const wrapperElement = editor.getWrapperElement();
//
//     const mousedownHandler = function (e) {
//         if (e.target.closest('.CodeMirror')) {
//             autoscrollState.isSelecting = true;
//             // Check immediately on mousedown in case we start at the edge
//             setTimeout(() => checkAutoScroll(e), 0);
//         }
//     };
//
//     const mouseupHandler = function () {
//         autoscrollState.isSelecting = false;
//         autoscrollState.lastMousePos = null;
//         stopAutoScroll();
//     };
//
//     const mousemoveHandler = function (event) {
//         checkAutoScroll(event);
//     };
//
//     const mouseleaveHandler = function () {
//         autoscrollState.lastMousePos = null;
//         stopAutoScroll();
//     };
//
//     // Add event listeners
//     wrapperElement.addEventListener("mousedown", mousedownHandler, true);
//     wrapperElement.addEventListener("mouseup", mouseupHandler);
//     wrapperElement.addEventListener("mousemove", mousemoveHandler);
//     wrapperElement.addEventListener("mouseleave", mouseleaveHandler);
//
//     // Additional: Check for auto-scroll during selection changes
//     const selectionChangeHandler = function (cm, obj) {
//         if (autoscrollState.isSelecting) {
//             // Small delay to let the selection update, then check mouse position
//             setTimeout(() => {
//                 const mouseEvent = window.lastMouseEvent;
//                 if (mouseEvent) {
//                     checkAutoScroll(mouseEvent);
//                 }
//             }, 0);
//         }
//     };
//
//     editor.on('beforeSelectionChange', selectionChangeHandler);
//
//     // Cleanup function (call this when destroying the editor)
//     editor.autoscrollCleanup = function() {
//         stopAutoScroll();
//         wrapperElement.removeEventListener("mousedown", mousedownHandler, true);
//         wrapperElement.removeEventListener("mouseup", mouseupHandler);
//         wrapperElement.removeEventListener("mousemove", mousemoveHandler);
//         wrapperElement.removeEventListener("mouseleave", mouseleaveHandler);
//         editor.off('beforeSelectionChange', selectionChangeHandler);
//     };
// }
//
// // Track the last mouse event for reference (this can stay global)
// if (!window.lastMouseEventTracked) {
//     document.addEventListener('mousemove', function (e) {
//         window.lastMouseEvent = e;
//     });
//     window.lastMouseEventTracked = true; // Prevent duplicate listeners
// }