import { defineConfig } from 'vite'
import { resolve } from 'path'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'

const __dirname = dirname(fileURLToPath(import.meta.url))

// https://vite.dev/config/
export default defineConfig({
  build: {
    lib: {
      entry: {
        example: resolve(__dirname, './example.tsx'),
        'example-class': resolve(__dirname, './example-class.tsx'),
      },
      name: 'Example',
      formats: ['es'],
    },
  },
  plugins: [
    react(),
  ],
})
