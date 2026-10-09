import { computed, ref, watch, type Ref } from 'vue'
import { executionNodeErrorMessage, type ExecutionNodeDetail, type ExecutionNodeEnrollment } from '@/api/browser'
import { agentRegistrationCode, requiresAgentRegistration } from './executionNodeEnrollmentInstructions'
import type { NodeSession } from './nodeSession'

// 材料只属于当前节点的弹层会话；关闭、路由切换、关联完成和卸载都清除引用。
export function useNodeEnrollment(node: Readonly<Ref<ExecutionNodeDetail | undefined>>, session: NodeSession, copy: (value: string) => Promise<void> = value => navigator.clipboard.writeText(value)) {
  const enrollmentDialogOpen = ref(false)
  const enrollment = ref<ExecutionNodeEnrollment>()
  const enrollmentBusy = ref(false)
  const enrollmentFailure = ref('')
  const copyNotice = ref('')
  const registrationRequired = computed(() => node.value !== undefined && requiresAgentRegistration(node.value.agentAssociationStatus))
  const registrationCode = computed(() => node.value && enrollment.value ? agentRegistrationCode(node.value.id, enrollment.value.enrollmentId, enrollment.value.enrollmentMaterial) : '')
  let version = 0
  let copyVersion = 0
  session.onReset(clearEnrollment)
  watch(registrationRequired, required => { if (!required) clearEnrollment() }, { flush: 'sync' })

  async function issueEnrollment() {
    const current = node.value
    if (!session.active() || !current || !registrationRequired.value || enrollmentBusy.value) return
    const active = session.capture()
    const requested = ++version
    copyVersion++
    enrollmentDialogOpen.value = true
    enrollment.value = undefined
    enrollmentFailure.value = copyNotice.value = ''
    enrollmentBusy.value = true
    const valid = () => active() && requested === version && enrollmentDialogOpen.value
    try {
      const result = await session.api.issueExecutionNodeEnrollment(current.id)
      if (valid()) enrollment.value = result
    } catch (error) {
      if (valid()) enrollmentFailure.value = executionNodeErrorMessage(error, '关联材料签发失败。')
    } finally {
      if (valid()) enrollmentBusy.value = false
    }
  }
  function clearEnrollment() {
    version++
    copyVersion++
    enrollment.value = undefined
    enrollmentFailure.value = copyNotice.value = ''
    enrollmentBusy.value = enrollmentDialogOpen.value = false
  }
  async function copyEnrollmentValue(label: string, value: string) {
    if (!session.active() || !enrollmentDialogOpen.value || !enrollment.value || !value) return
    const active = session.capture()
    const requested = ++copyVersion
    const dialogVersion = version
    copyNotice.value = ''
    const valid = () => active() && requested === copyVersion && dialogVersion === version && enrollmentDialogOpen.value
    try {
      await copy(value)
      if (valid()) copyNotice.value = `${label}已复制。`
    } catch {
      if (valid()) copyNotice.value = `${label}复制失败，请手动选择复制。`
    }
  }
  return { enrollmentDialogOpen, enrollment, enrollmentBusy, enrollmentFailure, copyNotice, registrationRequired, registrationCode, issueEnrollment, clearEnrollment, copyEnrollmentValue }
}
