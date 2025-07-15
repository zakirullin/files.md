/**
 * TreeJS is a JavaScript librarie for displaying TreeViews
 * on the web.
 *
 * @author Matthias Thalmann
 */

let tree;

function renderSidebar(focusDir = '', modifiedPaths) {
    let expandedDirs = new Set();
    let selectedNodes = new Set();

    // TODO save state
    if (tree) {
        // Save state for all nodes (both directories and files)
        function saveNodeState(node) {
            if (node.isExpanded()) {
                expandedDirs.add(node.toString());
            }
            if (node.isSelected()) {
                selectedNodes.add(node.toString());
            }

            // Recursively save state for child nodes
            if (node.getChildren) {
                node.getChildren().forEach(child => {
                    saveNodeState(child);
                });
            }
        }

        tree.getRoot().getChildren().forEach(child => {
            saveNodeState(child);
        });
    }

    root = new TreeNode('');
    root.path = '/';

    let inbox = new TreeNode("inbox");
    inbox.path = CHAT_PATH;
    inbox.setSelected(currentEditor.path === undefined);
    inbox.on('click', async function (n, node) {
        await openChat();
    });
    root.addChild(inbox)

    // Process directories
    // for (const dir in files) {
    //     if (dir === 'media') {
    //         continue;
    //     }
    //
    //     let dirNode = new TreeNode(dir, {expanded: false, dir: true});
    //
    //     // Process files in directory
    //     for (let file in files[dir]) {
    //         let fileNode = new TreeNode(file.replace(/\.md$/, ''), {expanded: false});
    //         fileNode.on('click', async function (n, node) {
    //             await openFile(node.parent.toString(), node.toString() + '.md');
    //         });
    //         dirNode.addChild(fileNode);
    //
    //         // Restore selected state for file nodes
    //         if (selectedNodes.has(file.replace(/\.md$/, ''))) {
    //             fileNode.setSelected(true);
    //         }
    //     }
    //
    //     root.addChild(dirNode);
    //
    //     // Handle focus directory or restore previous state
    //     if (dir === focusDir) {
    //         dirNode.setExpanded(true);
    //         dirNode.setSelected(true);
    //     } else {
    //         if (expandedDirs.has(dir)) dirNode.setExpanded(true);
    //         if (selectedNodes.has(dir)) dirNode.setSelected(true);
    //     }
    // }
    let dirNodes = {'/': root};

    // First pass: create all directories
    // Once got maximum call exceeded here
    walk(files, (path, isFile) => {
        if (path === '/media' || path.startsWith('/media/')) {
            return;
        }

        if (isFile) {
            return;
        }

        let dirNode = new TreeNode(toFilename(path), {expanded: false, dir: true});
        dirNode.path = removeTrailingSlash(path);
        if (path === '/archive/') {
            dirNode.isGroupEnd = true;
        }
        dirNodes[removeTrailingSlash(path)] = dirNode;

        // Add to parent
        const parentDirPath = toDirPath(path);
        const parentNode = dirNodes[parentDirPath] || root;
        parentNode.addChild(dirNode);

        // Handle focus directory or restore previous state
        let dir = toFilename(path);
        if (dir === focusDir) {
            dirNode.setExpanded(true);
            dirNode.setSelected(true);
        } else {
            if (expandedDirs.has(dir)) dirNode.setExpanded(true);
            if (selectedNodes.has(dir)) dirNode.setSelected(true);
        }

        if (modifiedPaths !== undefined) {
            console.log('PATHS', toRootDirName(modifiedPaths[0]), dir);
        }
        if (modifiedPaths !== undefined && modifiedPaths.some(modPath => toRootDirName(modPath) === dir)) {
            dirNode.shouldBlink = true;
        }
    });

    // Second pass: add all files
    walk(files, (path, isFile) => {
        if (path === '/media' || path.startsWith('/media/')) {
            return;
        }

        if (path === CONFIG_PATH || path === CHAT_PATH) {
            return;
        }

        if (!isFile) {
            return;
        }

        const {dirPath, filename} = toDirPathAndFilename(path);

        let fileNode = new TreeNode(filename.replace(/\.md$/, '').replace(/\.txt$/, ''), {expanded: false});
        fileNode.path = path;
        fileNode.on('click', async function (n, node) {
            await openFile(path);
        });

        const parentNode = dirNodes[dirPath] || root;
        parentNode.addChild(fileNode);

        if (modifiedPaths !== undefined && modifiedPaths.includes(path)) {
            fileNode.shouldBlink = true;
        }
    });

    const groupedDirs = new Set(['_read_', '_watch_', '_shop_', 'journal', 'habits', 'insights', 'archive', 'today', 'later']);
    const underscoreDirs = [];
    // Group all checklists
    for (const dir in dirNodes) {
        const filename = toFilename(dir);
        if (filename.startsWith('_') && filename.endsWith('_')) {
            underscoreDirs.push(dir);
            groupedDirs.add(filename);
        }
    }
    underscoreDirs.forEach((dir, index) => {
        const dirNode = dirNodes[dir];
        if (dirNode && dirNode.parent === root) {
            root.removeChild(dirNode);
            if (index === underscoreDirs.length - 1) {
                dirNode.isGroupEnd = true;
            }
            root.addChild(dirNode);
        }
    });

    const groups = [
        ['today', 'later'],
        ['journal', 'habits', 'insights'],
    ];
    for (let i = 0; i < groups.length; i++) {
        const dirList = groups[i];
        const existingDirs = dirList.filter(dir => dirNodes['/' + dir]);
        if (existingDirs.length === 0) continue;

        existingDirs.forEach((dir, index) => {
            const dirNode = dirNodes['/' + dir];
            if (dirNode && dirNode.parent === root) {
                // Add in the end
                root.removeChild(dirNode);
                if (index === existingDirs.length - 1) {
                    dirNode.isGroupEnd = true;
                }
                root.addChild(dirNode);
            }
        });
    }

    // Hide if only 2 groups
    let groupEndCount = 0;
    const rootChildren = root.getChildren();
    for (const child of rootChildren) {
        if (child.isGroupEnd) {
            groupEndCount++;
        }
    }
    if (groupEndCount < 2) {
        for (const child of rootChildren) {
            child.isGroupEnd = false;
        }
    }

    // Move all other nodes down
    for (const dir in dirNodes) {
        if (dir === '/' ||  groupedDirs.has(toFilename(dir))) continue;

        const dirNode = dirNodes[dir];
        if (dirNode && dirNode.parent === root) {
            root.removeChild(dirNode);
            root.addChild(dirNode);
        }
    }

    // Process root-level files
    // if (files['']) {
    //     for (let file in files['']) {
    //         if (file === CONFIG_FILENAME || file === CHAT_FILENAME) {
    //             continue;
    //         }
    //
    //         let fileNode = new TreeNode(file.replace(/\.md$/, '').replace(/\.txt$/, ''), {expanded: false});
    //         fileNode.on('click', async function (n, node) {
    //             await openFile('', file);
    //         });
    //         root.addChild(fileNode);
    //
    //         // Restore selected state for root-level file nodes
    //         if (selectedNodes.has(file.replace(/\.md$/, ''))) {
    //             fileNode.setSelected(true);
    //         }
    //     }
    // }
    //

    // Dirs first
    function sortTreeNode(node) {
        const children = node.getChildren();
        if (children && children.length > 0) {
            children.sort((a, b) => {
                const aName = a.toString();
                const bName = b.toString();
                const aIsDir = a.getOptions()?.dir === true;
                const bIsDir = b.getOptions()?.dir === true;

                // Inbox always comes first
                if (aName === 'inbox') return 1;
                if (bName === 'inbox') return 1;

                // Then sort by directory vs file
                if (aIsDir && !bIsDir) return -1;
                if (!aIsDir && bIsDir) return 1;

                return 0;
            });

            // Recursively sort children
            children.forEach(child => sortTreeNode(child));
        }
    }
    sortTreeNode(root);

    tree = new TreeView(root, '#tree', {
        show_root: false,
    });
}

