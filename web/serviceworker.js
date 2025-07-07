const urlsToCache = [
    '/',
    '/favicon.ico',
    '/manifest.json',
    '/app.css',
    '/lib/normalize.css',
    '/lib/sidebar.css',
    '/lib/codemirror.css',
    '/lib/hypermd.css',
    '/lib/theme-light.css',
    '/lib/theme-dark.css',
    '/chat.css',
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
    '/welcome.js',
    '/files.js',
    '/wasm_exec.js',
    '/app.js',
    '/wasm.js',
    '/chat.js',
    '/modals.js',
];

const urlParams = new URLSearchParams(self.location.search);
const COMMIT_HASH = urlParams.get('v') ? `?v=${urlParams.get('v')}` : '';
console.log('SW commit hash:', COMMIT_HASH);

const cacheName = `files-md-v${COMMIT_HASH}`;


self.addEventListener('install', event => {
    event.waitUntil(
        caches.open(cacheName)
            .then(cache => {
                const cachePromises = urlsToCache.map(url => {
                    if (url !== "/") {
                        url += COMMIT_HASH;
                    }
                    return cache.add(url)
                        .catch(err => console.error('✗ Failed to cache:', url, err));
                });
                return Promise.allSettled(cachePromises); // Won't fail if one fails
            })
            .then(() => {
                console.log('Caching complete, skipping waiting');
                return self.skipWaiting();
            })
    );
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
    console.log('intercepting fetch:', event.request.url);

    event.respondWith(
        caches.match(event.request)
            .then(response => {
                if (response) {
                    console.log('✓ Serving from cache:', event.request.url);
                    return response;
                }

                // Not in cache, try network
                console.log('Not in cache, trying network:', event.request.url);
                return fetch(event.request);
            })
            .catch(err => {
                console.error('Both cache and network failed:', event.request.url, err);
                return new Response('Offline and not cached', {
                    status: 503,
                    statusText: 'Service Unavailable'
                });
            })
    );
});