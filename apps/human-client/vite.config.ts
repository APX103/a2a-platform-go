import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const bffTarget = process.env.HUMAN_CLIENT_BFF ?? 'http://127.0.0.1:18100'

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
      '/api': bffTarget,
    },
  },
})