function TreeNode(userObject, options) {
    var children = new Array();
    var self = this;
    var events = new Array();

    var expanded = true;
    var enabled = true;
    var selected = false;
    var isDir = false;

    if (userObject) {
        if (typeof userObject !== "string" && typeof userObject.toString !== "function") {
            throw new Error("Parameter 1 must be of type String or Object, where it must have the function toString()");
        }
    } else {
        userObject = "";
    }

    if (!options || typeof options !== "object") {
        options = {};
    } else {
        expanded = TreeUtil.getProperty(options, "expanded", true);
        enabled = TreeUtil.getProperty(options, "enabled", true);
        selected = TreeUtil.getProperty(options, "selected", false);
        isDir = TreeUtil.getProperty(options, "dir", false);
    }

    /*
    * Methods
    */
    this.addChild = function (node) {
        if (!TreeUtil.getProperty(options, "allowsChildren", true)) {
            console.warn("Option allowsChildren is set to false, no child added");
            return;
        }

        if (node instanceof TreeNode) {
            children.push(node);

            //Konstante hinzufügen (workaround)
            Object.defineProperty(node, "parent", {
                value: this,
                writable: false,
                enumerable: true,
                configurable: true
            });
        } else {
            throw new Error("Parameter 1 must be of type TreeNode");
        }
    }

    this.prependChild = function (node) {
        if (!TreeUtil.getProperty(options, "allowsChildren", true)) {
            console.warn("Option allowsChildren is set to false, no child added");
            return;
        }
        if (node instanceof TreeNode) {
            children.unshift(node); // Add to beginning instead of end
            // Set parent property (same as addChild)
            Object.defineProperty(node, "parent", {
                value: this,
                writable: false,
                enumerable: true,
                configurable: true
            });
        } else {
            throw new Error("Parameter 1 must be of type TreeNode");
        }
    }

    this.removeChildPos = function (pos) {
        if (typeof children[pos] !== "undefined") {
            if (typeof children[pos] !== "undefined") {
                children.splice(pos, 1);
            }
        }
    }

    this.removeChild = function (node) {
        if (!(node instanceof TreeNode)) {
            throw new Error("Parameter 1 must be of type TreeNode");
        }

        this.removeChildPos(this.getIndexOfChild(node));
    }

    this.getChildren = function () {
        return children;
    }

    this.getChildCount = function () {
        return children.length;
    }

    this.getIndexOfChild = function (node) {
        for (var i = 0; i < children.length; i++) {
            if (children[i].equals(node)) {
                return i;
            }
        }

        return -1;
    }

    this.getRoot = function () {
        var node = this;

        while (typeof node.parent !== "undefined") {
            node = node.parent;
        }

        return node;
    }

    this.setUserObject = function (_userObject) {
        if (!(typeof _userObject === "string") || typeof _userObject.toString !== "function") {
            throw new Error("Parameter 1 must be of type String or Object, where it must have the function toString()");
        } else {
            userObject = _userObject;
        }
    }

    this.getUserObject = function () {
        return userObject;
    }

    this.setOptions = function (_options) {
        if (typeof _options === "object") {
            options = _options;
        }
    }

    this.changeOption = function (option, value) {
        options[option] = value;
    }

    this.getOptions = function () {
        return options;
    }

    this.isLeaf = function () {
        // PATCHED
        return !isDir;
        // return (children.length == 0);
    }

    this.setExpanded = function (_expanded) {
        if (this.isLeaf()) {
            return;
        }

        if (typeof _expanded === "boolean") {
            if (expanded == _expanded) {
                return;
            }

            expanded = _expanded;

            if (_expanded) {
                this.on("expand")(this);
            } else {
                this.on("collapse")(this);
            }

            this.on("toggle_expanded")(this);
        }
    }

    this.toggleExpanded = function () {
        if (expanded) {
            this.setExpanded(false);
        } else {
            this.setExpanded(true);
        }
    };

    this.isExpanded = function () {
        if (this.isLeaf()) {
            return true;
        } else {
            return expanded;
        }
    }

    this.setEnabled = function (_enabled) {
        if (typeof _enabled === "boolean") {
            if (enabled == _enabled) {
                return;
            }

            enabled = _enabled;

            if (_enabled) {
                this.on("enable")(this);
            } else {
                this.on("disable")(this);
            }

            this.on("toggle_enabled")(this);
        }
    }

    this.toggleEnabled = function () {
        if (enabled) {
            this.setEnabled(false);
        } else {
            this.setEnabled(true);
        }
    }

    this.isEnabled = function () {
        return enabled;
    }

    this.setSelected = function (_selected) {
        if (typeof _selected !== "boolean") {
            return;
        }

        if (selected == _selected) {
            return;
        }

        selected = _selected;

        if (_selected) {
            this.on("select")(this);
        } else {
            this.on("deselect")(this);
        }

        this.on("toggle_selected")(this);
    }

    this.toggleSelected = function () {
        if (selected) {
            this.setSelected(false);
        } else {
            this.setSelected(true);
        }
    }

    this.isSelected = function () {
        return selected;
    }

    this.open = function () {
        if (!this.isLeaf()) {
            this.on("open")(this);
        }
    }

    this.on = function (ev, callback) {
        if (typeof callback === "undefined") {
            if (typeof events[ev] !== "function") {
                return function () {
                };
            } else {
                return events[ev];
            }
        }

        if (typeof callback !== 'function') {
            throw new Error("Argument 2 must be of type function");
        }

        events[ev] = callback;
    }

    this.getListener = function (ev) {
        return events[ev];
    }

    this.equals = function (node) {
        if (node instanceof TreeNode) {
            if (node.getUserObject() == userObject) {
                return true;
            }
        }

        return false;
    }

    this.toString = function () {
        if (typeof userObject === "string") {
            return userObject;
        } else {
            return userObject.toString();
        }
    }
}

