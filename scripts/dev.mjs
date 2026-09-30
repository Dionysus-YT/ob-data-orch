import { spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { once } from 'node:events'
import { mkdir, readFile, writeFile, copyFile, readdir, stat, rm, rename } from 'node:fs/promises'
import { createInterface } from 'node:readline'
import { developmentSettings, shouldReload } from './dev-settings.mjs'
import https from 'node:https'
import net from 'node:net'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { setTimeout as delay } from 'node:timers/promises'

const repository = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const mode = process.argv[2] ?? 'control-plane'
if (!['control-plane', 'agent'].includes(mode) || process.argv.slice(3).some(value => value !== '--real-execution')) throw new Error('用法：dev.ps1 [-Agent] [-RealExecution]')
const enableRealExecution = process.argv.includes('--real-execution')
if (process.platform !== 'win32' || process.arch !== 'x64') throw new Error('当前开发入口仅验证 Windows AMD64')
const base = path.join(repository, 'var', 'dev', mode)
const executable = path.join(base, `${mode}.exe`)
const controlBase = path.join(repository, 'var', 'dev', 'control-plane')
const publicURL = 'https://127.0.0.1:18443'
const environment = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('OB_DATA_ORCH_')))
Object.assign(environment, { CGO_ENABLED: '0', GOOS: 'windows', GOARCH: 'amd64' })
let child, vite, closing = false, lock, input, realExecutionEnabled = false, reloadRequested = false

async function configure(file) {
  const original = JSON.parse(await readFile(file))
  const settings = developmentSettings(original, mode, enableRealExecution)
  if (settings.realExecutionEnabled !== original.realExecutionEnabled) {
    const temporary = `${file}.${process.pid}.tmp`
    try {
      await writeFile(temporary, JSON.stringify(settings, null, 2), { flag: 'wx', mode: 0o600 })
      await rename(temporary, file)
    } finally { await rm(temporary, { force: true }) }
  }
  realExecutionEnabled = settings.realExecutionEnabled
}

