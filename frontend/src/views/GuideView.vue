<script setup lang="ts">
import { ref } from 'vue'
import { api, type ApiError } from '../api'
import type { Settings } from '../types'
import { isDark, toggleTheme } from '../theme'

defineProps<{ bootError: string }>()
const emit = defineEmits<{ selected: [Settings] }>()

const dark = ref(isDark())
function onToggle() { toggleTheme(); dark.value = isDark() }

const path = ref('')
const busy = ref(false)
const picking = ref(false)
const error = ref('')

async function browse() {
  if (picking.value) return
  picking.value = true
  error.value = ''
  try {
    const r = await api.pickFolder()
    if (r.path) path.value = r.path
  } catch (e) {
    error.value = (e as ApiError).message
  } finally {
    picking.value = false
  }
}

async function choose() {
  if (!path.value.trim() || busy.value) return
  busy.value = true
  error.value = ''
  try {
    emit('selected', await api.putSettings(path.value.trim()))
  } catch (e) {
    error.value = (e as ApiError).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="guide">
    <button class="btn btn-icon theme-corner" :title="dark ? '切换浅色' : '切换暗色'" @click="onToggle">{{ dark ? '☾' : '☀' }}</button>
    <div class="guide-card">
      <h2>选择工作目录</h2>
      <p>工作台的所有项目、记录和文件都保存在一个目录里（类似 Obsidian 的仓库）。<br />输入一个已存在或新建的目录绝对路径，例如 <code>D:\我的工作台</code>。</p>
      <p v-if="bootError" class="err">{{ bootError }}</p>
      <div style="display:flex;gap:8px;margin-top:16px">
        <input class="input" v-model="path" placeholder="D:\我的工作台" @keydown.enter="choose" />
        <button class="btn" :disabled="picking" @click="browse">{{ picking ? '等待选择…' : '浏览…' }}</button>
        <button class="btn btn-primary" :disabled="busy || !path.trim()" @click="choose">{{ busy ? '初始化…' : '开始使用' }}</button>
      </div>
      <p v-if="error" class="err" style="margin-top:10px">{{ error }}</p>
      <p class="hint">切换目录会把旧目录的记录与文件复制到新目录（同名不覆盖）；换电脑时拷贝整个工作目录即可还原。</p>
    </div>
  </div>
</template>

<style scoped>
.theme-corner { position: absolute; top: 20px; right: 20px; }
.err { color: var(--ui-danger); font-size: 13px; margin: 8px 0 0; }
.hint { color: var(--ui-muted); font-size: 12px; margin: 14px 0 0; }
code { background: var(--ui-default); padding: 1px 5px; border-radius: 4px; }
</style>
