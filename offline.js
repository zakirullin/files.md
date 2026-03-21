const urlsToCache = [
    '/',
    '/web/favicon.ico',
    '/web/icon.png',
    '/web/icon_small.png',
    '/web/manifest.json',
    '/web/app.css',
    '/lib/normalize.css',
    '/lib/sidebar.css',
    '/lib/codemirror.css',
    '/lib/hypermd.css',
    '/lib/theme-light.css',
    '/lib/theme-dark.css',
    '/web/chat.css',
    '/lib/sidebar.js',
    '/lib/codemirror.js',
    '/lib/core.js',
    '/lib/markdown.js',
    '/lib/hypermd.js',
    '/lib/keymap.js',
    '/lib/click.js',
    '/lib/hide-token.js',
    '/lib/fold.js',
    '/lib/fold-image.js',
    '/lib/fold-link.js',
    '/lib/table-align.js',
    '/lib/autocomplete-link.js',
    '/lib/show-hint.js',
    '/lib/autoscroll.js',
    '/lib/codemirror-go.js',
    '/lib/codemirror-php.js',
    '/lib/codemirror-shell.js',
    '/lib/similarity.js',
    '/lib/emoji.js',
    '/lib/fs.js',
    '/lib/md.js',
    '/welcome.js',
    '/files.js',
    '/editor.js',
    '/app.js',
    '/inbox.js',
    '/modals.js',
];

const urlParams = new URLSearchParams(self.location.search);
const COMMIT_HASH = urlParams.get('v') ? `?v=${urlParams.get('v')}` : '';

const cacheName = `files-md-v${COMMIT_HASH}`;


self.addEventListener('install', event => {
    event.waitUntil((async () => {
        let cache;
        try {
            cache = await caches.open(cacheName);
        } catch (err) {
            console.error('Failed to open cache:', err);
            return;
        }

        for (let url of urlsToCache) {
            if (url !== "/" && url !== 'favicon.ico' && url !== 'small_icon.png' && url !== 'icon.png') {
                url = url + COMMIT_HASH;
            }

            try {
                await cache.add(url);
            } catch (err) {
                console.error('✗ Failed to cache:', url, err);
            }
        }

        return await self.skipWaiting();
    })());
});

self.addEventListener("activate", (event) => {
    console.log("Service worker is activated");

    event.waitUntil(
        caches.keys().then((cacheNames) => {
            return cacheNames.map((cache) => {
                if (cache !== cacheName) {
                    caches.delete(cache);
                }
            });
        })
    );
});

self.addEventListener("fetch", (event) => {
    // Skip non-GET requests and extensions
    if (event.request.method !== 'GET' ||
        event.request.url.startsWith('chrome-extension:') ||
        event.request.url.startsWith('moz-extension:')) {
        return;
    }

    event.respondWith(handleRequest(event.request));
});

async function handleRequest(request) {
    for (let i = 0; i < 3; i++) {
        try {
            const response = await fetch(request);
            // In South America I had poor internet connection, and some js files
            // were partly loaded/cached :( It seems like Chromium fires
            // range requests for some files.
            if (response.status === 206) {
                console.warn('⚠️ Partial content (206), not caching:', event.request.url);
                return response;
            }

            if (response.ok) {
                const cache = await caches.open(cacheName);
                await cache.put(request, response.clone());
            }

            return response;

        } catch (error) {
            if (i === 2) {
                console.log(`Using cache`, error);
                return await caches.match(request);
            }

            console.warn(`Fetch failed (attempt ${i + 1}), retrying...`, error);
            await new Promise(resolve => setTimeout(resolve, 500));
        }
    }
}
