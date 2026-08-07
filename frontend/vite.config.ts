/// <reference types="vitest" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react-swc'
import { resolve } from 'path'

export default defineConfig({
  base: '/static/',
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  build: {
    rollupOptions: {
      output: {
        // Force heavy vendor libraries into their own chunks so they don't
        // bloat the main bundle and only download when actually needed.
        // @lobehub/ui alone pulls shiki + mermaid + cytoscape; keeping it
        // out of index.js takes the main chunk from 3.3 MB → far less and
        // lets the chat page load its deps lazily.
        manualChunks: {
          'lobe-ui': ['@lobehub/ui']
        }
      }
    }
  },
  server: {
    port: 7002,
    proxy: {
      '/api': {
        target: 'https://console.zerone.life',
        changeOrigin: true,
        secure: false
      },
      '/auth': {
        target: 'https://console.zerone.life',
        changeOrigin: true,
        secure: false
      }
    }
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: true,
    // React 19 ships react/jsx-dev-runtime.production.js with
    // `exports.jsxDEV = void 0` — JSX in tests therefore breaks with
    // "jsxDEV is not a function" unless we keep NODE_ENV out of
    // 'production'. vitest sets NODE_ENV=production by default.
    env: {
      NODE_ENV: 'development'
    },
    server: {
      deps: {
        // @lobehub/ui ships pre-bundled .mjs that imports
        // @emoji-mart/data (a .json file) without the ESM `with { type: 'json' }`
        // attribute. Node's native ESM loader rejects this; inline-transforming
        // these modules lets Vite's json plugin handle them in tests.
        inline: ['@lobehub/ui', '@emoji-mart/data', '@emoji-mart/react']
      }
    }
  }
})
