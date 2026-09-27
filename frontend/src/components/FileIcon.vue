<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ ext?: string; category?: 'record' | 'file'; size?: number }>()

const DOC = '<path d="M6.5 2.5h7L17.5 7v12.5a2 2 0 0 1-2 2h-9a2 2 0 0 1-2-2v-13a2 2 0 0 1 2-2Z"/><path d="M13.5 2.5V7h4"/>'

const SHAPES: Record<string, string> = {
  file: DOC,
  text: DOC + '<path d="M8 12.5h8M8 16.5h5"/>',
  image: '<rect x="3.5" y="5" width="17" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="m4.5 17 5-5 3 3 3-3 4 4"/>',
  grid: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M3.5 9.5h17M3.5 14.5h17M12 4.5v15"/>',
  board: '<rect x="3.5" y="4" width="17" height="11.5" rx="1.5"/><path d="M12 15.5v2M8.5 21l3.5-3.5 3.5 3.5"/>',
  archive: '<path d="M3.5 4.5h17v4h-17z"/><path d="M5.5 8.5v10.5a1.5 1.5 0 0 0 1.5 1.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5"/><path d="M10 12.5h4"/>',
}

const STYLE: Record<string, { shape: string; color: string }> = {
  md: { shape: 'text', color: '#8b5cf6' },
  pdf: { shape: 'text', color: '#dc2626' },
  doc: { shape: 'text', color: '#2563eb' },
  xls: { shape: 'grid', color: '#059669' },
  ppt: { shape: 'board', color: '#ea580c' },
  img: { shape: 'image', color: '#10b981' },
  zip: { shape: 'archive', color: '#ca8a04' },
}

const info = computed(() => {
  const e = (props.ext || '').toLowerCase().replace(/^\./, '')
  if (props.category === 'record' || e === 'md') return STYLE.md
  if (['pdf'].includes(e)) return STYLE.pdf
  if (['doc', 'docx', 'rtf'].includes(e)) return STYLE.doc
  if (['xls', 'xlsx', 'csv'].includes(e)) return STYLE.xls
  if (['ppt', 'pptx', 'key'].includes(e)) return STYLE.ppt
  if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg'].includes(e)) return STYLE.img
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(e)) return STYLE.zip
  return { shape: 'file', color: 'currentColor' }
})
</script>

<template>
  <svg class="fi" :width="size || 16" :height="size || 16" viewBox="0 0 24 24" fill="none"
    :stroke="info.color" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"
    v-html="SHAPES[info.shape]" />
</template>

<style scoped>
.fi { vertical-align: -3px; margin-right: 6px; flex-shrink: 0; }
</style>
