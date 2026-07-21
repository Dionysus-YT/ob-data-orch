<script setup lang="ts">
import { runtimeConfig } from '@/config/runtime'
</script>

<template>
  <main class="shell">
    <section
      class="status-card"
      aria-labelledby="page-title"
    >
      <p class="eyebrow">
        OB Data Orch
      </p>
      <h1 id="page-title">
        工程骨架已启动
      </h1>
      <p class="description">
        当前仅验证基础工程、路由与构建链路，不提供产品功能或真实任务执行入口。
      </p>
      <dl class="facts">
        <div>
          <dt>开发门禁</dt>
          <dd>{{ runtimeConfig.stage }}</dd>
        </div>
        <div>
          <dt>真实任务执行</dt>
          <dd>{{ runtimeConfig.realExecutionEnabled ? '开启' : '关闭' }}</dd>
        </div>
      </dl>
    </section>
  </main>
</template>
