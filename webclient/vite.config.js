import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  base: '/client/',
  build: { outDir: '../static/client', emptyOutDir: true },
  server: {
    proxy: Object.fromEntries(
      ['chat', 'session', 'user', 'group'].map((path) => [
        `/${path}`,
        {
          target: process.env.WUZAPI_DEV_TARGET || 'http://127.0.0.1:8080',
        },
      ]),
    ),
  },
});
