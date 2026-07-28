import preact from '@preact/preset-vite'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vitest/config'

const packageInfo = JSON.parse(
  readFileSync(new URL('./package.json', import.meta.url), 'utf8'),
) as { version: string }

export default defineConfig({
  plugins: [preact()],
  define: {
    __PATTY_VIEW_VERSION__: JSON.stringify(packageInfo.version),
  },
  build: {
    rolldownOptions: {
      output: {
        entryFileNames: 'assets/pattyView.js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: (asset) => asset.names.some((name) => name.endsWith('.css'))
          ? 'assets/pattyView.css'
          : 'assets/[name][extname]',
      },
    },
  },
  worker: {
    rolldownOptions: {
      output: {
        entryFileNames: 'assets/[name].js',
        chunkFileNames: 'assets/[name].js',
        assetFileNames: 'assets/[name][extname]',
      },
    },
  },
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
