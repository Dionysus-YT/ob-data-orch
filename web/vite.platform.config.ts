import { mergeConfig } from 'vite'
import base from './vite.config'

// 浏览器验证复用真实编译链，但与正在运行的开发服务隔离预构建缓存，避免争用缓存刷新当前会话。
// HMR 仅连接隔离端口，不连接开发控制面。
export default mergeConfig(base, { cacheDir: 'node_modules/.vite-platform', server: { host: '127.0.0.1', port: 15174, strictPort: true, hmr: true, ws: { protocol: 'ws', host: '127.0.0.1', clientPort: 15174 } } })
