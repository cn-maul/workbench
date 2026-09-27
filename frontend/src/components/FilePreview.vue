<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { api } from '../api'
import type { Asset } from '../types'
import FileIcon from './FileIcon.vue'

const props = defineProps<{ asset: Asset }>()
const emit = defineEmits<{ close: [] }>()

const isPdf = computed(() => props.asset.ext.toLowerCase() === 'pdf')
const src = computed(() => api.downloadUrl(props.asset.id, true))

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}
onMounted(() => window.addEventListener('keydown', onKey))
onUnmounted(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <div class="fp-mask" @click.self="emit('close')">
    <div class="fp">
      <header class="fp-head">
        <FileIcon :ext="asset.ext" category="file" :size="18" />
        <strong>{{ asset.original_name }}</strong>
        <span style="flex:1"></span>
        <a class="btn btn-sm" :href="api.downloadUrl(asset.id)">下载</a>
        <button class="btn btn-sm" @click="emit('close')">关闭</button>
      </header>
      <div class="fp-body">
        <iframe v-if="isPdf" :src="src" class="fp-frame" :title="asset.original_name" />
        <img v-else :src="src" class="fp-img" :alt="asset.original_name" @error="emit('close')" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.fp-mask {
  position: fixed;
  inset: 0;
  background: var(--ui-overlay);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  animation: fade-in 0.15s cubic-bezier(0, 0, 0.2, 1);
  outline: none;
}
.fp {
  background: var(--ui-surface);
  border-radius: var(--radius-large);
  box-shadow: var(--ui-overlay-shadow);
  width: min(1100px, calc(100vw - 32px));
  height: min(760px, 88vh);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  animation: pop-in 0.25s cubic-bezier(0.25, 0.46, 0.45, 0.94);
}
.fp-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 18px;
  border-bottom: 1px solid var(--ui-divider);
  flex-shrink: 0;
}
.fp-body { flex: 1; min-height: 0; background: var(--ui-content1); }
.fp-frame { width: 100%; height: 100%; border: none; display: block; }
.fp-img { width: 100%; height: 100%; object-fit: contain; display: block; }
</style>
