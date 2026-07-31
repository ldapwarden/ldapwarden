import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { TanStackRouterVite } from '@tanstack/router-plugin/vite'
import path from 'path'

export default defineConfig({
  plugins: [
    TanStackRouterVite(),
    react(),
  ],
  resolve: {
    alias: {
      // import.meta.dirname, not __dirname: vite 8.2 warns that its native
      // config loader — slated to become the default — cannot provide the CommonJS
      // globals.
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        // Override with VITE_API_PROXY_TARGET when the backend runs on a
        // non-default port (e.g. `SERVER_PORT=8888`).
        target: process.env.VITE_API_PROXY_TARGET || 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
})
