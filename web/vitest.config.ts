import { defineConfig } from 'vitest/config'
import path from 'path'

// Separate from vite.config.ts on purpose: the app config runs the TanStack
// router plugin, which regenerates routeTree.gen.ts. A test run should not
// rewrite source files.
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  test: {
    // jsdom is installed for the component tests this makes possible; the
    // suite below is pure logic and does not need a DOM.
    environment: 'node',
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})
