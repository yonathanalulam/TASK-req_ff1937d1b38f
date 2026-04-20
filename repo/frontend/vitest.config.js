import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

// Unit-test config kept separate from vite.config.js so the production build
// isn't forced to pull in the jsdom/vitest deps. Tests run under jsdom and
// can import real Vue components via @vue/test-utils.
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    include: ['tests/unit/**/*.spec.{js,mjs,ts}'],
    coverage: {
      reporter: ['text', 'json-summary'],
    },
  },
})
