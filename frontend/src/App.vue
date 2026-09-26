<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, setWorkspaceRequiredHandler, type ApiError } from './api'
import type { Settings } from './types'
import GuideView from './views/GuideView.vue'
import { toasts } from './useToast'

const ready = ref(false)
const settings = ref<Settings | null>(null)
const bootError = ref('')

async function bootstrap() {
  try {
    const s = await api.getSettings()
    settings.value = s
    ready.value = true
  } catch (e) {
    bootError.value = (e as ApiError).message
    ready.value = true
  }
}

setWorkspaceRequiredHandler(() => {
  if (settings.value) settings.value = { ...settings.value, needs_select: true, workspace: '' }
})

onMounted(bootstrap)

function onSelected(s: Settings) {
  settings.value = s
}
</script>

<template>
  <div v-if="!ready" class="guide"><div class="guide-card">正在启动…</div></div>
  <GuideView v-else-if="settings?.needs_select || bootError" :boot-error="bootError" @selected="onSelected" />
  <router-view v-else />
  <Transition name="toast">
    <div v-for="t in toasts" :key="t.text" class="toast" :class="{ error: t.error }">{{ t.text }}</div>
  </Transition>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity 0.2s, transform 0.2s; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateX(-50%) translateY(8px); }
</style>
