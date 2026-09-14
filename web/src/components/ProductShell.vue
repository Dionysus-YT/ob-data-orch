<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, ref, watch } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import { ArrowDownToLine, ArrowUpFromLine, Database, House, KeyRound, LayoutTemplate, ListTodo, ScrollText, Server, Settings, Zap } from '@lucide/vue'
import ProductHeader from './ProductHeader.vue'
import OrchDialog from '@/workbench/components/OrchDialog.vue'
import { workbenchFoundationKey } from './workbenchFoundation'

const route = useRoute()
const foundation = computed(() => route.meta.workbenchFoundation === true)
provide(workbenchFoundationKey, foundation)
const helpOpen = ref(false)
const navigationOpen = ref(false)
const navigationElement = ref<HTMLElement>()
const navigationToggle = ref<{ focus: () => void }>()
const navigationHint = ref<{ label: string; top: number; left: number }>()
let hintTimer: ReturnType<typeof setTimeout> | undefined
function hideNavigationHint() {
  clearTimeout(hintTimer)
  navigationHint.value = undefined
}
function keepNavigationHint() { clearTimeout(hintTimer) }
function dismissNavigationHint() { hintTimer = setTimeout(hideNavigationHint, 120) }
// 提示使用独立固定浮层，避免被导航自身的滚动区域裁切；仅在图标轨模式显示。
function showNavigationHint(event: Event, label: string) {
  hideNavigationHint()
  if (!window.matchMedia('(min-width: 481px) and (max-width: 1280px)').matches) return
  const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect()
  navigationHint.value = { label, top: Math.max(72, Math.min(window.innerHeight - 24, bounds.top + bounds.height / 2)), left: bounds.right + 12 }
}
function dismissHintOnEscape(event: KeyboardEvent) { if (event.key === 'Escape') hideNavigationHint() }
onMounted(() => {
  window.addEventListener('resize', hideNavigationHint)
  window.addEventListener('scroll', hideNavigationHint, true)
  window.addEventListener('keydown', dismissHintOnEscape)
})
onBeforeUnmount(() => {
  hideNavigationHint()
  window.removeEventListener('resize', hideNavigationHint)
  window.removeEventListener('scroll', hideNavigationHint, true)
  window.removeEventListener('keydown', dismissHintOnEscape)
})
// 折叠导航在打开后将焦点移至当前位置，关闭后返回触发按钮，避免键盘用户跳过菜单。
watch(navigationOpen, async (open) => {
  await nextTick()
  if (open) (navigationElement.value?.querySelector<HTMLElement>('[aria-current="page"]') ?? navigationElement.value?.querySelector<HTMLElement>('a'))?.focus()
  else navigationToggle.value?.focus()
})
// 页面切换后关闭窄屏导航；草稿离开保护仍交由现有路由守卫处理。
watch(() => route.fullPath, () => { navigationOpen.value = false; hideNavigationHint() })

const navGroups = [
  { label: '运行概览', items: [{ label: '首页', to: '/', icon: House }] },
  { label: '任务配置', items: [{ label: '数据源管理', to: '/data-sources', icon: Database }, { label: '导出任务', to: '/exports/new', icon: ArrowUpFromLine }, { label: '普通导入', to: '/imports/normal/new', icon: ArrowDownToLine }, { label: '旁路导入', to: '/imports/direct/new', icon: Zap }, { label: '模板中心', to: '/templates', icon: LayoutTemplate }] },
  { label: '运行与支持', items: [{ label: '任务中心', to: '/tasks', icon: ListTodo }, { label: '执行节点', to: '/nodes', icon: Server }, { label: '日志中心', to: '/logs', icon: ScrollText }] },
  { label: '平台设置', items: [{ label: '系统设置', to: '/settings', icon: Settings }, { label: '存储凭据', to: '/settings/storage-credentials', icon: KeyRound }] },
]

const wideContent = computed(() => route.meta.contentWidth === 'wide')
const activeNavigation = computed(() => navGroups.flatMap((group) => group.items)
  .filter((item) => route.path === item.to || (item.to !== '/' && route.path.startsWith(`${item.to}/`)))
  .sort((a, b) => b.to.length - a.to.length)[0]?.to)
</script>

<template>
  <div class="product-layout enterprise-shell" :class="{ 'navigation-open': navigationOpen }">
    <a class="enterprise-skip" href="#main-workspace">跳至工作区</a>
    <ProductHeader ref="navigationToggle" :navigation-open="navigationOpen" @toggle-navigation="navigationOpen = !navigationOpen" @help="helpOpen = true" />
    <button v-if="navigationOpen" class="navigation-scrim" aria-label="关闭导航" @click="navigationOpen = false" />
    <aside id="product-navigation" ref="navigationElement" class="side-nav" aria-label="主导航" @keydown.esc="navigationOpen = false">
      <nav>
        <section v-for="group in navGroups" :key="group.label" class="nav-group">
          <p><span>{{ group.label }}</span></p>
          <RouterLink v-for="item in group.items" :key="item.to" :to="item.to" class="nav-item" :aria-label="item.label" :aria-current="activeNavigation === item.to ? 'page' : undefined" :class="{ active: activeNavigation === item.to }" @mouseenter="showNavigationHint($event, item.label)" @mouseleave="dismissNavigationHint" @focus="showNavigationHint($event, item.label)" @blur="hideNavigationHint" @click="hideNavigationHint">
            <component :is="item.icon" class="nav-icon" :size="16" :stroke-width="1.75" aria-hidden="true" /><span class="nav-label">{{ item.label }}</span>
          </RouterLink>
        </section>
      </nav>
    </aside>
    <div v-if="navigationHint" class="navigation-hint" role="tooltip" :style="{ top: `${navigationHint.top}px`, left: `${navigationHint.left}px` }" @mouseenter="keepNavigationHint" @mouseleave="dismissNavigationHint">{{ navigationHint.label }}</div>
    <div class="page-frame">
      <main id="main-workspace" class="page-content" :class="{ 'page-content-wide': wideContent }" tabindex="-1"><slot /></main>
    </div>
    <OrchDialog :open="helpOpen" title="OB Data Orch" confirm-label="关闭" @confirm="helpOpen = false" @cancel="helpOpen = false"><p>OB Loader / Dumper 4.3.5 的轻量可视化编排平台。</p><p>从数据源开始配置任务，在预检查后核对脱敏命令；执行状态与失败原因在任务中心查看。</p><p>数据源保存与连接测试相互独立，页面不会直接连接数据库。</p></OrchDialog>
  </div>
</template>
