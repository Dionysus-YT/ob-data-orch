<script setup lang="ts">
import { Alert, Button, Form, FormItem, InputPassword, Modal } from 'ant-design-vue'
import { nextTick, onBeforeUnmount, onMounted, reactive, ref, useId } from 'vue'
import { AccountError, accountApi } from '@/api/account'
import { useLogoutContext } from '@/composables/useLogoutGuard'
import { activateModal } from './modalFocus'

const emit = defineEmits<{ close: [] }>()
const logoutContext = useLogoutContext(), controller = new AbortController(), api = accountApi(controller.signal)
const form = reactive({ current: '', next: '', confirmation: '' })
const errors = reactive({ current: '', next: '', confirmation: '' })
const busy = ref(false), failure = ref(''), id = useId()
const currentInput = ref<{ focus: () => void; $el: HTMLElement }>()
let disposed = false, release: (() => void) | undefined
function clearSecrets() { form.current = ''; form.next = ''; form.confirmation = '' }
function close() { if (!busy.value && !disposed) { clearSecrets(); emit('close') } }
onMounted(async () => {
  await nextTick()
  const root = currentInput.value?.$el.closest<HTMLElement>('[role="dialog"]')
  const input = root?.querySelector<HTMLInputElement>('input')
  if (!disposed && root && input) release = activateModal(root, input, close)
})
onBeforeUnmount(() => { disposed = true; controller.abort(); clearSecrets(); release?.() })

async function submit() {
  if (disposed || busy.value) return
  errors.current = form.current ? '' : '请输入当前密码'
  const size = new TextEncoder().encode(form.next).length
  errors.next = size < 8 || size > 256 ? '新密码须为 8 至 256 字节' : form.next === form.current ? '新密码不能与当前密码相同' : ''
  errors.confirmation = form.confirmation !== form.next || !form.confirmation ? '两次输入的新密码不一致' : ''
  if (errors.current || errors.next || errors.confirmation) return
  busy.value = true
  failure.value = ''
  try {
    if (!logoutContext || !await logoutContext.confirm() || disposed) return
    await api.changePassword(form.current, form.next)
    if (disposed) return
    clearSecrets()
    logoutContext.commit()
    window.location.assign('/login')
  } catch (error) {
    if (disposed) return
    const problem = error instanceof AccountError ? error : new AccountError(0, 'REQUEST_FAILED')
    if (problem.code === 'CURRENT_PASSWORD_INVALID') { errors.current = '当前密码不正确'; form.current = ''; currentInput.value?.focus() }
    else if (problem.code === 'PASSWORD_FIELDS_INVALID') failure.value = '密码未通过校验，请检查长度并使用不同的新密码。'
    else if (problem.status === 429) failure.value = '尝试过于频繁，请 1 分钟后再试。'
    else if (problem.status === 401) { clearSecrets(); failure.value = '登录已失效，请重新登录后修改密码。' }
    else if (problem.status === 403 || problem.code === 'CSRF_TOKEN_UNAVAILABLE') failure.value = '页面安全校验失效，未修改密码。请关闭弹窗并刷新页面。'
    else if (problem.code === 'PASSWORD_CHANGE_UNAVAILABLE') failure.value = '密码未能保存，请稍后重试；若持续失败，请联系管理员检查安装配置。'
    else { clearSecrets(); failure.value = '修改结果暂无法确认，请先尝试重新登录；如原密码仍有效，可重新修改。' }
  } finally { if (!disposed) busy.value = false }
}
</script>

<template>
  <Modal :open="true" title="修改密码" :width="400" :closable="!busy" :mask-closable="false" :keyboard="!busy" :destroy-on-close="true" :aria-describedby="`${id}-notice ${id}-impact`" @cancel="close">
    <div class="orch-confirmation-body">
      <div class="account-password-body">
        <Alert :id="`${id}-notice`" type="info" show-icon message="为保障密码安全，建议不要重复使用相同密码" />
        <Alert v-if="failure" type="error" show-icon :message="failure" />
        <Form layout="vertical" :disabled="busy" @finish="submit">
          <FormItem label="当前密码" :html-for="`${id}-current`" :validate-status="errors.current ? 'error' : undefined" :help="errors.current || undefined"><InputPassword :id="`${id}-current`" ref="currentInput" v-model:value="form.current" aria-label="当前密码" placeholder="请输入" autocomplete="current-password" :aria-invalid="Boolean(errors.current)" @change="errors.current = ''" /></FormItem>
          <FormItem label="新密码" :html-for="`${id}-next`" :validate-status="errors.next ? 'error' : undefined" :help="errors.next || undefined"><InputPassword :id="`${id}-next`" v-model:value="form.next" aria-label="新密码" placeholder="请输入密码" autocomplete="new-password" :aria-invalid="Boolean(errors.next)" @change="errors.next = ''" /></FormItem>
          <FormItem label="确认新密码" :html-for="`${id}-confirmation`" :validate-status="errors.confirmation ? 'error' : undefined" :help="errors.confirmation || undefined"><InputPassword :id="`${id}-confirmation`" v-model:value="form.confirmation" aria-label="确认新密码" placeholder="请再次输入新密码" autocomplete="new-password" :aria-invalid="Boolean(errors.confirmation)" @change="errors.confirmation = ''" @press-enter="submit" /></FormItem>
          <p :id="`${id}-impact`" class="section-hint">修改后所有已登录会话将退出。</p>
        </Form>
      </div>
    </div>
    <template #footer><Button :disabled="busy" @click="close">取消</Button><Button type="primary" aria-label="确定" :aria-busy="busy" :loading="busy" :disabled="busy" @click="submit">确定</Button></template>
  </Modal>
</template>
