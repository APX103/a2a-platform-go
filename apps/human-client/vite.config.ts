import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const apiTarget = process.env.HUMAN_CLIENT_BFF
  ?? process.env.VITE_A2A_PLATFORM_URL
  ?? 'http://127.0.0.1:18090'

export default defineConfig({
  base: './',
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      '/api': apiTarget,
      '/agent': apiTarget,
    },
  },
})
