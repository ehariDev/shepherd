import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { execSync } from 'child_process';
import { writeFileSync } from 'fs';
import path from 'path';
import { defineConfig } from 'vite';

const buildInfoPlugin = {
  name: 'build-info',
  closeBundle() {
    const sha = (() => {
      try {
        return execSync('git rev-parse --short HEAD').toString().trim();
      } catch {
        return 'dev';
      }
    })();
    const dirty = (() => {
      try {
        return execSync('git status --porcelain').toString().trim() !== '';
      } catch {
        return true;
      }
    })();
    writeFileSync(
      '../internal/spa/dist/BUILD_INFO.json',
      JSON.stringify({
        git_sha: sha,
        built_at: new Date().toISOString(),
        dirty,
      }),
    );
  },
};

export default defineConfig({
  plugins: [react(), tailwindcss(), buildInfoPlugin],
  resolve: {
    alias: { '@': path.resolve(__dirname, './src') },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
    },
  },
  build: {
    outDir: '../internal/spa/dist',
    emptyOutDir: true,
  },
  test: {
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
    exclude: ['tests/**', 'node_modules/**', 'dist/**'],
    // Environment stays 'node' by default (the 16 pure-TS tests keep their
    // speed); component tests opt into jsdom with a per-file
    // `// @vitest-environment jsdom` docblock instead.
    //
    // `globals: true` is needed for exactly one thing: @testing-library/react
    // registers its automatic cleanup() via `afterEach` only when `afterEach`
    // already exists on globalThis at import time (see its index.js). Without
    // this, unmounted components from one test's render() pile up in the
    // jsdom document and leak into the next test in the same file.
    globals: true,
  },
});
