<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, ref, shallowRef } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, formatSize, isPreviewable } from '../api'
import type { Asset, SearchHit } from '../types'
import FileIcon from './FileIcon.vue'
import FilePreview from './FilePreview.vue'
import { errToast } from '../useToast'

const RecordOverlay = defineAsyncComponent(() => import('./RecordOverlay.vue'))

const route = useRoute()
const router = useRouter()

const q = ref('')
const scope = ref<'all' | 'current'>('all')
const hits = shallowRef<SearchHit[]>([])
const open = ref(false)
const searching = ref(false)
const overlayAsset = ref<Asset | null>(null)
const previewAsset = ref<Asset | null>(null)
const wrap = ref<HTMLElement | null>(null)
let timer: number | undefined
let seq = 0 // 慢响应晚到时丢弃，避免旧结果覆盖新结果

const currentId = computed(() => (route.name === 'project' ? String(route.params.id) : ''))
const scopeLabel = computed(() => {
  if (scope.value === 'all') return '全部项目'
  return currentId.value ? '当前项目' : '全部项目'
})

function doSearch() {
  const kw = q.value.trim()
  if (!kw) { hits.value = []; open.value = false; return }
  searching.value = true
  const s = ++seq
  const pid = scope.value === 'current' ? currentId.value : ''
  api.search(kw, pid).then(r => { if (s === seq) hits.value = r }).catch(e => {
    if (s === seq) errToast(e)
  }).finally(() => { if (s === seq) { searching.value = false; open.value = true } })
}

function onInput() {
  window.clearTimeout(timer)
  timer = window.setTimeout(doSearch, 250)
}

function onScope() {
  if (scope.value === 'current' && !currentId.value) scope.value = 'all'
  if (q.value.trim()) doSearch()
}

function toAsset(h: SearchHit): Asset {
  return {
    id: h.id, project_id: h.project_id, category: h.category,
    original_name: h.original_name, stored_name: '', stored_path: '',
    ext: h.ext, size: h.size, uploaded_at: h.uploaded_at,
  }
}

function onResult(h: SearchHit) {
  open.value = false
  if (h.category === 'file') {
    if (isPreviewable(h.ext)) previewAsset.value = toAsset(h)
    else window.open(api.downloadUrl(h.id), '_blank')
    return
  }
  overlayAsset.value = toAsset(h)
}

function goProject(h: SearchHit) {
  open.value = false
  router.push({ name: 'project', params: { id: h.project_id } })
}

function onDocDown(e: MouseEvent) {
  if (wrap.value && !wrap.value.contains(e.target as Node)) open.value = false
}
document.addEventListener('mousedown', onDocDown)
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocDown)
  window.clearTimeout(timer)
})
</script>

<template>
  <div ref="wrap" class="sb">
    <select class="input sb-scope" v-model="scope" @change="onScope">
      <option value="all">全部项目</option>
      <option value="current" :disabled="!currentId">当前项目</option>
    </select>
    <div class="sb-field">
      <svg class="sb-ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <circle cx="11" cy="11" r="7" /><path d="M20 20l-3.6-3.6" />
      </svg>
      <input class="input sb-q" v-model="q" placeholder="搜记录名 / 文件名"
        @input="onInput" @keydown.enter.prevent="doSearch" @focus="q.trim() && (open = true)" />
    </div>
    <div v-if="open" class="sb-panel">
      <div v-if="!hits.length" class="sb-empty">{{ searching ? '搜索中…' : '没有匹配的文件名' }}</div>
      <div v-for="h in hits" :key="h.id" class="sb-row" @click="onResult(h)">
        <FileIcon :ext="h.ext" :category="h.category" />
        <span class="sb-name">{{ h.original_name }}</span>
        <span class="sb-proj" @click.stop="goProject(h)">{{ h.project_name }}</span>
        <span class="sb-meta">{{ h.category === 'record' ? '记录' : '文件' }} · {{ formatSize(h.size) }}</span>
      </div>
    </div>
  </div>

  <RecordOverlay v-if="overlayAsset" :key="overlayAsset.id" :asset="overlayAsset" :project-id="overlayAsset.project_id"
    :start-editing="false" @close="overlayAsset = null" @saved="() => {}" />

  <FilePreview v-if="previewAsset" :key="previewAsset.id" :asset="previewAsset" @close="previewAsset = null" />
</template>

<style scoped>
.sb { position: relative; display: flex; align-items: center; gap: 6px; }
.sb-scope { width: auto; }
.sb-field { position: relative; display: flex; align-items: center; }
.sb-ico { position: absolute; left: 10px; top: 50%; transform: translateY(-50%); width: 14px; height: 14px; color: var(--ui-content3); pointer-events: none; }
.sb-q { width: 220px; padding-left: 30px; }
.sb-panel {
  position: absolute; top: 40px; right: 0; width: 380px; max-height: 380px; overflow: auto; z-index: 60;
  background: var(--ui-surface); border-radius: var(--radius-large);
  box-shadow: var(--ui-overlay-shadow); padding: 6px;
}
.sb-row { display: flex; align-items: center; gap: 8px; min-height: 36px; padding: 6px 10px; border-radius: 16px; cursor: pointer; font-size: 13px; }
@media (hover: hover) {
  .sb-row:hover { background: var(--ui-default); }
}
.sb-name { flex: 0 1 auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-proj { margin-left: auto; flex: none; font-size: 12px; color: var(--ui-focus); cursor: pointer; }
.sb-proj:hover { text-decoration: underline; }
.sb-meta { flex: none; font-size: 11px; color: var(--ui-content3); }
.sb-empty { padding: 12px; text-align: center; font-size: 12px; color: var(--ui-muted); }
</style>
