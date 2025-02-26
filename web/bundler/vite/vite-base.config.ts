import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { resolve } from 'path'
import { access } from 'node:fs/promises'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

// https://vite.dev/config/
export default defineConfig({
  build: {
    outDir: './vite-dist',
    rollupOptions: {
      external: [
        'react',
        'react-dom',
        '@aptre/bldr',
        '@aptre/bldr-react',
        '@aptre/protobuf-es-lite',
        'starpc',
      ],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
        },
      },
    },
    minify: false,
    sourcemap: true,
    cssCodeSplit: true,
    terserOptions: {
      compress: false,
      mangle: false,
    },
  },
  resolve: {
    alias: {
      '@go/*': resolve(__dirname, '../../../vendor/*'),
      '@aptre/bldr': resolve(__dirname, '../../../.bldr/src/web/bldr/index.js'),
      '@aptre/bldr-react': resolve(
        __dirname,
        '../../../.bldr/src/web/bldr-react/index.js',
      ),
    },
  },
  plugins: [
    react(),
    // tailwindcss(),
    {
      // This plugin fixes issues with resolving paths like @go/foo/bar/baz.js where baz.ts exists only.
      name: 'go-ts-resolver',
      async resolveId(source) {
        // Handle only @go/ paths that end in .js
        if (!source.startsWith('@go/') || !source.endsWith('.js')) {
          return null
        }

        // Convert @go/ path to vendor path
        const vendorPath = resolve(
          __dirname,
          '../../../vendor',
          source.slice('@go/'.length),
        )
        const tsPath = vendorPath.replace(/\.js$/, '.ts')

        try {
          await access(tsPath)
          return tsPath
        } catch {
          return null
        }
      },
    },
  ],
})
