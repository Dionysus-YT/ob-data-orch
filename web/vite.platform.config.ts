import { mergeConfig } from 'vite'
import base from './vite.config'

// 浏览器验证复用真实编译链，HMR 仅连接隔离端口，不连接开发控制面。
export default mergeConfig(base, { server: { host: '127.0.0.1', port: 15174, strictPort: true, hmr: true, ws: { protocol: 'ws', host: '127.0.0.1', clientPort: 15174 } } })
