import { defineConfig, devices } from '@playwright/test'

// 隔离 Vite 端口，无 API 代理；测试只使用合成路由响应，不接触开发控制面。
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 90000,
  reporter: [['list']],
  outputDir: '../artifacts/frontend-platform',
  use: { baseURL: 'http://127.0.0.1:15174', trace: 'retain-on-failure' },
  projects: [{ name: 'chrome', use: { ...devices['Desktop Chrome'], channel: 'chrome' } }],
  webServer: {
    command: 'npx vite --config vite.platform.config.ts',
    url: 'http://127.0.0.1:15174/tests/fixtures/qualification.html',
    reuseExistingServer: false,
    timeout: 60000,
  },
})
