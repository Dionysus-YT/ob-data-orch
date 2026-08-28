<script setup lang="ts">
withDefaults(
  defineProps<{
    name: string
    size?: number
  }>(),
  { size: 16 },
)
</script>

<template>
  <svg class="icon" :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor" aria-hidden="true">
    <path v-if="name === 'home'" d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" />
    <g v-else-if="name === 'database'"><ellipse cx="12" cy="5" rx="7" ry="3" /><path d="M5 5v7c0 1.7 3.1 3 7 3s7-1.3 7-3V5M5 12v7c0 1.7 3.1 3 7 3s7-1.3 7-3v-7" /></g>
    <g v-else-if="name === 'upload'"><path d="M12 16V3m0 0L7 8m5-5 5 5M4 15v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5" /></g>
    <g v-else-if="name === 'download'"><path d="M12 3v13m0 0 5-5m-5 5-5-5M4 15v5a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-5" /></g>
    <path v-else-if="name === 'bolt'" d="m13 2-8 12h7l-1 8 8-12h-7z" />
    <g v-else-if="name === 'tasks'"><path d="M9 6h11M9 12h11M9 18h11" /><path d="m3 6 1 1 2-2m-3 7 1 1 2-2m-3 7 1 1 2-2" /></g>
    <g v-else-if="name === 'server'"><rect x="3" y="3" width="18" height="7" rx="1" /><rect x="3" y="14" width="18" height="7" rx="1" /><path d="M7 6.5h.01M7 17.5h.01" /></g>
    <g v-else-if="name === 'log'"><path d="M5 3h11l3 3v15H5z" /><path d="M16 3v4h4M8 12h8M8 16h8M8 8h3" /></g>
    <g v-else-if="name === 'settings'"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.2 2.2-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-3.2v-.2a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L6.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H4.8v-3.2H5a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9l-.1-.1 2.2-2.2.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6v-.2h3.2v.2a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1 2.2 2.2-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2V14h-.2a1.7 1.7 0 0 0-1.6 1z" /></g>
    <path v-else-if="name === 'key'" d="M15 7a5 5 0 1 0-4.8 6.4L12 15h2v2h2v2h3v-3l-5.4-5.4A5 5 0 0 0 15 7Z" />
    <g v-else-if="name === 'help'"><circle cx="12" cy="12" r="8" /><path d="M9.7 9a2.4 2.4 0 0 1 4.7.8c0 1.7-2.4 2-2.4 3.7M12 17h.01" /></g>
    <path v-else-if="name === 'refresh'" d="M20 11a8 8 0 1 0 1 4.2M20 4v7h-7" />
    <path v-else-if="name === 'plus'" d="M12 5v14M5 12h14" />
    <g v-else-if="name === 'search'"><circle cx="10.5" cy="10.5" r="5.5" /><path d="m15 15 5 5" /></g>
    <path v-else-if="name === 'chevron-down'" d="m6 9 6 6 6-6" />
    <path v-else-if="name === 'chevron-right'" d="m9 6 6 6-6 6" />
    <g v-else-if="name === 'alert'"><path d="M12 3 2.5 20h19z" /><path d="M12 9v4m0 4h.01" /></g>
    <g v-else-if="name === 'clock'"><circle cx="12" cy="12" r="8" /><path d="M12 7v5l3 2" /></g>
    <path v-else-if="name === 'more'" d="M5 12h.01M12 12h.01M19 12h.01" stroke-width="3" />
    <path v-else-if="name === 'x'" d="m6 6 12 12M18 6 6 18" />
    <path v-else-if="name === 'check'" d="m5 12 4 4L19 6" />
    <g v-else-if="name === 'copy'"><rect x="9" y="9" width="10" height="10" rx="1" /><path d="M15 9V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h4" /></g>
    <path v-else-if="name === 'arrow-left'" d="M19 12H5m6-6-6 6 6 6" />
    <path v-else d="M5 12h14" />
  </svg>
</template>
