<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { api, type ApiError } from '../api'
import type { Settings, TabItem } from '../types'
import { errToast, toast } from '../useToast'
import { bumpTree } from '../bus'
import Modal from '../components/Modal.vue'
import Tabs from '../components/Tabs.vue'

const secs: TabItem<'workspace' | 'access'>[] = [
  { value: 'workspace', label: '工作目录' },
  { value: 'access', label: '访问控制' },
]
const sec = ref<'workspace' | 'access'>('workspace')

const settings = ref<Settings | null>(null)
const path = ref('')
const busy = ref(false)
const picking = ref(false)

const newPwd = ref('')
const lan = ref(false)
const accessBusy = ref(false)
const showClear = ref(false)

onMounted(async () => {
  try {
    settings.value = await api.getSettings()
    path.value = settings.value.workspace
    lan.value = !!settings.value.lan_enabled
  } catch (e) {
    errToast(e)
  }
})

async function browse() {
  if (picking.value) return
  picking.value = true
  try {
    const r = await api.pickFolder()
    if (r.path) path.value = r.path
  } catch (e) {
    errToast(e as ApiError)
  } finally {
    picking.value = false
  }
}

async function switchWs() {
  if (!path.value.trim() || busy.value) return
  busy.value = true
  try {
    const r = await api.putSettings(path.value.trim())
    settings.value = r
    bumpTree()
    toast(r.migrated ? `已切换，并迁移 ${r.migrated} 个文件` : '已切换工作目录')
  } catch (e) {
    errToast(e)
  } finally {
    busy.value = false
  }
}

async function savePwd() {
  if (!newPwd.value.trim() || accessBusy.value) return
  accessBusy.value = true
  try {
    const r = await api.putAccess(newPwd.value.trim(), null)
    settings.value = { ...settings.value!, ...r }
    newPwd.value = ''
    toast('密码已保存')
  } catch (e) {
    errToast(e)
  } finally {
    accessBusy.value = false
  }
}

async function clearPwd() {
  if (accessBusy.value) return
  accessBusy.value = true
  try {
    const r = await api.putAccess('', false)
    settings.value = { ...settings.value!, ...r }
    lan.value = false
    showClear.value = false
    toast('已清除密码')
  } catch (e) {
    errToast(e)
  } finally {
    accessBusy.value = false
  }
}

async function toggleLan() {
  if (accessBusy.value) return
  accessBusy.value = true
  try {
    const r = await api.putAccess(null, lan.value)
    settings.value = { ...settings.value!, ...r }
  } catch (e) {
    lan.value = !lan.value
    errToast(e)
  } finally {
    accessBusy.value = false
  }
}
</script>

