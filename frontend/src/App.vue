<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, setUnauthorizedHandler, setWorkspaceRequiredHandler, type ApiError } from './api'
import type { Settings } from './types'
import GuideView from './views/GuideView.vue'
import LoginView from './components/LoginView.vue'
import { toasts } from './useToast'

const ready = ref(false)
const settings = ref<Settings | null>(null)
const bootError = ref('')
const needLogin = ref(false)

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
setUnauthorizedHandler(() => { needLogin.value = true })

onMounted(bootstrap)

function onLoggedIn() {
  needLogin.value = false
  bootError.value = ''
  bootstrap()
}

function onSelected(s: Settings) {
  settings.value = s
}
</script>

<template>
  <LoginView v-if="needLogin" @done="onLoggedIn" />
  <div v-else-if="!ready" class="guide"><div class="guide-card">正在启动…</div></div>
  <GuideView v-else-if="settings?.needs_select || bootError" :boot-error="bootError" @selected="onSelected" />
  <router-view v-else />
  <Transition name="toast">
    <div v-if="toasts.length" :key="toasts[0].id" class="toast" :class="{ error: toasts[0].error }">{{ toasts[0].text }}</div>
  </Transition>
</template>

<style scoped>
.toast-enter-active, .toast-leave-active { transition: opacity 0.15s ease, transform 0.35s cubic-bezier(0.32, 0.72, 0, 1); }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateX(-50%) translateY(12px); }
</style>
