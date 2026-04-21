import { defineConfig } from 'vite'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { resolve } from 'path'
import { buildConfig } from './web/bundler/vite/build.js'

const __dirname = dirname(fileURLToPath(import.meta.url))
const EXAMPLE_DIR = './example'

// This is an example vite.config.ts for building the bldr example.

// https://vitejs.dev/config/
export default defineConfig(async () => {
  return buildConfig(
    {
      command: 'build',
      mode: process.env.NODE_ENV || 'development',
    },
    resolve(__dirname, EXAMPLE_DIR, 'vite.config.ts')
  )
})
