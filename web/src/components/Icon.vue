<script setup>
defineProps({ name: { type: String, required: true } })
const paths = {
  overview: 'M3 13h4v8H3zM10 8h4v13h-4zM17 3h4v18h-4z',
  devices: 'M7 2h10a2 2 0 0 1 2 2v16a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zM11 18h2',
  events: 'M12 3l9 16H3L12 3zM12 10v4M12 17h.01',
  apps: 'M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z',
  account: 'M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0',
  logout: 'M15 4h4v16h-4M10 8l-4 4 4 4M6 12h10',
  refresh: 'M20 11a8 8 0 1 0-2.3 5.7M20 4v7h-7',
  docs: 'M6 3h9l4 4v14H6zM14 3v5h5M9 13h7M9 17h7',
  back: 'M15 18l-6-6 6-6',
  download: 'M12 4v11M7 10l5 5 5-5M5 20h14',
}
</script>
<template>
  <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path :d="paths[name]" /></svg>
</template>
