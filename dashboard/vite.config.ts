/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// En développement (npm run dev), les appels à l'API sont relayés vers cfa-api.
const api = process.env.CFA_API ?? 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    proxy: Object.fromEntries(['/events', '/goals', '/tasks', '/approvals', '/login', '/healthz'].map((p) => [p, api])),
  },
  test: { environment: 'node' },
});
