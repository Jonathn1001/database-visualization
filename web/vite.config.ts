import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7777',
        changeOrigin: false,
      },
    },
  },
  build: {
    // Build straight into the Go server package so embed.FS can include it.
    // go:embed cannot reference parent paths, so the output must live under
    // internal/server/ rather than web/dist (see DESIGN_PLAN §5.4).
    outDir: '../internal/server/dist',
    // Keep false: the committed .gitkeep placeholder must survive builds so
    // `go build` compiles on a fresh checkout. build.sh cleans stale assets.
    emptyOutDir: false,
    sourcemap: false,
    chunkSizeWarningLimit: 1500,
  },
});
