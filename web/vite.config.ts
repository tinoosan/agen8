import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/rpc': 'http://localhost:8080',
      '/events': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
