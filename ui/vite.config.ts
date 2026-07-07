import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { createRequire } from 'node:module';
import { dirname } from 'node:path';

const require = createRequire(import.meta.url);
const wordpressUiPackage = require.resolve('@wordpress/ui/package.json');
const wordpressThemeTokens = require.resolve('@wordpress/theme/design-tokens.css', {
  paths: [dirname(wordpressUiPackage)],
});

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@wordpress/theme/design-tokens.css': wordpressThemeTokens,
    },
  },
  server: {
    port: 5173,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/test/setup.ts',
  },
});
