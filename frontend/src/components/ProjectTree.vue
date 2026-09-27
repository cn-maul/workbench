<script setup lang="ts">
import { ref, shallowRef, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import type { Project, ProjectGroups } from '../types'
import Modal from './Modal.vue'
import { errToast } from '../useToast'
import { treeVersion, bumpTree } from '../bus'

const route = useRoute()
const router = useRouter()

const groups = shallowRef<ProjectGroups | null>(null)
const showCreate = ref(false)
const openHistory = ref(true)
const openCustom = ref(true)
const newName = ref('')

const renameTarget = ref<Project | null>(null)
const renameInput = ref<HTMLInputElement | null>(null)
const renameName = ref('')
const deleteTarget = ref<Project | null>(null)
const deleteCount = ref(-1)
const deleting = ref(false)

watch(renameTarget, v => { if (v) nextTick(() => { renameInput.value?.focus(); renameInput.value?.select() }) })

const currentVirtualId = () => {
  const d = new Date()
  return -(d.getFullYear() * 100 + d.getMonth() + 1)
}

async function refresh() {
  try {
    groups.value = await api.listProjects()
  } catch (e) {
    errToast(e)
  }
}

// 只在数据变更信号（treeVersion）时拉取；激活态由 isActive 读 route 渲染，无需监听路由。
watch(treeVersion, refresh, { immediate: true })

function go(p: Project) {
  router.push({ name: 'project', params: { id: p.id } })
}

function isActive(p: Project): boolean {
  const cur = String(route.params.id ?? '')
  if (cur === p.id) return true
  return p.pending === true && cur === String(currentVirtualId())
}

async function create() {
  const name = newName.value.trim()
  if (!name) return
  try {
    const p = await api.createProject(name)
    showCreate.value = false
    newName.value = ''
    bumpTree()
    router.push({ name: 'project', params: { id: p.id } })
  } catch (e) {
    errToast(e)
  }
}

function startRename(p: Project) {
  renameTarget.value = p
  renameName.value = p.name
}

async function doRename() {
  const p = renameTarget.value
  const name = renameName.value.trim()
  if (!p || !name) return
  try {
    await api.renameProject(p.id, name)
    renameTarget.value = null
    bumpTree()
  } catch (e) {
    errToast(e)
  }
}

async function startDelete(p: Project) {
  deleteTarget.value = p
  deleteCount.value = -1
  try {
    deleteCount.value = (await api.listAssets(p.id)).length
  } catch { /* 数量仅用于提示 */ }
}

async function doDelete() {
  const p = deleteTarget.value
  if (!p || deleting.value) return
  deleting.value = true
  try {
    await api.deleteProject(p.id)
    deleteTarget.value = null
    bumpTree()
    if (String(route.params.id ?? '') === p.id) router.push('/')
  } catch (e) {
    errToast(e)
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <nav class="tree">
    <template v-if="groups">
      <div class="tree-group">
        <div class="tree-label">本月</div>
        <div class="tree-item" :class="{ active: isActive(groups.current_monthly) }" @click="go(groups.current_monthly)">
          {{ groups.current_monthly.name }}
          <span v-if="groups.current_monthly.pending" class="pending">（空）</span>
        </div>
      </div>
      <div v-if="groups.history_monthly.length" class="tree-group">
        <button class="tree-label clickable" :aria-expanded="openHistory" @click="openHistory = !openHistory">
          历史月份
          <span class="spacer" />
          <span class="badge">{{ groups.history_monthly.length }}</span>
          <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
            stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
        </button>
        <div v-show="openHistory" class="tree-body">
          <div v-for="p in groups.history_monthly" :key="p.id" class="tree-item" :class="{ active: isActive(p) }" @click="go(p)">{{ p.name }}</div>
        </div>
      </div>
      <div class="tree-group">
        <button class="tree-label clickable" :aria-expanded="openCustom" @click="openCustom = !openCustom">
          自定义项目
          <span class="spacer" />
          <button class="btn btn-ghost" title="新建自定义项目" @click.stop="showCreate = true">＋</button>
          <svg class="chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
            stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
        </button>
        <div v-show="openCustom" class="tree-body">
          <div v-for="p in groups.custom" :key="p.id" class="tree-item" :class="{ active: isActive(p) }" @click="go(p)">
            <span class="tname">{{ p.name }}</span>
            <span class="tacts">
              <button class="tact" title="改名" @click.stop="startRename(p)">✎</button>
              <button class="tact danger" title="删除" @click.stop="startDelete(p)">✕</button>
            </span>
          </div>
          <div v-if="!groups.custom.length" class="tree-empty">还没有项目</div>
        </div>
      </div>
    </template>
  </nav>

  <Modal v-if="showCreate" title="新建自定义项目" @close="showCreate = false">
    <input class="input" v-model="newName" placeholder="项目名称" @keydown.enter="create" />
    <template #foot>
      <button class="btn" @click="showCreate = false">取消</button>
      <button class="btn btn-primary" :disabled="!newName.trim()" @click="create">创建</button>
    </template>
  </Modal>

  <Modal v-if="renameTarget" title="项目改名" @close="renameTarget = null">
    <input class="input" ref="renameInput" v-model="renameName" placeholder="项目名称" @keydown.enter="doRename" />
    <template #foot>
      <button class="btn" @click="renameTarget = null">取消</button>
      <button class="btn btn-primary" :disabled="!renameName.trim()" @click="doRename">保存</button>
    </template>
  </Modal>

  <Modal v-if="deleteTarget" title="删除项目" @close="deleteTarget = null">
    <p style="margin:0">确定删除「{{ deleteTarget.name }}」？
      {{ deleteCount >= 0 ? `项目内 ${deleteCount} 个记录/文件` : '项目内所有内容' }}将一并从磁盘删除，无法恢复。</p>
    <template #foot>
      <button class="btn" @click="deleteTarget = null">取消</button>
      <button class="btn btn-danger" :disabled="deleting" @click="doDelete">删除</button>
    </template>
  </Modal>
</template>

<style scoped>
.tree-empty { padding: 4px 10px 8px; font-size: 12px; color: var(--ui-muted); }
.tname { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.tacts { display: none; gap: 2px; }
.tree-item:hover .tacts { display: inline-flex; }
.tact {
  border: none;
  background: none;
  cursor: pointer;
  color: var(--ui-content3);
  font-size: 13px;
  line-height: 1;
  padding: 3px 5px;
  border-radius: var(--radius-tiny);
}
.tact:hover { background: var(--ui-default-hover); color: var(--ui-content1); }
.tact.danger:hover { color: var(--ui-danger); }
</style>
