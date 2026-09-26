<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type ApiError } from '../api'
import type { Settings } from '../types'
import { toast } from '../useToast'

const settings = ref<Settings | null>(null)
const path = ref('')
const busy = ref(false)

onMounted(async () => {
  try {
    settings.value = await api.getSettings()
    path.value = settings.value.workspace
  } catch (e) {
    toast((e as ApiError).message, true)
  }
})

async function switchWs() {
  if (!path.value.trim() || busy.value) return
  busy.value = true
  try {
    settings.value = await api.putSettings(path.value.trim())
    toast('已切换工作目录')
  } catch (e) {
    toast((e as ApiError).message, true)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div style="max-width:560px">
    <h2 style="margin-top:0">设置</h2>
    <div class="field">
      <div class="field-label">当前工作目录</div>
      <div>{{ settings?.workspace || '（未设置）' }}
        <span v-if="settings && !settings.workspace_exists" class="badge badge-danger">目录不存在</span>
      </div>
    </div>
    <div class="field">
      <div class="field-label">切换工作目录</div>
      <div style="display:flex;gap:8px">
        <input class="input" v-model="path" placeholder="D:\其他工作台" />
        <button class="btn btn-primary" :disabled="busy" @click="switchWs">{{ busy ? '切换中…' : '切换' }}</button>
      </div>
      <p class="hint">切换只改变指向，不会迁移或合并旧目录的数据。</p>
    </div>
  </div>
</template>

<style scoped>
.field { margin-bottom: 20px; }
.field-label { font-size: 12px; color: var(--ui-content3); margin-bottom: 6px; }
.hint { color: var(--ui-content3); font-size: 12px; margin: 6px 0 0; }
</style>
