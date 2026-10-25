import { watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

// usePageEntryPosition 在独立页面进入时定位正文，只接受当前页面内的锚点。
export function usePageEntryPosition(content: Readonly<Ref<HTMLElement | undefined>>) {
  const route = useRoute()
  watch([content, () => route.hash], ([root, hash]) => {
    if (!root) return
    let anchorId = ''
    try { anchorId = decodeURIComponent(hash.slice(1)) } catch { /* 非法片段按无锚点处理，不影响页面进入。 */ }
    const anchor = anchorId ? document.getElementById(anchorId) : null
    if (anchor && root.contains(anchor)) anchor.scrollIntoView({ block: 'start', behavior: 'instant' })
    else window.scrollTo({ top: 0, left: 0, behavior: 'instant' })
  }, { flush: 'post' })
}
