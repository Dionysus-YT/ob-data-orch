<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

import DataSourceConfirmDialog from './DataSourceConfirmDialog.vue'
import DataSourceFormView from './DataSourceFormView.vue'

const props = withDefaults(
  defineProps<{ modelValue: boolean; dataSourceId?: string | null; focusTest?: boolean }>(),
  { dataSourceId: null, focusTest: false },
)
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; saved: [id: string] }>()

const dirty = ref(false)
const discardConfirmVisible = ref(false)
const drawerElement = ref<HTMLElement>()
const closeButton = ref<HTMLButtonElement>()
const title = computed(() => (props.dataSourceId ? '编辑数据源' : '新增数据源'))
let previousBodyOverflow = ''
let previouslyFocusedElement: HTMLElement | null = null

function getFocusableElements(): HTMLElement[] {
  if (!drawerElement.value) return []

  return Array.from(
    drawerElement.value.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  )
}

function onDrawerKeydown(event: KeyboardEvent) {
  if (!props.modelValue || discardConfirmVisible.value) return

  if (event.key === 'Escape') {
    event.preventDefault()
    requestClose()
    return
  }

  if (event.key !== 'Tab') return

  const focusableElements = getFocusableElements()
  if (focusableElements.length === 0) {
    event.preventDefault()
    closeButton.value?.focus()
    return
  }

  const firstElement = focusableElements[0]
  const lastElement = focusableElements[focusableElements.length - 1]
  if (event.shiftKey && document.activeElement === firstElement) {
    event.preventDefault()
    lastElement?.focus()
  } else if (!event.shiftKey && document.activeElement === lastElement) {
    event.preventDefault()
    firstElement?.focus()
  }
}

watch(
  () => props.modelValue,
  async (visible) => {
    if (visible) {
      previouslyFocusedElement = document.activeElement as HTMLElement | null
      previousBodyOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      window.addEventListener('keydown', onDrawerKeydown)
      await nextTick()
      closeButton.value?.focus()
      return
    }
    window.removeEventListener('keydown', onDrawerKeydown)
    document.body.style.overflow = previousBodyOverflow
    dirty.value = false
    discardConfirmVisible.value = false
    previouslyFocusedElement?.focus()
    previouslyFocusedElement = null
  },
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onDrawerKeydown)
  document.body.style.overflow = previousBodyOverflow
})

function requestClose() {
  if (dirty.value) {
    discardConfirmVisible.value = true
    return
  }
  close()
}

function close() {
  emit('update:modelValue', false)
}

function discardAndClose() {
  dirty.value = false
  discardConfirmVisible.value = false
  close()
}

function onSaved(id: string) {
  dirty.value = false
  emit('saved', id)
}
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer-fade">
      <div v-if="modelValue" class="data-source-drawer-mask" @click.self="requestClose">
        <aside ref="drawerElement" class="data-source-drawer" role="dialog" aria-modal="true" :aria-labelledby="'data-source-drawer-title'">
          <header class="data-source-drawer-header">
            <div class="data-source-drawer-heading">
              <h2 id="data-source-drawer-title">{{ title }}</h2>
              <p v-if="dataSourceId" class="data-source-drawer-id">{{ dataSourceId }}</p>
              <p v-else>保存后可在当前 Drawer 中继续选择执行节点并进行真实连接测试。</p>
            </div>
            <button ref="closeButton" type="button" class="drawer-close" aria-label="关闭" @click="requestClose">
              <svg viewBox="0 0 20 20" aria-hidden="true">
                <path d="M5 5l10 10M15 5L5 15" />
              </svg>
            </button>
          </header>
          <div class="data-source-drawer-body">
            <DataSourceFormView
              :data-source-id="dataSourceId"
              :standalone="false"
              :focus-test="focusTest"
              @saved="onSaved"
              @cancel="requestClose"
              @dirty-change="dirty = $event"
            />
          </div>
        </aside>
      </div>
    </Transition>
  </Teleport>

  <DataSourceConfirmDialog
    :open="discardConfirmVisible"
    title="放弃未保存的更改？"
    description="当前 Drawer 中仍有未保存内容。关闭后，这些内容将无法恢复。"
    confirm-label="放弃更改"
    danger
    @cancel="discardConfirmVisible = false"
    @confirm="discardAndClose"
  />
</template>

<style scoped>
.data-source-drawer-mask {
  position: fixed;
  inset: 0;
  z-index: 100;
  background: rgb(15 23 42 / 42%);
}

.data-source-drawer {
  position: absolute;
  inset-block: 0;
  right: 0;
  display: flex;
  flex-direction: column;
  width: min(var(--app-drawer-width, 680px), calc(100vw - 72px));
  background: #fff;
  box-shadow: -14px 0 36px rgb(15 23 42 / 18%);
}

.data-source-drawer-header {
  display: flex;
  flex: none;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  min-height: 76px;
  padding: 16px 24px 14px;
  border-bottom: 1px solid #dde3ea;
}

.data-source-drawer-heading {
  min-width: 0;
}

.data-source-drawer-header h2 {
  margin: 0;
  color: #1f2937;
  font-size: 18px;
  font-weight: 600;
  line-height: 26px;
}

.data-source-drawer-header p {
  margin: 4px 0 0;
  color: #66758a;
  font-size: 12px;
  line-height: 18px;
}

.data-source-drawer-header .data-source-drawer-id {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
}

.drawer-close {
  display: grid;
  flex: none;
  width: 32px;
  height: 32px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  color: #657489;
  background: transparent;
  cursor: pointer;
}

.drawer-close svg {
  width: 18px;
  height: 18px;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-width: 1.7;
}

.drawer-close:hover {
  color: #263548;
  background: #f1f4f8;
}

.drawer-close:focus-visible {
  outline: 2px solid #2563c9;
  outline-offset: 2px;
}

.data-source-drawer-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.drawer-fade-enter-active,
.drawer-fade-leave-active {
  transition: opacity 180ms ease;
}

.drawer-fade-enter-active .data-source-drawer,
.drawer-fade-leave-active .data-source-drawer {
  transition: transform 200ms ease;
}

.drawer-fade-enter-from,
.drawer-fade-leave-to {
  opacity: 0;
}

.drawer-fade-enter-from .data-source-drawer,
.drawer-fade-leave-to .data-source-drawer {
  transform: translateX(100%);
}

@media (max-width: 760px) {
  .data-source-drawer {
    width: 100vw;
  }

  .data-source-drawer-header {
    padding-inline: 18px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .drawer-fade-enter-active,
  .drawer-fade-leave-active,
  .drawer-fade-enter-active .data-source-drawer,
  .drawer-fade-leave-active .data-source-drawer {
    transition: none;
  }
}
</style>
