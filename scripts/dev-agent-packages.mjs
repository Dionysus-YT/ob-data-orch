import { copyFile, mkdir, mkdtemp, rename, rm, stat } from 'node:fs/promises'
import path from 'node:path'

// buildDevelopmentAgentPackages 在独立目录构建三平台下载资源，全部完成后发布；不操作运行数据或 Agent 身份。
export async function buildDevelopmentAgentPackages(repository, controlBase, buildAgent) {
  const targets = [
    { platform: 'windows-x86_64', os: 'windows', arch: 'amd64', binary: 'agent.exe', launchers: ['启动.cmd', 'manage.ps1'] },
    { platform: 'linux-x86_64', os: 'linux', arch: 'amd64', binary: 'agent', launchers: ['start.sh'] },
    { platform: 'linux-aarch64', os: 'linux', arch: 'arm64', binary: 'agent', launchers: ['start.sh'] },
  ]
  await mkdir(controlBase, { recursive: true })
  const staging = await mkdtemp(path.join(controlBase, '.agents-build-'))
  const destination = path.join(controlBase, 'agents')
  const previous = `${staging}-previous`
  let retained = false
  try {
    for (const target of targets) {
      const directory = path.join(staging, target.platform)
      await mkdir(directory)
      const binary = path.join(directory, target.binary)
      await buildAgent(binary, target.os, target.arch)
      const info = await stat(binary)
      if (!info.isFile() || info.size === 0) throw new Error(`Agent ${target.platform} 构建输出无效`)
      for (const launcher of target.launchers) {
        await copyFile(path.join(repository, 'scripts', 'distribution', launcher), path.join(directory, launcher))
      }
    }
    // 失败构建不得替换上一套完整资源；正常开发重建在控制面停服后进入此发布步骤。
    try { await rename(destination, previous); retained = true }
    catch (error) { if (error.code !== 'ENOENT') throw error }
    try { await rename(staging, destination) }
    catch (error) {
      if (retained) await rename(previous, destination)
      throw error
    }
    if (retained) await rm(previous, { recursive: true, force: true })
  } finally {
    // staging 是 controlBase 内由 mkdtemp 创建的本次独占目录，只清除未发布的临时构建。
    await rm(staging, { recursive: true, force: true })
  }
}
