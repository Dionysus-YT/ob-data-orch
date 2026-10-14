import assert from 'node:assert/strict'
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { buildDevelopmentAgentPackages } from '../dev-agent-packages.mjs'

async function fixture(t) {
  const repository = await mkdtemp(path.join(os.tmpdir(), 'obdo-dev-packages-'))
  t.after(() => rm(repository, { recursive: true, force: true }))
  const distribution = path.join(repository, 'scripts', 'distribution')
  const controlBase = path.join(repository, 'var', 'dev', 'control-plane')
  await mkdir(distribution, { recursive: true })
  await mkdir(path.join(controlBase, 'data'), { recursive: true })
  for (const name of ['启动.cmd', 'manage.ps1', 'start.sh']) await writeFile(path.join(distribution, name), `synthetic-${name}`)
  await writeFile(path.join(controlBase, 'data', 'settings.json'), 'synthetic-retained-settings')
  await writeFile(path.join(controlBase, 'data', 'control-plane-ca.pem'), 'synthetic-retained-trust')
  return { repository, distribution, controlBase }
}

test('首次开发构建发布三个新命名平台和完整入口，保留设置与信任材料', async t => {
  const { repository, controlBase } = await fixture(t)
  const calls = []
  await buildDevelopmentAgentPackages(repository, controlBase, async (output, goOS, goArch) => {
    calls.push([goOS, goArch])
    await writeFile(output, `synthetic-${goOS}-${goArch}`)
  })
  assert.deepEqual(calls, [['windows', 'amd64'], ['linux', 'amd64'], ['linux', 'arm64']])
  const packages = path.join(controlBase, 'agents')
  assert.deepEqual((await readdir(packages)).sort(), ['linux-aarch64', 'linux-x86_64', 'windows-x86_64'])
  for (const [platform, binary, content, launchers] of [
    ['windows-x86_64', 'agent.exe', 'synthetic-windows-amd64', ['启动.cmd', 'manage.ps1']],
    ['linux-x86_64', 'agent', 'synthetic-linux-amd64', ['start.sh']],
    ['linux-aarch64', 'agent', 'synthetic-linux-arm64', ['start.sh']],
  ]) {
    assert.equal(await readFile(path.join(packages, platform, binary), 'utf8'), content)
    assert.deepEqual((await readdir(path.join(packages, platform))).sort(), [binary, ...launchers].sort())
    for (const launcher of launchers) assert.equal(await readFile(path.join(packages, platform, launcher), 'utf8'), `synthetic-${launcher}`)
  }
  assert.equal(await readFile(path.join(controlBase, 'data', 'settings.json'), 'utf8'), 'synthetic-retained-settings')
  assert.equal(await readFile(path.join(controlBase, 'data', 'control-plane-ca.pem'), 'utf8'), 'synthetic-retained-trust')
  assert.deepEqual((await readdir(controlBase)).sort(), ['agents', 'data'])
})

test('成功重建整套替换分发资源并清除旧名称与暂存目录', async t => {
  const { repository, controlBase } = await fixture(t)
  await mkdir(path.join(controlBase, 'agents', 'windows-amd64'), { recursive: true })
  await writeFile(path.join(controlBase, 'agents', 'windows-amd64', 'agent.exe'), 'synthetic-old-binary')
  for (const generation of ['first', 'second']) {
    await buildDevelopmentAgentPackages(repository, controlBase, output => writeFile(output, `synthetic-${generation}`))
  }
  assert.equal(await readFile(path.join(controlBase, 'agents', 'windows-x86_64', 'agent.exe'), 'utf8'), 'synthetic-second')
  assert.deepEqual((await readdir(path.join(controlBase, 'agents'))).sort(), ['linux-aarch64', 'linux-x86_64', 'windows-x86_64'])
  assert.deepEqual((await readdir(controlBase)).sort(), ['agents', 'data'])
})

for (const failure of ['build', 'missing-launcher', 'missing-binary']) {
  test(`构建失败保留上一套完整资源且不发布半成品：${failure}`, async t => {
    const { repository, distribution, controlBase } = await fixture(t)
    await buildDevelopmentAgentPackages(repository, controlBase, output => writeFile(output, 'synthetic-previous'))
    if (failure === 'missing-launcher') await rm(path.join(distribution, 'start.sh'))
    await assert.rejects(buildDevelopmentAgentPackages(repository, controlBase, async (output, goOS) => {
      if (goOS === 'linux') {
        if (failure === 'build') throw new Error('synthetic-build-failure')
        if (failure === 'missing-binary') return
      }
      await writeFile(output, 'synthetic-unpublished')
    }))
    for (const [platform, binary] of [['windows-x86_64', 'agent.exe'], ['linux-x86_64', 'agent'], ['linux-aarch64', 'agent']]) {
      assert.equal(await readFile(path.join(controlBase, 'agents', platform, binary), 'utf8'), 'synthetic-previous')
    }
    assert.equal(await readFile(path.join(controlBase, 'data', 'settings.json'), 'utf8'), 'synthetic-retained-settings')
    assert.deepEqual((await readdir(controlBase)).sort(), ['agents', 'data'])
  })
}
