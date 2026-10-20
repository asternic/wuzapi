export class APIError extends Error {
  constructor(message, status) {
    super(message);
    this.status = status;
  }
}

export function createAPI(base, token) {
  const url = new URL(base || location.origin, location.origin);
  if (
    !['http:', 'https:'].includes(url.protocol) ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  ) {
    throw new Error(
      'Usá una URL HTTP(S) sin credenciales, parámetros ni fragmentos.',
    );
  }
  const root = url.href.replace(/\/$/, '');
  const lifetime = new AbortController();
  return {
    base: root,
    close() {
      lifetime.abort();
    },
    async request(path, body, method = body === undefined ? 'GET' : 'POST') {
      const form = body instanceof FormData;
      const response = await fetch(root + path, {
        method,
        redirect: 'error',
        cache: 'no-store',
        credentials: 'omit',
        signal: AbortSignal.any([
          lifetime.signal,
          AbortSignal.timeout(form ? 120000 : 30000),
        ]),
        headers: {
          token,
          ...(form || body === undefined
            ? {}
            : { 'Content-Type': 'application/json' }),
        },
        body:
          body === undefined ? undefined : form ? body : JSON.stringify(body),
      });
      const text = await response.text();
      let result;
      try {
        result = JSON.parse(text);
      } catch {
        throw new APIError(
          'El servidor no devolvió JSON. Revisá la URL y el proxy.',
          response.status,
        );
      }
      if (!response.ok || result.success === false)
        throw new APIError(
          result.error || `Error HTTP ${response.status}`,
          response.status,
        );
      let data = Object.hasOwn(result, 'data') ? result.data : result;
      if (typeof data === 'string') {
        try {
          data = JSON.parse(data);
        } catch {
          /* Legacy text responses remain text. */
        }
      }
      return data;
    },
  };
}

export function readFile(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = () => reject(new Error('No se pudo leer el archivo.'));
    reader.readAsDataURL(file);
  });
}
