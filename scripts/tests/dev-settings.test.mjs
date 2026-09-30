import assert from 'node:assert/strict'
import test from 'node:test'
import { developmentSettings, shouldReload } from '../dev-settings.mjs'

test('显式开启真实能力，并在后续无参数启动时保持开启及原身份配置', () => {
  for (const mode of ['control-plane', 'agent']) {
    const original = {
      publicUrl: 'https://127.0.0.1:18443', controlPlaneUrl: 'https://127.0.0.1:18443',
      stateDirectory: 'data/agent-security', realExecutionEnabled: false,
      controlPlaneCaFile: 'control-plane-ca.pem', retainedField: 'synthetic-value',
    }
    const enabled = developmentSettings(original, mode, true)
    assert.deepEqual(enabled, { ...original, realExecutionEnabled: true })
    assert.deepEqual(developmentSettings(enabled, mode, false), enabled)
    assert.deepEqual(developmentSettings(original, mode, false), original)
    assert.equal(original.realExecutionEnabled, false)
  }
})

test('无效开关、外部地址及其他身份目录仍拒绝', () => {
  const config = { controlPlaneUrl: 'https://127.0.0.1:18443', stateDirectory: 'data/agent-security', realExecutionEnabled: true }
  for (const change of [{ realExecutionEnabled: 'true' }, { controlPlaneUrl: 'https://example.test' }, { stateDirectory: '../existing-identity' }]) {
    assert.throws(() => developmentSettings({ ...config, ...change }, 'agent', true))
  }
})

test('真实模式只有手工请求才重启，合成模式保留源码自动重启', () => {
  for (const real of [false, true]) {
    for (const changed of [false, true]) {
      for (const requested of [false, true]) {
        assert.equal(shouldReload(real, changed, requested), requested || (!real && changed))
      }
    }
  }
})
