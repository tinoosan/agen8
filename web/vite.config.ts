import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const daemonProxyTarget = process.env.AGEN8_WEB_PROXY_TARGET || 'http://127.0.0.1:7777'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  // Pre-bundle ALL known heavy/Radix deps upfront so Vite never
  // re-optimizes mid-session. Without this, lazy-loaded pages trigger
  // on-demand optimization → stale bundles → "504 Outdated Optimize
  // Dep" → "Failed to fetch dynamically imported module" → blank page.
  optimizeDeps: {
    // Always rebuild optimized deps on dev-server start so stale
    // prebundles from prior lockfile/branch state can't leak into
    // the current session.
    force: true,
    include: [
      '@radix-ui/react-alert-dialog',
      '@radix-ui/react-avatar',
      '@radix-ui/react-checkbox',
      '@radix-ui/react-collapsible',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-label',
      '@radix-ui/react-popover',
      '@radix-ui/react-progress',
      '@radix-ui/react-scroll-area',
      '@radix-ui/react-select',
      '@radix-ui/react-separator',
      '@radix-ui/react-slot',
      '@radix-ui/react-switch',
      '@radix-ui/react-tabs',
      '@radix-ui/react-tooltip',
      '@xyflow/react',
      '@xyflow/system',
      'react-markdown',
      'remark-gfm',
      'framer-motion',
      '@tanstack/react-query',
      '@tanstack/react-table',
      '@uiw/react-codemirror',
      '@codemirror/view',
      '@codemirror/state',
      '@codemirror/commands',
      '@codemirror/search',
      '@codemirror/language',
      '@codemirror/lang-javascript',
      '@codemirror/lang-json',
      '@codemirror/lang-python',
      '@codemirror/lang-go',
      '@codemirror/lang-java',
      '@codemirror/lang-rust',
      '@codemirror/lang-sql',
      '@codemirror/lang-html',
      '@codemirror/lang-css',
      '@codemirror/lang-markdown',
      '@codemirror/lang-yaml',
      'sonner',
      'wouter',
      'lucide-react',
      'clsx',
      'zustand',
      '@dagrejs/dagre',
      'd3-force',
      'yaml',
    ],
  },
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    sourcemap: true,
    rollupOptions: {
      output: {
        manualChunks: {
          'query-vendor': ['@tanstack/react-query'],
          'markdown-vendor': ['react-markdown', 'remark-gfm'],
          'motion-vendor': ['framer-motion'],
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/rpc': daemonProxyTarget,
      '/api': daemonProxyTarget,
      '/auth/chatgpt': daemonProxyTarget,
      '/setup': daemonProxyTarget,
      '/events': {
        target: daemonProxyTarget,
        changeOrigin: true,
      },
    },
  },
})
