self.addEventListener('fetch', function(event) {
  // Verificar se a URL é http ou https antes de tentar fazer cache
  const url = new URL(event.request.url);
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return; // Ignorar protocolos que não sejam http ou https
  }

  event.respondWith(
    caches.match(event.request)
      .then(function(response) {
        // Cache hit - return response
        if (response) {
          return response;
        }

        return fetch(event.request).then(
          function(response) {
            // Check if we received a valid response
            if(!response || response.status !== 200 || response.type !== 'basic') {
              return response;
            }

            // IMPORTANT: Clone the response. A response is a stream
            // and because we want the browser to consume the response
            // as well as the cache consuming the response, we need
            // to clone it so we have two streams.
            var responseToCache = response.clone();

            caches.open(CACHE_NAME)
              .then(function(cache) {
                try {
                  // Verifica se a URL é válida para armazenamento em cache
                  const reqUrl = new URL(event.request.url);
                  if (reqUrl.protocol === 'http:' || reqUrl.protocol === 'https:') {
                    cache.put(event.request, responseToCache);
                  }
                } catch (error) {
                  console.warn('Erro ao armazenar em cache:', error);
                }
              });

            return response;
          }
        );
      })
  );
}); 