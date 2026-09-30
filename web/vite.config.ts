import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [vue()],
  // 安装包下载由控制面按当前地址生成，网页构建不复制任何运行数据。
  publicDir: false,
  // 开发页面和 HMR 都经同源 HTTPS 控制面访问，Vite 本身只监听回环。
  server: {
    host: '127.0.0.1',
    port: 15173,
    strictPort: true,
    hmr: { protocol: 'wss', host: '127.0.0.1', clientPort: 18443 },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
