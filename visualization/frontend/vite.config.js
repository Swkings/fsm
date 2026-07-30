import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.js',
    restoreMocks: true,
  },
  server: {
    proxy: {
      '/api': process.env.FSM_API_TARGET || 'http://127.0.0.1:5005',
    },
  },
})
