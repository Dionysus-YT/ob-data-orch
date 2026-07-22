<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const taskLabel = computed(() => typeof route.params.id === 'string' && route.params.id ? route.params.id : '未指定任务')
</script>

<template>
  <section class="page-heading"><div><h1>任务详情</h1><p>已提交任务的配置、数据源、节点与命令均为不可变快照；所有命令默认只读且脱敏。</p></div><RouterLink class="button button-secondary" to="/tasks">返回任务中心</RouterLink></section>
  <section class="content-card task-detail-empty">
    <div class="detail-kicker">请求的任务</div><h2>{{ taskLabel }}</h2>
    <div class="empty-state"><div class="empty-mark">□</div><h2>任务详情功能尚未接入</h2><p>接入后本页将按同一任务上下文展示运行概览、配置快照、计划或实际启动命令、当前任务日志和操作记录；不会把跨任务日志查询塞进详情页。</p></div>
  </section>
  <section class="module-sections task-detail-sections">
    <article class="content-card section-placeholder"><h2>运行概览</h2><p>只在有可靠日志或结果证据时显示进度、阶段、持续时间和结果；无可靠解析时不显示百分比或 ETA。</p></article>
    <article class="content-card section-placeholder"><h2>配置快照与命令</h2><p>回显提交时的用户值、实际值、派生来源、工具默认、版本和预检查证据。默认只能复制脱敏命令。</p></article>
    <article class="content-card section-placeholder"><h2>执行日志</h2><p>这里只查看当前任务的增量日志；跨任务检索会跳转日志中心并带入任务条件。</p></article>
    <article class="content-card section-placeholder"><h2>安全操作</h2><p>取消、从头重新执行、基于原配置新建和条件化检查点继续均需重新核对权限、预检查和当前状态；旁路导入永不显示检查点继续。</p></article>
  </section>
</template>
