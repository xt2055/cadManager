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
    },
  },
  optimizeDeps: {
    // MLightCAD is loaded on demand, but its package must still be prepared
    // when Vite starts; otherwise the first dynamic import can point to a
    // missing .vite/deps file after dependencies were installed or updated.
    include: [
      '@mlightcad/cad-simple-viewer',
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
