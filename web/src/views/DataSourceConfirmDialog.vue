<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description: string
    confirmLabel: string
    objectName?: string
    impacts?: readonly string[]
    danger?: boolean
    busy?: boolean
  }>(),
  { objectName: '', impacts: () => [], danger: false, busy: false },
)

const emit = defineEmits<{ cancel: []; confirm: [] }>()
const dialogElement = ref<HTMLElement>()
const cancelButton = ref<HTMLButtonElement>()

function getFocusableElements(): HTMLElement[] {
  if (!dialogElement.value) return []

  return Array.from(
    dialogElement.value.querySelectorAll<HTMLElement>(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  )
}

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    await nextTick()
    cancelButton.value?.focus()
  },
)

function onKeydown(event: KeyboardEvent) {
  if (!props.open) return

  if (event.key === 'Escape' && !props.busy) {
    event.preventDefault()
    emit('cancel')
    return
  }

  if (event.key !== 'Tab') return

  const focusableElements = getFocusableElements()
  if (focusableElements.length === 0) {
    event.preventDefault()
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
  () => props.open,
  (open) => {
    if (open) window.addEventListener('keydown', onKeydown)
    else window.removeEventListener('keydown', onKeydown)
  },
  { immediate: true },
)

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <Transition name="data-source-dialog-fade">
      <div v-if="open" class="data-source-dialog-mask" @click.self="!busy && emit('cancel')">
        <section ref="dialogElement" class="data-source-dialog" role="alertdialog" aria-modal="true" :aria-labelledby="`${title}-title`" :aria-describedby="`${title}-description`">
          <header>
            <h2 :id="`${title}-title`">{{ title }}</h2>
          </header>
          <div class="data-source-dialog-body">
            <p :id="`${title}-description`">{{ description }}</p>
            <p v-if="objectName" class="data-source-dialog-object">{{ objectName }}</p>
            <ul v-if="impacts.length">
              <li v-for="impact in impacts" :key="impact">{{ impact }}</li>
            </ul>
          </div>
          <footer>
            <button ref="cancelButton" type="button" class="data-source-dialog-button" :disabled="busy" @click="emit('cancel')">取消</button>
            <button type="button" class="data-source-dialog-button" :class="danger ? 'is-danger' : 'is-primary'" :disabled="busy" @click="emit('confirm')">
              {{ busy ? '处理中…' : confirmLabel }}
            </button>
          </footer>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.data-source-dialog-mask {
  position: fixed;
  inset: 0;
  z-index: 140;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgb(15 23 42 / 42%);
}

.data-source-dialog {
  width: min(460px, 100%);
  overflow: hidden;
  border: 1px solid #d7dee8;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 18px 44px rgb(15 23 42 / 20%);
}

.data-source-dialog header,
.data-source-dialog footer {
  padding: 16px 20px;
}

.data-source-dialog header {
  border-bottom: 1px solid #e1e6ed;
}

.data-source-dialog h2 {
  margin: 0;
  color: #1f2937;
  font-size: 16px;
  font-weight: 600;
  line-height: 24px;
}

.data-source-dialog-body {
  padding: 18px 20px 20px;
  color: #526174;
  font-size: 14px;
  line-height: 22px;
}

.data-source-dialog-body p {
  margin: 0;
}

.data-source-dialog-object {
  margin-top: 14px !important;
  padding: 9px 11px;
  border-left: 3px solid #c9d2dd;
  background: #f7f9fc;
  color: #273548;
  font-weight: 600;
}

.data-source-dialog ul {
  display: grid;
  gap: 6px;
  margin: 14px 0 0;
  padding-left: 20px;
}

.data-source-dialog footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  border-top: 1px solid #e1e6ed;
  background: #f9fafb;
}

.data-source-dialog-button {
  min-width: 76px;
  height: 36px;
  padding: 0 14px;
  border: 1px solid #c9d2dd;
  border-radius: 4px;
  color: #344256;
  background: #fff;
  font: 500 13px/1 "Segoe UI", "Microsoft YaHei UI", sans-serif;
  cursor: pointer;
}

.data-source-dialog-button:hover:not(:disabled) {
  border-color: #9aa8ba;
  background: #f7f9fc;
}

.data-source-dialog-button.is-primary {
  border-color: #2563c9;
  color: #fff;
  background: #2563c9;
}

.data-source-dialog-button.is-danger {
  border-color: #b42318;
  color: #fff;
  background: #b42318;
}

.data-source-dialog-button:focus-visible {
  outline: 2px solid #2563c9;
  outline-offset: 2px;
}

.data-source-dialog-button:disabled {
  color: #8a95a5;
  border-color: #dce2e9;
  background: #f1f3f6;
  cursor: not-allowed;
}

.data-source-dialog-fade-enter-active,
.data-source-dialog-fade-leave-active {
  transition: opacity 160ms ease;
}

.data-source-dialog-fade-enter-from,
.data-source-dialog-fade-leave-to {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .data-source-dialog-fade-enter-active,
  .data-source-dialog-fade-leave-active {
    transition: none;
  }
}
</style>
