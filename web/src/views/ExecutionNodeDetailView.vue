<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const nodeLabel = computed(() => typeof route.params.id === 'string' && route.params.id ? route.params.id : '未指定节点')
</script>

<template>
  <section class="page-heading"><div><h1>执行节点详情</h1><p>节点详情展示注册、心跳、环境、容量和有限任务关系的事实；不提供远程命令、文件浏览、工具安装或任务级路径检查。</p></div><RouterLink class="button button-secondary" to="/nodes">返回执行节点</RouterLink></section>
  <section class="content-card task-detail-empty"><div class="detail-kicker">请求的节点</div><h2>{{ nodeLabel }}</h2><div class="empty-state"><div class="empty-mark">□</div><h2>节点详情功能尚未接入</h2><p>Agent 关联和心跳事实接入后，本页只显示当前操作者有权查看的节点信息。节点离线不会直接宣称运行任务失败。</p></div></section>
  <section class="module-sections task-detail-sections"><article class="content-card section-placeholder"><h2>四维状态与接收新任务</h2><p>管理、在线、环境和容量分别表达；接收结论说明阻断原因，不自动改派等待或运行任务。</p></article><article class="content-card section-placeholder"><h2>工具与环境事实</h2><p>仅显示 Agent 上报的工具版本、固定环境检查、默认目录和配置指纹摘要；不展示 Token 或编辑节点配置。</p></article><article class="content-card section-placeholder"><h2>资源快照</h2><p>资源信息只作为节点事实和任务选择依据，不构建监控大屏或用平台服务器资源替代节点状态。</p></article><article class="content-card section-placeholder"><h2>关联任务与事件</h2><p>仅展示有限关联任务和节点事件；任务操作进入任务详情，跨节点或长时间日志查询进入日志中心。</p></article></section>
</template>
