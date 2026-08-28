import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@mlightcad/cad-viewer/style.css': fileURLToPath(new URL('./node_modules/@mlightcad/cad-viewer/dist/cad-viewer.css', import.meta.url)),
      '@mlightcad/cad-agent-plugin/style.css': fileURLToPath(new URL('./node_modules/@mlightcad/cad-agent-plugin/dist/style.css', import.meta.url)),
    },
  },
  build: {
    cssMinify: false,
  },
  optimizeDeps: {
    // MLightCAD is loaded on demand, but its package must still be prepared
    // when Vite starts; otherwise the first dynamic import can point to a
    // missing .vite/deps file after dependencies were installed or updated.
    include: [
      '@mlightcad/cad-simple-viewer',
      '@mlightcad/cad-viewer',
      '@mlightcad/data-model',
      '@mlightcad/mtext-renderer',
      '@mlightcad/three-renderer',
      '@mlightcad/libredwg-converter',
      '@mlightcad/libredwg-web',
      '@velipso/polybool',
      'lodash-es',
    ],
  },
})
