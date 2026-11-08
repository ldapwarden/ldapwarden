import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import path from 'path'

export default defineConfig({
  plugins: [
    // autoCodeSplitting gives each route its own chunk, loaded on navigation:
    // the sign-in page no longer downloads every detail form and the admin
    // page. The router renders defaultPendingComponent while a chunk loads.
    // Must come before react().
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
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