function launch(command, args, options = {}) {
  const process = spawn(command, args, { cwd: repository, env: environment, stdio: 'inherit', windowsHide: true, ...options })
  // 立即注册退出承诺，避免进程快速退出后才开始等待。
  process.finished = new Promise((resolve, reject) => {
    process.once('error', reject)
    process.once('exit', code => resolve(code))
  })
  process.finished.catch(() => {})
  return process
}
async function command(program, args, options) {
  const result = await launch(program, args, options).finished
  if (result !== 0) throw new Error(`${path.basename(program)} 执行失败 (${result})`)
}
async function exists(file) { try { await stat(file); return true } catch (error) { if (error.code === 'ENOENT') return false; throw error } }
async function build() {
  // 固定开发目录中的旧程序已停止；删除后重建，不覆盖运行中的二进制。
  await rm(executable, { force: true })
  await command('go', ['build', '-trimpath', '-o', executable, `./cmd/${mode}`])
}
async function snapshot() {
  const hash = createHash('sha256')
  async function visit(directory) {
    for (const entry of (await readdir(directory, { withFileTypes: true })).sort((a, b) => a.name.localeCompare(b.name))) {
      const file = path.join(directory, entry.name)
      if (entry.isDirectory()) await visit(file)
      else if (/\.(go|sql|json)$/.test(entry.name)) { hash.update(file); hash.update(await readFile(file)) }
    }
  }
  for (const directory of [`cmd/${mode}`, 'internal', 'contracts', 'migrations']) await visit(path.join(repository, directory))
  for (const file of ['go.mod', 'go.sum']) hash.update(await readFile(path.join(repository, file)))
  return hash.digest('hex')
}
async function stop() {
  if (!child || child.exitCode !== null) return
  await writeFile(path.join(base, 'dev.stop'), '')
  const result = await Promise.race([child.finished.then(() => true), delay(25000, null, { ref: false }).then(() => false)])
  if (!result) throw new Error('开发进程未正常停止，已阻止替换二进制；请检查终端')
  child = undefined
}
async function checkPort(port) {
  const server = net.createServer()
  server.listen(port, '127.0.0.1')
  await once(server, 'listening')
  await new Promise(resolve => server.close(resolve))
}
async function health() {
  const ca = await readFile(path.join(controlBase, 'data', 'control-plane-ca.pem'))
  return new Promise((resolve, reject) => {
    const request = https.get(`${publicURL}/healthz`, { ca, timeout: 2000 }, response => {
      response.resume()
      response.statusCode === 200 ? resolve() : reject(new Error('健康响应无效'))
    })
    request.on('timeout', () => request.destroy(new Error('健康检查超时')))
    request.on('error', reject)
  })
}
async function start() {
  await rm(path.join(base, 'dev.stop'), { force: true })
  if (mode === 'control-plane') await checkPort(18443)
  child = launch(executable, mode === 'control-plane' ? ['-dev-web-url', 'http://127.0.0.1:15173'] : ['-dev-watch'])
  if (mode === 'control-plane') {
    let ready = false
    for (let attempt = 0; attempt < 40; attempt++) {
      if (child.exitCode !== null) break
      try { await health(); ready = true; break } catch { await delay(250) }
    }
    if (!ready) throw new Error('开发控制面健康检查失败')
  } else {
    await delay(1000)
    if (child.exitCode !== null) throw new Error('开发 Agent 启动失败')
  }
  console.log(mode === 'control-plane' ? `开发服务：${publicURL}；前端保存即更新。` : '开发 Agent 已启动；请在节点页确认心跳。')
  console.log(realExecutionEnabled ? '真实能力已开启。Go 改动不会自动重启；结束测试后输入 r 并回车应用修改。' : '合成模式：Go 改动自动重启。输入 r 并回车也可重编译重启。')
}
async function initialize() {
  if (mode === 'control-plane') {
    await mkdir(path.join(base, 'web'), { recursive: true })
    await copyFile(path.join(repository, 'web', 'index.html'), path.join(base, 'web', 'index.html'))
    if (!await exists(path.join(base, 'data', 'settings.json'))) {
      // 由 PowerShell 隐藏密码输入，仅通过标准输入传给初始化程序。
      await command('powershell.exe', ['-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', path.join(repository, 'scripts', 'dev-initialize.ps1')])
    }
    await configure(path.join(base, 'data', 'settings.json'))
  } else {
    await health()
    const configPath = path.join(base, 'agent-config.json')
    if (!await exists(configPath)) {
      await copyFile(path.join(controlBase, 'data', 'control-plane-ca.pem'), path.join(base, 'control-plane-ca.pem'))
      await writeFile(configPath, JSON.stringify({ formatVersion: 'agent-bundle-config-v1', controlPlaneUrl: publicURL, controlPlaneCaFile: 'control-plane-ca.pem', stateDirectory: 'data/agent-security', realExecutionEnabled: false }))
    }
    await configure(configPath)
    await command(executable, ['-initialize'])
  }
}
async function cleanup() {
  try { await stop() }
  finally { if (vite && vite.exitCode === null) { vite.kill(); await vite.finished } }
}
process.on('SIGINT', () => { closing = true })
process.on('SIGTERM', () => { closing = true })
try {
  await mkdir(base, { recursive: true })
  // 用本机独占端口防止重复监视器；进程退出后系统释放，不遗留锁文件。
  lock = net.createServer()
  lock.listen(mode === 'control-plane' ? 18444 : 18445, '127.0.0.1')
  await once(lock, 'listening')
  if (mode === 'control-plane') { await checkPort(18443); await checkPort(15173) }
  await build()
  await initialize()
  if (mode === 'control-plane') {
    vite = launch(process.execPath, [path.join(repository, 'web/node_modules/vite/bin/vite.js')], { cwd: path.join(repository, 'web') })
    await delay(1000)
    if (vite.exitCode !== null) throw new Error('Vite 启动失败，请先执行 npm --prefix web ci')
  }
  let previous = await snapshot()
  await start()
  input = createInterface({ input: process.stdin, terminal: false })
  input.on('line', line => { if (line.trim().toLowerCase() === 'r') reloadRequested = true })
  while (!closing) {
    await delay(1000)
    if (vite && vite.exitCode !== null) throw new Error('Vite 已退出')
    if (child && child.exitCode !== null) throw new Error('开发服务意外退出，请检查日志后重新启动开发入口')
    const current = await snapshot()
    const changed = current !== previous
    previous = current
    if (changed && realExecutionEnabled) console.log('检测到 Go/契约源码变化；当前测试继续运行。结束测试后输入 r 并回车重启。')
    if (!shouldReload(realExecutionEnabled, changed, reloadRequested)) continue
    reloadRequested = false
    await stop()
    try { await build(); if (!closing) await start() }
    catch (error) {
      await stop()
      child = undefined
      console.error(`${error.message}；保留数据，${realExecutionEnabled ? '修复后输入 r 并回车重试' : '等待下一次源码修改后重试'}。`)
    }
  }
} catch (error) {
  console.error(error.message)
  process.exitCode = 1
} finally {
  input?.close()
  try { await cleanup() } catch (error) { console.error(error.message); process.exitCode = 1 }
  if (lock?.listening) lock.close()
}
