<script setup lang="ts">
import { computed, defineAsyncComponent, ref, shallowRef, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api, formatSize, isPreviewable } from '../api'
import type { Asset, Project, TabItem } from '../types'
import Modal from '../components/Modal.vue'
import Tabs from '../components/Tabs.vue'
import FileIcon from '../components/FileIcon.vue'
import FilePreview from '../components/FilePreview.vue'
import { errToast, toast } from '../useToast'
import { bumpTree, treeVersion } from '../bus'

// markdown-it/dompurify 只被记录浮层用到：懒加载，避免押进首屏 chunk
const RecordOverlay = defineAsyncComponent(() => import('../components/RecordOverlay.vue'))

const route = useRoute()

const project = ref<Project | null>(null)
const pending = ref(false)
const tab = ref<'record' | 'file'>('record')
const tabs: TabItem<'record' | 'file'>[] = [{ value: 'record', label: '记录库' }, { value: 'file', label: '文件库' }]
const assets = shallowRef<Asset[]>([])
const loading = ref(false)
const uploadInput = ref<HTMLInputElement | null>(null)
const showBlank = ref(false)
const blankTitle = ref('')
const overlayAsset = ref<Asset | null>(null)
const previewAsset = ref<Asset | null>(null)
const overlayEdit = ref(false)

const selected = ref(new Set<number>())
const showBatchDelete = ref(false)
const batching = ref(false)

