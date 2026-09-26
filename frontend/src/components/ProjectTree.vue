<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, type ApiError } from '../api'
import type { Project, ProjectGroups } from '../types'
import Modal from './Modal.vue'
import { toast } from '../useToast'
import { treeVersion } from '../bus'

const route = useRoute()
const router = useRouter()

const groups = ref<ProjectGroups | null>(null)
const showCreate = ref(false)
const openHistory = ref(true)
const openCustom = ref(true)
const newName = ref('')

const currentVirtualId = () => {
  const d = new Date()
  return -(d.getFullYear() * 100 + d.getMonth() + 1)
}

async function refresh() {
  try {
    groups.value = await api.listProjects()
  } catch (e) {
    toast((e as ApiError).message, true)
  }
}

watch(() => [route.path, treeVersion.value], refresh, { immediate: true })

function go(p: Project) {
  router.push({ name: 'project', params: { id: String(p.id) } })
}

function isActive(p: Project): boolean {
  const cur = String(route.params.id ?? '')
  if (cur === String(p.id)) return true
  return p.pending === true && cur === String(currentVirtualId())
}

async function create() {
  const name = newName.value.trim()
  if (!name) return
  try {
    const p = await api.createProject(name)
    showCreate.value = false
    newName.value = ''
    await refresh()
    router.push({ name: 'project', params: { id: String(p.id) } })
  } catch (e) {
    toast((e as ApiError).message, true)
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
        <button class="tree-label clickable" @click="openHistory = !openHistory">
          <span class="chev" :class="{ open: openHistory }">▶</span>
          历史月份
          <span class="spacer" />
          <span class="badge">{{ groups.history_monthly.length }}</span>
        </button>
        <div v-show="openHistory" class="tree-body">
          <div v-for="p in groups.history_monthly" :key="p.id" class="tree-item" :class="{ active: isActive(p) }" @click="go(p)">{{ p.name }}</div>
        </div>
      </div>
      <div class="tree-group">
        <button class="tree-label clickable" @click="openCustom = !openCustom">
          <span class="chev" :class="{ open: openCustom }">▶</span>
          自定义项目
          <span class="spacer" />
          <button class="btn btn-sm" @click.stop="showCreate = true">＋</button>
        </button>
        <div v-show="openCustom" class="tree-body">
          <div v-for="p in groups.custom" :key="p.id" class="tree-item" :class="{ active: isActive(p) }" @click="go(p)">{{ p.name }}</div>
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
</template>

<style scoped>
.tree-empty { padding: 4px 10px 8px; font-size: 12px; color: var(--ui-content4); }
</style>
