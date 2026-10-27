import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The React UI talks to one local Go node. By default Vite proxies /api and
// /ws to a node on :8080. Open the UI with ?node=http://localhost:8081 to
// attach it to a different node.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/ws': { target: 'ws://localhost:8080', ws: true },
    },
  },
})