const page = ref(1)
const pageSize = ref(Number(localStorage.getItem('wb-pagesize')) || 20)
const totalPages = computed(() => Math.max(1, Math.ceil(assets.value.length / pageSize.value)))
const pagedAssets = computed(() => assets.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
watch(totalPages, n => { if (page.value > n) page.value = n })
watch(tab, () => { page.value = 1 })

function onPageSize(e: Event) {
  pageSize.value = Number((e.target as HTMLSelectElement).value) || 20
  localStorage.setItem('wb-pagesize', String(pageSize.value))
  page.value = 1
}

function jumpPage(e: Event) {
  const el = e.target as HTMLInputElement
  const n = parseInt(el.value, 10)
  if (isNaN(n)) { el.value = String(page.value); return }
  page.value = Math.min(Math.max(1, n), totalPages.value)
  el.value = String(page.value)
}
function syncJump(e: Event) {
  (e.target as HTMLInputElement).value = String(page.value)
}

async function load() {
  loading.value = true
  try {
    const id = route.params.id as string
    const [pr, list] = await Promise.all([api.getProject(id), api.listAssets(id, tab.value)])
    project.value = pr.project
    pending.value = pr.pending
    assets.value = list
  } catch (e) {
    errToast(e)
  } finally {
    loading.value = false
  }
}

async function reloadAssets() {
  try {
    assets.value = await api.listAssets(route.params.id as string, tab.value)
  } catch (e) {
    errToast(e)
  }
}

watch(() => route.params.id, load, { immediate: true })
let skipNextReload = false
watch([tab, treeVersion], () => {
  if (route.name !== 'project') return
  selected.value = new Set()
  if (skipNextReload) { skipNextReload = false; return }
  reloadAssets()
})

const projectId = computed(() => project.value?.id ?? String(route.params.id))

const selectedIds = computed(() => [...selected.value])
const pageAllSelected = computed(() =>
  pagedAssets.value.length > 0 && pagedAssets.value.every(a => selected.value.has(a.id)))
const pageSomeSelected = computed(() =>
  !pageAllSelected.value && pagedAssets.value.some(a => selected.value.has(a.id)))

function toggleAsset(id: number) {
  const s = new Set(selected.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  selected.value = s
}

function togglePage() {
  const s = new Set(selected.value)
  if (pageAllSelected.value) pagedAssets.value.forEach(a => s.delete(a.id))
  else pagedAssets.value.forEach(a => s.add(a.id))
  selected.value = s
}

function batchDownload() {
  window.open(api.batchDownloadUrl(selectedIds.value), '_blank')
}

async function doBatchDelete() {
  if (batching.value) return
  batching.value = true
  try {
    const r = await api.batchDelete(selectedIds.value)
    showBatchDelete.value = false
    selected.value = new Set()
    toast(`已删除 ${r.deleted} 项`)
    bumpTree() // treeVersion 变化会触发列表重载
  } catch (e) {
    errToast(e)
  } finally {
    batching.value = false
  }
}

async function doUpload(files: FileList | null) {
  if (!files || !files.length) return
  try {
    const list = Array.from(files)
    const uploaded = await api.uploadFiles(projectId.value, tab.value, list)
    toast(`已上传 ${uploaded.length} 个文件`)
    assets.value = [...uploaded, ...assets.value] // 新资产在列表最前，免整表重载
    if (pending.value) { skipNextReload = true; bumpTree() } // 空项目首次有内容：刷新项目列表
    if (tab.value === 'record') {
      const firstMd = uploaded.find(a => a.ext === 'md')
      if (firstMd) { openOverlay(firstMd, false); return }
    }
  } catch (e) {
    errToast(e)
  }
}

async function createBlank() {
  const title = blankTitle.value.trim()
  if (!title) return
  try {
    const a = await api.createBlank(projectId.value, title)
    showBlank.value = false
    blankTitle.value = ''
    if (tab.value !== 'record') tab.value = 'record' // 切 tab 触发重载，拿到含新记录的列表
    else assets.value = [a, ...assets.value]
    if (pending.value) { skipNextReload = true; bumpTree() } // 本地已前插，树刷新时跳过重复重载
    overlayEdit.value = true
    overlayAsset.value = a
  } catch (e) {
    errToast(e)
  }
}

function openOverlay(a: Asset, edit: boolean) {
  overlayEdit.value = edit
  overlayAsset.value = a
}

function onOverlaySaved() {
  load()
}
</script>

<template>
  <div v-if="project">
    <div class="head">
      <h2 style="margin:0">{{ project.name }}<span v-if="pending" class="badge" style="margin-left:8px">尚无内容</span></h2>
      <span style="flex:1"></span>
      <button class="btn" @click="uploadInput?.click()">上传文件</button>
      <button class="btn btn-primary" @click="showBlank = true">新建记录</button>
      <input ref="uploadInput" type="file" multiple hidden @change="doUpload($event.target.files); ($event.target as HTMLInputElement).value = ''" />
    </div>

    <Tabs v-model="tab" :items="tabs" style="margin:12px 0" />

    <div v-if="loading" class="empty">加载中…</div>
    <div v-else-if="!assets.length" class="empty">{{ tab === 'record' ? '还没有记录，点右上「新建记录」或上传 .md' : '还没有文件，点右上「上传文件」' }}</div>
    <div v-else class="table-card table-wrap">
      <table class="table">
        <thead>
          <tr>
            <th class="col-check"><input type="checkbox" class="ck" :checked="pageAllSelected" :indeterminate="pageSomeSelected" @change="togglePage" /></th>
            <th>名称</th><th class="col-size">大小</th><th class="col-time">时间</th><th class="col-acts"></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in pagedAssets" :key="a.id" :class="{ picked: selected.has(a.id) }">
            <td><input type="checkbox" class="ck" :checked="selected.has(a.id)" @change="toggleAsset(a.id)" /></td>
            <td>
              <FileIcon :ext="a.ext" :category="a.category" />
              <a v-if="a.category === 'file' && isPreviewable(a.ext)" class="pname" title="点击预览" @click="previewAsset = a">{{ a.original_name }}</a>
              <a v-else-if="a.category === 'record'" class="pname" title="点击查看" @click="openOverlay(a, false)">{{ a.original_name }}</a>
              <template v-else>{{ a.original_name }}</template>
            </td>
            <td>{{ formatSize(a.size) }}</td>
            <td>{{ a.uploaded_at.slice(0, 16).replace('T', ' ') }}</td>
            <td class="col-acts">
              <template v-if="a.category === 'record'">
                <button class="btn btn-sm" @click="openOverlay(a, true)">编辑</button>
                <a class="btn btn-sm" :href="api.downloadUrl(a.id)" style="margin-left:6px">下载</a>
              </template>
              <a v-else class="btn btn-sm" :href="api.downloadUrl(a.id)">下载</a>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="selected.size" class="batch-bar">
      <span>已选 {{ selected.size }} 项</span>
      <button class="btn btn-sm" :disabled="batching" @click="batchDownload">批量下载</button>
      <button class="btn btn-sm btn-danger" :disabled="batching" @click="showBatchDelete = true">批量删除</button>
      <button class="btn btn-sm" :disabled="batching" @click="selected = new Set()">取消</button>
    </div>

    <div v-if="assets.length && !loading" class="pager">
      <label>每页
        <select class="input sel" :value="pageSize" @change="onPageSize">
          <option v-for="n in [10, 20, 50, 100]" :key="n" :value="n">{{ n }}</option>
        </select>
        条
      </label>
      <span class="total">共 {{ assets.length }} 条</span>
      <span style="flex:1"></span>
      <button class="btn btn-sm" :disabled="page <= 1" @click="page--">‹ 上一页</button>
      <span>第 <input class="input jump" type="text" inputmode="numeric" autocomplete="off" :value="page" @keydown.enter="jumpPage" @blur="syncJump" /> / {{ totalPages }} 页</span>
      <button class="btn btn-sm" :disabled="page >= totalPages" @click="page++">下一页 ›</button>
    </div>
  </div>

  <Modal v-if="showBlank" title="新建记录" @close="showBlank = false">
    <input class="input" v-model="blankTitle" placeholder="记录名称（不必带 .md）" @keydown.enter="createBlank" />
    <template #foot>
      <button class="btn" @click="showBlank = false">取消</button>
      <button class="btn btn-primary" :disabled="!blankTitle.trim()" @click="createBlank">创建</button>
    </template>
  </Modal>

  <Modal v-if="showBatchDelete" title="批量删除" @close="showBatchDelete = false">
    <p style="margin:0">确定删除选中的 {{ selected.size }} 项？文件将从磁盘一并删除，无法恢复。</p>
    <template #foot>
      <button class="btn" @click="showBatchDelete = false">取消</button>
      <button class="btn btn-danger" :disabled="batching" @click="doBatchDelete">删除</button>
    </template>
  </Modal>

  <RecordOverlay v-if="overlayAsset" :key="overlayAsset.id" :asset="overlayAsset" :project-id="projectId" :start-editing="overlayEdit"
    @close="overlayAsset = null" @saved="onOverlaySaved" />

  <FilePreview v-if="previewAsset" :key="previewAsset.id" :asset="previewAsset" @close="previewAsset = null" />
</template>

<style scoped>
.head { display: flex; align-items: center; gap: 10px; }
.pager { display: flex; flex-wrap: wrap; align-items: center; column-gap: 10px; row-gap: 8px; margin-top: 12px; font-size: 13px; color: var(--ui-content2); white-space: nowrap; }
.pager .sel { width: auto; height: 32px; padding: 0 8px; display: inline-block; }
.pager .jump { width: 56px; height: 28px; padding: 0 8px; display: inline-block; text-align: center; }
.pname { cursor: pointer; color: var(--ui-focus); }
.pname:hover { text-decoration: underline; }
.pager label { display: inline-flex; align-items: center; gap: 6px; }
/* HeroUI checkbox__control：16px、圆角 6、bg-field + shadow-field，选中铺满 accent */
.ck {
  -webkit-appearance: none;
  appearance: none;
  position: relative;
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  margin: 0;
  vertical-align: middle;
  border-radius: 6px;
  background: var(--ui-field-bg);
  box-shadow: var(--ui-field-shadow);
  cursor: pointer;
  transition: background-color 0.15s ease, box-shadow 0.15s cubic-bezier(0, 0, 0.2, 1);
}
.ck:checked, .ck:indeterminate { background: var(--ui-focus); }
@media (hover: hover) {
  .ck:checked:hover, .ck:indeterminate:hover { background: var(--ui-accent-hover); }
}
.ck:focus-visible { box-shadow: 0 0 0 2px var(--ui-page), 0 0 0 4px var(--ui-focus); }
.ck:checked::after {
  content: '';
  position: absolute;
  inset: 0;
  background: center / 12px no-repeat url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%23ffffff' stroke-width='3.5' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M5 13l4 4 10-10'/%3E%3C/svg%3E");
}
.ck:indeterminate::after {
  content: '';
  position: absolute;
  left: 3px;
  right: 3px;
  top: 7px;
  height: 2px;
  border-radius: 1px;
  background: #fff;
}
/* 选中行 = accent-soft（HeroUI table 行选中同款） */
tr.picked td { background: color-mix(in oklab, var(--ui-focus) 15%, var(--ui-surface)); }
.batch-bar { display: flex; align-items: center; gap: 10px; margin-top: 10px; padding: 8px 16px; font-size: 13px;
  background: var(--ui-surface); border-radius: var(--radius-large); box-shadow: var(--ui-surface-shadow); }
</style>
