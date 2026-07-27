import preact from '@preact/preset-vite'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [preact()],
  server: {
    host: '127.0.0.1',
    port: 4177,
  },
  preview: {
    host: '127.0.0.1',
    port: 4177,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
})
