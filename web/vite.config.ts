import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: '/vue/',
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8081',
    },
  },
  build: {
    outDir: '../public/vue',
    emptyOutDir: true,
  },
})