<template>
  <div class="settings">
    <div class="page-head">
      <h2>设置</h2>
      <p class="page-sub">工作目录与访问密码改动即时生效；局域网监听需重启工作台。</p>
    </div>

    <Tabs v-model="sec" :items="secs" class="sec-tabs" />

    <section v-if="sec === 'workspace'" class="card">
      <div class="row">
        <div class="row-info">
          <div class="row-name">当前工作目录</div>
          <div class="row-hint">项目、记录与文件都保存在这里</div>
        </div>
        <div class="row-ctl">
          <span class="path" :title="settings?.workspace">{{ settings?.workspace || '（未设置）' }}</span>
          <span v-if="settings && !settings.workspace_exists" class="badge badge-danger">目录不存在</span>
        </div>
      </div>

      <div class="row">
        <div class="row-info">
          <div class="row-name">切换工作目录</div>
          <div class="row-hint">旧目录的记录与文件会复制到新目录（同名不覆盖），旧目录原样保留</div>
        </div>
        <div class="row-ctl row-form">
          <input class="input" v-model="path" placeholder="D:\其他工作台" @keydown.enter="switchWs" />
          <button class="btn" :disabled="picking" @click="browse">{{ picking ? '等待选择…' : '浏览…' }}</button>
          <button class="btn btn-primary" :disabled="busy || !path.trim()" @click="switchWs">{{ busy ? '迁移中…' : '切换' }}</button>
        </div>
      </div>
    </section>

    <section v-else class="card">
      <div class="row">
        <div class="row-info">
          <div class="row-name">访问密码
            <span class="badge" :class="{ 'badge-primary': settings?.has_password }">{{ settings?.has_password ? '已设置' : '未设置' }}</span>
          </div>
          <div class="row-hint">设置后立即生效，重新打开页面需先输密码。忘记密码：删除 exe 旁 workbench.json 里的 password_sha256 字段后重启。</div>
        </div>
        <div class="row-ctl row-form">
          <input class="input" type="password" v-model="newPwd" :placeholder="settings?.has_password ? '输入新密码以修改' : '设置访问密码'" @keydown.enter="savePwd" />
          <button class="btn btn-primary" :disabled="!newPwd.trim() || accessBusy" @click="savePwd">{{ settings?.has_password ? '修改' : '设置' }}</button>
          <button v-if="settings?.has_password" class="btn btn-danger" :disabled="accessBusy" @click="showClear = true">清除</button>
        </div>
      </div>

      <div class="row">
        <div class="row-info">
          <div class="row-name">局域网访问</div>
          <div class="row-hint">允许同一局域网的其他设备访问（监听 0.0.0.0，放 NAS 上必开）</div>
          <div class="row-hint" v-if="!settings?.has_password">需先设置访问密码才能开启。</div>
          <div class="row-hint" v-else-if="settings?.lan_enabled">已开启：局域网设备访问 http://&lt;本机IP&gt;:{{ settings?.port }}</div>
          <div class="row-hint" v-else>当前关闭，仅本机可访问。</div>
        </div>
        <div class="row-ctl">
          <label class="switch">
            <input type="checkbox" v-model="lan" :disabled="!settings?.has_password || accessBusy" @change="toggleLan" />
            <span class="ctl" />
          </label>
        </div>
      </div>
    </section>
  </div>

  <Modal v-if="showClear" title="清除访问密码" @close="showClear = false">
    <p style="margin:0">清除后进入工作台不再需要密码，局域网访问也会同时关闭。确定清除？</p>
    <template #foot>
      <button class="btn" @click="showClear = false">取消</button>
      <button class="btn btn-danger" :disabled="accessBusy" @click="clearPwd">清除密码</button>
    </template>
  </Modal>
</template>

<style scoped>
.settings { max-width: 820px; }
.page-head { margin-bottom: 14px; }
.page-head h2 { margin: 0 0 4px; font-size: 20px; font-weight: 600; }
.page-sub { margin: 0; font-size: 13px; color: var(--ui-muted); }
/* HeroUI 的 panel 与标签组之间是 mt-4 */
.sec-tabs { width: 100%; max-width: 448px; margin-bottom: 16px; }

/* 这里的卡片是一张「行列表」，所以内衬交给行自己（HeroUI 卡片 p-4 会再加一层） */
section.card { padding: 0; gap: 0; }
.row { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 6px 28px; align-items: center; padding: 16px 22px; }
.row + .row { border-top: 1px solid var(--ui-divider); }
.row-name { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 500; }
.row-hint { margin-top: 3px; font-size: 12px; line-height: 1.65; color: var(--ui-muted); }
.row-ctl { display: flex; align-items: center; justify-content: flex-end; gap: 8px; min-width: 0; }
.row-form .input { flex: 1; min-width: 0; }

.path {
  font-family: 'Cascadia Mono', Consolas, ui-monospace, monospace;
  font-size: 12px;
  color: var(--ui-foreground);
  background: var(--ui-default);
  padding: 5px 10px;
  border-radius: 6px;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .row { grid-template-columns: 1fr; }
  .row-ctl { justify-content: flex-start; }
}
</style>
