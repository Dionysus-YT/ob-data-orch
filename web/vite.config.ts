import { existsSync, readFileSync } from 'node:fs'
import { Agent } from 'node:https'
import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// 本机开发页始终代理到回环 TLS 控制面，避免遗留 HTTP 环境变量造成协议错配。
const localMvpControlPlaneUrl = 'https://127.0.0.1:8080'
const localMvpCAFile = fileURLToPath(new URL('../var/local-mvp-tls/control-plane-ca.pem', import.meta.url))
// 启动脚本生成本机 CA 后由代理显式使用，缺失时仍保持验证失败，不能静默降级为不校验证书。
const localMvpProxyAgent = new Agent(existsSync(localMvpCAFile) ? { ca: readFileSync(localMvpCAFile) } : {})

export default defineConfig({
  plugins: [vue()],
  // Local MVP 下载包只存在于 Git 忽略的运行时目录，不能把 Agent 二进制提交进前端源码。
  publicDir: fileURLToPath(new URL('../var/local-mvp-web-assets', import.meta.url)),
  server: {
    proxy: {
      // 本机 MVP 代理仍验证控制面证书；通过 NODE_EXTRA_CA_CERTS 显式信任测试 CA，绝不跳过 TLS 校验。
      '/api': { target: localMvpControlPlaneUrl, changeOrigin: true, secure: true, agent: localMvpProxyAgent },
    },
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
