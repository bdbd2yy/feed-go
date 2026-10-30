import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/static': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/api': {
        // Force IPv4 to avoid Windows resolving `localhost` -> `::1` (IPv6) and causing ECONNREFUSED
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
})
