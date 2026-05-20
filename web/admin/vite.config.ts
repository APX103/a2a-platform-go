import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  server: {
    port: 3001,
    proxy: {
      '/api': 'http://localhost:18090',
      '/health': 'http://localhost:18090',
    }
  }
})
