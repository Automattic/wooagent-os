import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

// The dev proxy lets the browser hit `/api/woo/*` without CORS — Vite forwards
// to the WooCommerce REST API on the staging store. Override the target in a
// local `.env.local` (`WOO_BASE=...`) when pointing at a different site.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const wooBase =
    env.WOO_BASE ?? 'https://store.example.com';
  return {
    plugins: [react()],
    server: {
      port: 5174,
      proxy: {
        '/api/woo': {
          target: wooBase,
          changeOrigin: true,
          secure: true,
          rewrite: (path) => path.replace(/^\/api\/woo/, '/wp-json/wc/v3'),
        },
      },
    },
  };
});