function TreePath(root, node) {
    var nodes = new Array();

    this.setPath = function (root, node) {
        nodes = new Array();

        while (typeof node !== "undefined" && !node.equals(root)) {
            nodes.push(node);
            node = node.parent;
        }

        if (node.equals(root)) {
            nodes.push(root);
        } else {
            nodes = new Array();
            throw new Error("Node is not contained in the tree of root");
        }

        nodes = nodes.reverse();

        return nodes;
    }

    this.getPath = function () {
        return nodes;
    }

    this.toString = function () {
        return nodes.join(" - ");
    }

    if (root instanceof TreeNode && node instanceof TreeNode) {
        this.setPath(root, node);
    }
}

function TreeView(root, container, options) {
    var self = this;
    var draggedNode = null;
    var draggedElement = null;
    var dropIndicator = null;

    if (typeof root === "undefined") {
        throw new Error("Parameter 1 must be set (root)");
    }

    if (!(root instanceof TreeNode)) {
        throw new Error("Parameter 1 must be of type TreeNode");
    }

    if (container) {
        if (!TreeUtil.isDOM(container)) {
            container = document.querySelector(container);

            if (container instanceof Array) {
                container = container[0];
            }

            if (!TreeUtil.isDOM(container)) {
                throw new Error("Parameter 2 must be either DOM-Object or CSS-QuerySelector (#, .)");
            }
        }
    } else {
        container = null;
    }

    if (!options || typeof options !== "object") {
        options = {};
    }



    function createDropIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'tj_drop_indicator';
        return indicator;
    }

    function findNodeElement(element) {
        while (element && !element.tj_node) {
            element = element.parentElement;
        }
        return element;
    }

    function getDropPosition(e, element) {
        const rect = element.getBoundingClientRect();
        const y = e.clientY - rect.top;
        const height = rect.height;

        if (y < height * 0.25) return 'before';
        if (y > height * 0.75) return 'after';
        return 'inside';
    }

    this.setRoot = function (_root) {
        if (root instanceof TreeNode) {
            root = _root;
        }
    }

    this.getRoot = function () {
        return root;
    }

    this.expandAllNodes = function () {
        root.setExpanded(true);
        root.getChildren().forEach(function (child) {
            TreeUtil.expandNode(child);
        });
    }

    this.expandPath = function (path) {
        if (!(path instanceof TreePath)) {
            throw new Error("Parameter 1 must be of type TreePath");
        }
        path.getPath().forEach(function (node) {
            node.setExpanded(true);
        });
    }

    this.collapseAllNodes = function () {
        root.setExpanded(false);
        root.getChildren().forEach(function (child) {
            TreeUtil.collapseNode(child);
        });
    }

    this.setContainer = function (_container) {
        if (TreeUtil.isDOM(_container)) {
            container = _container;
        } else {
            _container = document.querySelector(_container);
            if (_container instanceof Array) {
                _container = _container[0];
            }
            if (!TreeUtil.isDOM(_container)) {
                throw new Error("Parameter 1 must be either DOM-Object or CSS-QuerySelector (#, .)");
            }
        }
    }

    this.getContainer = function () {
        return container;
    }

    this.setOptions = function (_options) {
        if (typeof _options === "object") {
            options = _options;
        }
    }

    this.changeOption = function (option, value) {
        options[option] = value;
    }

    this.getOptions = function () {
        return options;
    }

    this.getSelectedNodes = function () {
        return TreeUtil.getSelectedNodesForNode(root);
    }

    this.reload = function () {
        if (container == null) {
            console.warn("No container specified");
            return;
        }

        container.classList.add("tj_container");
        var cnt = document.createElement("ul");

        if (TreeUtil.getProperty(options, "show_root", true)) {
            cnt.appendChild(renderNode(root));
        } else {
            root.getChildren().forEach(function (child) {
                cnt.appendChild(renderNode(child));
            });
        }

        container.innerHTML = "";
        container.appendChild(cnt);
        setupContainerDropZone();
    }

    function setupContainerDropZone() {
        container.addEventListener('dragover', function(e) {
            e.preventDefault();
            if (dropIndicator && !e.target.closest('.tj_description')) {
                dropIndicator.remove();
                dropIndicator = null;
            }
            e.dataTransfer.dropEffect = 'move';
        });

        container.addEventListener('drop', function(e) {
            e.preventDefault();
            if (dropIndicator) {
                dropIndicator.remove();
                dropIndicator = null;
            }

            if (e.dataTransfer.files.length > 0) {
                handleExternalFileDrop(e);
            }
        });
    }

    function handleExternalFileDrop(e) {
        console.log(e);
        const files = Array.from(e.dataTransfer.files);
        files.forEach(file => {
            if (file.type === 'text/plain' || file.name.endsWith('.md')) {
                const reader = new FileReader();
                reader.onload = function(event) {
                    const content = event.target.result;
                    const fileName = file.name.replace(/\.[^/.]+$/, "");

                    if (typeof window.handleDroppedFile === 'function') {
                        window.handleDroppedFile(fileName, content);
                    } else {
                        console.log('Dropped file:', fileName, content);
                    }
                };
                reader.readAsText(file);
            }
        });
    }

    function createGroupHeader(headerText, headerClass) {
        var li_header = document.createElement("li");
        li_header.className = "tj_group_header " + headerClass;
        li_header.innerHTML = '<span class="tj_group_title">' + headerText + '</span>';
        return li_header;
    }

    function shouldShowGroupHeaders() {
        var groupCount = 0;
        var children = root.getChildren();

        if (children.length > 0) {
            groupCount = 1; // First group
            for (var i = 0; i < children.length; i++) {
                if (children[i].isGroupEnd) {
                    groupCount++;
                }
            }
        }

        return groupCount > 1;
    }

    function renderNode(node) {
        var li_outer = document.createElement("li");
        var span_desc = document.createElement("span");
        span_desc.className = "tj_description";
        span_desc.tj_node = node;

        var needsGroupHeader = false;
        var groupHeaderText = "";
        var groupHeaderClass = "";

        if (node.parent === root) {
            let siblings = root.getChildren();
            let myIndex = siblings.indexOf(node);

            // ?? =)
            if (myIndex === 0) {
                needsGroupHeader = true;
            } else if (myIndex > 0 && siblings[myIndex - 1].isGroupEnd) {
                needsGroupHeader = true;
            }

            if (needsGroupHeader) {
                let nodeStr = node.toString();
                if (nodeStr.startsWith('_') && nodeStr.endsWith('_')) {
                    groupHeaderText = "Lists";
                    groupHeaderClass = "lists";
                } else if (['today', 'later'].includes(nodeStr)) {
                    groupHeaderText = "Tasks";
                    groupHeaderClass = "tasks";
                } else if (['journal', 'habits', 'insights'].includes(nodeStr)) {
                    groupHeaderText = "Personal";
                    groupHeaderClass = "personal";
                } else if (nodeStr === 'inbox') {
                    groupHeaderText = "Flow";
                    groupHeaderClass = "flow";
                } else {
                    groupHeaderText = "Files";
                    groupHeaderClass = "user-dirs";
                }
            }
        }

        // If we need a group header, we'll return a document fragment with both header and node
        if (needsGroupHeader && shouldShowGroupHeaders()) {
            var fragment = document.createDocumentFragment();
            fragment.appendChild(createGroupHeader(groupHeaderText, groupHeaderClass));
        }

        if (node.shouldBlink) {
            span_desc.classList.add('sidebar-blink');
            setTimeout(() => span_desc.classList.remove('sidebar-blink'), 1500);
            node.shouldBlink = false;
        }

        if (node.isGroupEnd) {
            span_desc.classList.add("group-end");
        }

        if (node.isLeaf()) {
            span_desc.draggable = true;
        }

        if (!node.isEnabled()) {
            li_outer.setAttribute("disabled", "");
            node.setExpanded(false);
            node.setSelected(false);
        }

        if (node.isSelected()) {
            span_desc.classList.add("selected");
        }

        if (node.isExpanded()) {
            span_desc.classList.add("expanded");
        }

        span_desc.addEventListener("dragstart", function(e) {
            if (!node.isLeaf()) return;

            draggedNode = node;
            draggedElement = span_desc;
            span_desc.classList.add("tj_dragging");

            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', node.toString());
            e.dataTransfer.setDragImage(span_desc, -10, span_desc.offsetHeight / 2);
        });

        span_desc.addEventListener("dragend", function(e) {
            span_desc.classList.remove("tj_dragging");
            if (dropIndicator) {
                dropIndicator.remove();
                dropIndicator = null;
            }
            draggedNode = null;
            draggedElement = null;
        });

        span_desc.addEventListener("dragover", function(e) {
            e.preventDefault();
            if (!draggedNode || draggedNode === node) return;

            const position = getDropPosition(e, span_desc);

            if (dropIndicator) dropIndicator.remove();
            dropIndicator = createDropIndicator();

            if (position === 'before') {
                const rect = li_outer.getBoundingClientRect();
                dropIndicator.style.top = rect.top + 'px';
                dropIndicator.style.left = rect.left + 'px';
                dropIndicator.style.width = rect.width + 'px';
                document.body.appendChild(dropIndicator);
            } else if (position === 'after') {
                const rect = li_outer.getBoundingClientRect();
                dropIndicator.style.top = rect.bottom + 'px';
                dropIndicator.style.left = rect.left + 'px';
                dropIndicator.style.width = rect.width + 'px';
                document.body.appendChild(dropIndicator);
            } else if (position === 'inside' && !node.isLeaf()) {
                span_desc.classList.add("tj_drop_target");
                return;
            }

            dropIndicator.classList.add("active");
        });

        span_desc.addEventListener("dragleave", function(e) {
            span_desc.classList.remove("tj_drop_target");
        });

        span_desc.addEventListener("drop", function(e) {
            e.preventDefault();
            e.stopPropagation();

            span_desc.classList.remove("tj_drop_target");

            if (!draggedNode || draggedNode === node) return;

            const position = getDropPosition(e, span_desc);

            if (typeof window.handleNodeMove === 'function') {
                const sourceDir = draggedNode.parent ? draggedNode.parent.toString() : '';
                console.log(draggedNode.parent);
                console.log(sourceDir);

                const sourceFile = draggedNode.toString() + '.md';
                let targetDir;
                if (position === 'inside' && !node.isLeaf()) {
                    // TODO handle multiple subdirs?
                    targetDir = node.toString();
                } else {
                    targetDir = node.parent ? node.parent.toString() : '';
                }

                window.handleNodeMove(sourceDir, sourceFile, targetDir);
            }

            if (dropIndicator) {
                dropIndicator.remove();
                dropIndicator = null;
            }
        });

        span_desc.addEventListener("click", function (e) {
            var cur_el = e.target;

            while (typeof cur_el.tj_node === "undefined" || cur_el.classList.contains("tj_container")) {
                cur_el = cur_el.parentElement;
            }

            var node_cur = cur_el.tj_node;

            if (typeof node_cur === "undefined") {
                return;
            }

            if (node_cur.isEnabled()) {
                if (e.ctrlKey == false) {
                    if (!node_cur.isLeaf()) {
                        node_cur.toggleExpanded();
                        self.reload();
                    } else {
                        node_cur.open();
                    }

                    node_cur.on("click")(e, node_cur);
                }

                if (e.ctrlKey == true) {
                    node_cur.toggleSelected();
                    self.reload();
                } else {
                    var rt = node_cur.getRoot();

                    if (rt instanceof TreeNode) {
                        TreeUtil.getSelectedNodesForNode(rt).forEach(function (_nd) {
                            _nd.setSelected(false);
                        });
                    }
                    node_cur.setSelected(true);

                    self.reload();
                }
            }
        });

        span_desc.addEventListener("contextmenu", function (e) {
            var cur_el = e.target;

            while (typeof cur_el.tj_node === "undefined" || cur_el.classList.contains("tj_container")) {
                cur_el = cur_el.parentElement;
            }

            var node_cur = cur_el.tj_node;

            if (typeof node_cur === "undefined") {
                return;
            }

            if (typeof node_cur.getListener("contextmenu") !== "undefined") {
                node_cur.on("contextmenu")(e, node_cur);
                e.preventDefault();
            } else if (typeof TreeConfig.context_menu === "function") {
                TreeConfig.context_menu(e, node_cur);
                e.preventDefault();
            }
        });

        if (node.isLeaf() && !TreeUtil.getProperty(node.getOptions(), "forceParent", false)) {
            var ret = '';
            var icon = TreeUtil.getProperty(node.getOptions(), "icon", "");

            let name = node.toString();
            if (startsWithEmoji(name)) {
                ret += '<span class="tj_mod_icon" ><div style="width: 22px; text-align: center; transform: translateY(-2px);">' + getFirstEmoji(node.toString()) + '</div></span>';
                name = trimFirstEmoji(name);
            } else if (node.toString() === 'inbox') {
                ret += '<span class="tj_mod_icon" style="padding-right: 2px">' + TreeConfig.chat_icon + '</span>';
            } else if (icon != "") {
                ret += '<span class="tj_mod_icon">' + icon + '</span>';
            } else if ((icon = TreeUtil.getProperty(options, "leaf_icon", "")) != "") {
                ret += '<span class="tj_icon">' + icon + '</span>';
            } else {
                ret += '<span class="tj_icon">' + TreeConfig.leaf_icon + '</span>';
            }

            span_desc.innerHTML = ret + name + "</span>";
            span_desc.classList.add("tj_leaf");
            if (node.toString() === 'inbox') {
                span_desc.classList.add("sidebar-inbox");
            }

            li_outer.appendChild(span_desc);
        } else {
            var ret = '';
            if (node.isExpanded()) {
                ret += '<span class="tj_mod_icon">' + TreeConfig.open_icon + '</span>';
            } else {
                if (node.toString().startsWith('_') && node.toString().endsWith('_')) {
                    ret += '<span class="tj_mod_icon">' + TreeConfig.checklists_icon + '</span>';
                } else if (node.toString() === 'today' || node.toString() === 'later') {
                    ret += '<span class="tj_mod_icon">' + TreeConfig.tasks_icon + '</span>';
                } else {
                    ret += '<span class="tj_mod_icon">' + TreeConfig.close_icon + '</span>';
                }
            }

            var icon = TreeUtil.getProperty(node.getOptions(), "icon", "");
            icon = '';
            if (icon != "") {
                ret += '<span class="tj_icon">' + icon + '</span>';
            } else if ((icon = TreeUtil.getProperty(options, "parent_icon", "")) != "") {
                ret += '<span class="tj_icon">' + icon + '</span>';
            } else {
                ret += '<span class="tj_icon">' + TreeConfig.parent_icon + '</span>';
            }

            span_desc.innerHTML = ret + node.toString() + '</span>';

            li_outer.appendChild(span_desc);

            if (node.isExpanded()) {
                var ul_container = document.createElement("ul");

                node.getChildren().forEach(function (child) {
                    ul_container.appendChild(renderNode(child));
                });

                li_outer.appendChild(ul_container)
            }
        }


        if (needsGroupHeader && shouldShowGroupHeaders()) {
            fragment.appendChild(li_outer);
            return fragment;
        }

        return li_outer;
    }

    if (typeof container !== "undefined")
        this.reload();
}

