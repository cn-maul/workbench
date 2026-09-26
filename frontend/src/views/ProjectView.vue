<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api, formatSize, type ApiError } from '../api'
import type { Asset, Project } from '../types'
import Modal from '../components/Modal.vue'
import RecordOverlay from '../components/RecordOverlay.vue'
import { toast } from '../useToast'
import { bumpTree } from '../bus'

const route = useRoute()

const project = ref<Project | null>(null)
const pending = ref(false)
const tab = ref<'record' | 'file'>('record')
const assets = ref<Asset[]>([])
const loading = ref(false)
const uploadInput = ref<HTMLInputElement | null>(null)
const showBlank = ref(false)
const blankTitle = ref('')
const overlayAsset = ref<Asset | null>(null)
const overlayEdit = ref(false)

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
    const pr = await api.getProject(id)
    project.value = pr.project
    pending.value = pr.pending
    assets.value = await api.listAssets(pr.project.id, tab.value)
  } catch (e) {
    toast((e as ApiError).message, true)
  } finally {
    loading.value = false
  }
}

watch(() => [route.params.id, tab.value], load, { immediate: true })

const projectId = computed(() => String(project.value?.id ?? route.params.id))

async function doUpload(files: FileList | null) {
  if (!files || !files.length) return
  try {
    const list = Array.from(files)
    const uploaded = await api.uploadFiles(projectId.value, tab.value, list)
    toast(`已上传 ${uploaded.length} 个文件`)
    if (tab.value === 'record' && uploaded.length) {
      const firstMd = uploaded.find(a => a.ext === 'md')
      if (firstMd) { await load(); openOverlay(firstMd, false); return }
    }
    await load()
  } catch (e) {
    toast((e as ApiError).message, true)
  }
}

async function createBlank() {
  const title = blankTitle.value.trim()
  if (!title) return
  try {
    const a = await api.createBlank(projectId.value, title)
    showBlank.value = false
    blankTitle.value = ''
    if (tab.value !== 'record') tab.value = 'record'
    else await load()
    bumpTree()
    overlayEdit.value = true
    overlayAsset.value = a
  } catch (e) {
    toast((e as ApiError).message, true)
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

    <div class="tabs" style="margin:12px 0">
      <button class="tab" :class="{ active: tab === 'record' }" @click="tab = 'record'">记录库</button>
      <button class="tab" :class="{ active: tab === 'file' }" @click="tab = 'file'">文件库</button>
    </div>

    <div v-if="loading" class="empty">加载中…</div>
    <div v-else-if="!assets.length" class="empty">{{ tab === 'record' ? '还没有记录，点右上「新建记录」或上传 .md' : '还没有文件，点右上「上传文件」' }}</div>
    <div v-else class="card table-wrap">
      <table class="table">
        <thead><tr><th>名称</th><th>大小</th><th>时间</th><th style="width:160px"></th></tr></thead>
        <tbody>
          <tr v-for="a in pagedAssets" :key="a.id">
            <td>{{ a.original_name }}</td>
            <td>{{ formatSize(a.size) }}</td>
            <td>{{ a.uploaded_at.slice(0, 16).replace('T', ' ') }}</td>
            <td style="text-align:right">
              <template v-if="a.category === 'record'">
                <button class="btn btn-sm" @click="openOverlay(a, false)">查看</button>
                <button class="btn btn-sm" @click="openOverlay(a, true)">编辑</button>
                <a class="btn btn-sm" :href="api.downloadUrl(a.id)" style="margin-left:6px">下载</a>
              </template>
              <a v-else class="btn btn-sm" :href="api.downloadUrl(a.id)">下载</a>
            </td>
          </tr>
        </tbody>
      </table>
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

  <RecordOverlay v-if="overlayAsset" :key="overlayAsset.id" :asset="overlayAsset" :project-id="projectId" :start-editing="overlayEdit"
    @close="overlayAsset = null" @saved="onOverlaySaved" />
</template>

<style scoped>
.head { display: flex; align-items: center; gap: 10px; }
.pager { display: flex; align-items: center; gap: 10px; margin-top: 12px; font-size: 13px; color: var(--ui-content2); }
.pager .sel { width: auto; padding: 3px 8px; display: inline-block; }
.pager .jump { width: 56px; display: inline-block; padding: 3px 8px; text-align: center; }
.pager label { display: inline-flex; align-items: center; gap: 6px; }
</style>
