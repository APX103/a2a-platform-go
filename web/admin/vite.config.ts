import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const apiTarget = process.env.VITE_DEV_API_PROXY ?? 'http://localhost:18090'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    port: 3001,
    strictPort: true,
    proxy: {
      '/api': apiTarget,
      '/health': apiTarget,
      '/agent/': apiTarget,
      '/.well-known': apiTarget,
    }
  }
})