/*
* Util-Methods
*/
const TreeUtil = {
    default_leaf_icon: "<span>&#128441;</span>",
    default_parent_icon: "<span>&#128449;</span>",
    default_open_icon: "<svg width=\"22px\" height=\"22px\" viewBox=\"0 0 32 32\" xmlns=\"http://www.w3.org/2000/svg\" fill=\"none\"> <path stroke-linecap=\"round\" stroke-width=\"2\" d=\"M4 26V8a2 2 0 012-2h6c3 0 3 3 5 3h7a2 2 0 012 2v2M4 26l3.783-12.294A1 1 0 018.739 13H26M4 26h19.523a2 2 0 001.911-1.412l3.168-10.294A1 1 0 0027.646 13H26\"/> </svg>",
    default_close_icon: "<svg width=\"22px\" height=\"22px\" viewBox=\"0 0 32 32\" xmlns=\"http://www.w3.org/2000/svg\" fill=\"none\"> <path stroke-linecap=\"round\" stroke-width=\"2\" d=\"M28 11v13a2 2 0 01-2 2H6a2 2 0 01-2-2V8a2 2 0 012-2h6c3 0 3 3 5 3h9.003C27.108 9 28 9.895 28 11z\"/> </svg>",
    checklists_icon: "<svg width=\"22px\" height=\"22px\" fill=\"none\" viewBox=\"0 0 32 32\"> <path  stroke-linecap=\"round\" stroke-width=\"2\" d=\"M28 11v13a2 2 0 01-2 2H6a2 2 0 01-2-2V8a2 2 0 012-2h6c3 0 3 3 5 3h9.003C27.108 9 28 9.895 28 11zM12 15h8M12 19h8\"/> </svg>",
    tasks_icon: "<svg width=\"22px\" height=\"22px\" fill=\"none\" viewBox=\"0 0 32 32\"> <path stroke-linecap=\"round\" stroke-width=\"2\" d=\"M28 11v13a2 2 0 01-2 2H6a2 2 0 01-2-2V8a2 2 0 012-2h6c3 0 3 3 5 3h9.003C27.108 9 28 9.895 28 11z\"/> <path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M12 17.5l3 3 6-6\"/> </svg>",
    chat_icon: "<svg xmlns=\"http://www.w3.org/2000/svg\" style=\"transform: translateX(-2px);\" width=\"25px\" height=\"25px\" fill=\"none\" viewBox=\"0 0 34 34\"> <path stroke-linecap=\"round\" stroke-linejoin=\"round\" stroke-width=\"2\" d=\"M 25 7 H 11 a 4 4 0 0 0 -4 4 v 10 a 4 4 0 0 0 4 4 h 7 l 6 4 v -4 h 1 a 4 4 0 0 0 4 -4 V 11 a 4 4 0 0 0 -4 -4 z\"/> </svg>",

    isDOM: function (obj) {
        try {
            return obj instanceof HTMLElement;
        } catch (e) {
            return (typeof obj === "object") &&
                (obj.nodeType === 1) && (typeof obj.style === "object") &&
                (typeof obj.ownerDocument === "object");
        }
    },

    getProperty: function (options, opt, def) {
        if (typeof options[opt] === "undefined") {
            return def;
        }

        return options[opt];
    },

    expandNode: function (node) {
        node.setExpanded(true);

        if (!node.isLeaf()) {
            node.getChildren().forEach(function (child) {
                TreeUtil.expandNode(child);
            });
        }
    },

    collapseNode: function (node) {
        node.setExpanded(false);

        if (!node.isLeaf()) {
            node.getChildren().forEach(function (child) {
                TreeUtil.collapseNode(child);
            });
        }
    },

    getSelectedNodesForNode: function (node) {
        if (!(node instanceof TreeNode)) {
            throw new Error("Parameter 1 must be of type TreeNode");
        }

        var ret = new Array();

        if (node.isSelected()) {
            ret.push(node);
        }

        node.getChildren().forEach(function (child) {
            if (child.isSelected()) {
                if (ret.indexOf(child) == -1) {
                    ret.push(child);
                }
            }

            if (!child.isLeaf()) {
                TreeUtil.getSelectedNodesForNode(child).forEach(function (_node) {
                    if (ret.indexOf(_node) == -1) {
                        ret.push(_node);
                    }
                });
            }
        });

        return ret;
    }
};

window.handleNodeMove = async function(sourceDir, sourceFile, targetDir) {
    console.log(`Moving ${sourceDir}/${sourceFile} to ${targetDir}/`);

    console.log(`${sourceDir}/${sourceFile}`);
    if (currentEditor.path === `${sourceDir}/${sourceFile}`) {
       await moveCurrentFile(targetDir);
    } else {
        await moveFile(`${sourceDir}/${sourceFile}`, `${targetDir}/${sourceFile}`);
    }
};

// WHEN?
window.handleDroppedFile = async function(fileName, content) {
    console.log(`Creating new file: ${fileName}`, content);

    if (typeof createFileFromContent === 'function') {
        await createFileFromContent(fileName + '.md', content);
    }

    if (typeof renderSidebar === 'function') {
        renderSidebar();
    }
};

var TreeConfig = {
    leaf_icon: TreeUtil.default_leaf_icon,
    parent_icon: TreeUtil.default_parent_icon,
    open_icon: TreeUtil.default_open_icon,
    close_icon: TreeUtil.default_close_icon,
    tasks_icon: TreeUtil.tasks_icon,
    chat_icon: TreeUtil.chat_icon,
    checklists_icon: TreeUtil.checklists_icon,
    context_menu: undefined
};


