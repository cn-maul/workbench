<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import MarkdownIt from 'markdown-it'
import DOMPurify from 'dompurify'
import { api, type ApiError } from '../api'
import type { Asset, RefResult } from '../types'
import { toast } from '../useToast'

const props = defineProps<{ asset: Asset; projectId: string; startEditing?: boolean }>()
const emit = defineEmits<{ close: []; saved: [] }>()

const md = new MarkdownIt({ html: false, linkify: false })

const text = ref('')
const lastSaved = ref('')
const size = ref(0)
const refs = ref<RefResult[]>([])
const editing = ref(!!props.startEditing)
const preview = ref(false)
const dirty = ref(false)
const busy = ref(false)
const taRef = ref<HTMLTextAreaElement | null>(null)

function renderMd(src: string): string {
  let html = md.render(src)
  html = html.replace(/\[\[([^\]]+)\]\]/g, (_m, name: string) => {
    const r = refs.value.find(x => x.name === name)
    const cls = r && !r.exists ? 'md-ref missing' : 'md-ref'
    return `<span class="${cls}" data-ref="${encodeURIComponent(name)}" title="[[${name}]]">${name}</span>`
  })
  return DOMPurify.sanitize(html)
}
const rendered = computed(() => renderMd(text.value))

async function load() {
  try {
    const r = await api.getText(props.asset.id)
    text.value = r.text
    lastSaved.value = r.text
    size.value = r.size
    refs.value = await api.getReferences(props.asset.id)
  } catch (e) {
    toast((e as ApiError).message, true)
  }
}
onMounted(load)

async function save() {
  if (busy.value) return
  busy.value = true
  try {
    const r = await api.putText(props.asset.id, text.value, size.value)
    size.value = r.size
    lastSaved.value = text.value
    dirty.value = false
    refs.value = await api.getReferences(props.asset.id)
    emit('saved')
    toast('已保存')
  } catch (e) {
    const err = e as ApiError
    if (err.status === 409) {
      toast('文件在外部已变化，已重新载入最新内容', true)
      const r = await api.getText(props.asset.id)
      text.value = r.text
      lastSaved.value = r.text
      size.value = r.size
      dirty.value = false
      preview.value = false
    } else {
      toast(err.message, true)
    }
  } finally {
    busy.value = false
  }
}

function onGlobalKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
    e.preventDefault()
    if (dirty.value) save()
  }
}
onMounted(() => window.addEventListener('keydown', onGlobalKey))
onUnmounted(() => window.removeEventListener('keydown', onGlobalKey))

function onInput(e: Event) {
  text.value = (e.target as HTMLTextAreaElement).value
  dirty.value = true
}

function onKey(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'b') { e.preventDefault(); wrapSel('**', '**', '粗体') }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'i') { e.preventDefault(); wrapSel('*', '*', '斜体') }
}

// 点工具栏按钮不清掉 textarea 选区；筛选输入框除外
function protectToolbar(e: MouseEvent) {
  if ((e.target as HTMLElement).closest('button')) e.preventDefault()
}

function startEdit() {
  editing.value = true
  preview.value = false
  nextTick(() => taRef.value?.focus())
}
function cancelEdit() {
  text.value = lastSaved.value
  dirty.value = false
  preview.value = false
  editing.value = false
}
function close() {
  if (dirty.value && !confirm('有未保存的修改，确定关闭？')) return
  emit('close')
}

/* ---------- 工具栏动作（作用于整框的选区） ---------- */
function sel(): [number, number] {
  const el = taRef.value
  return el ? [el.selectionStart, el.selectionEnd] : [text.value.length, text.value.length]
}

function apply(newText: string, s: number, e: number) {
  text.value = newText
  dirty.value = true
  nextTick(() => {
    const el = taRef.value
    if (!el) return
    el.focus()
    el.selectionStart = s
    el.selectionEnd = e
  })
}

function wrapSel(pre: string, post: string, ph: string) {
  if (preview.value) { toast('请先退出预览再使用工具栏'); return }
  if (!editing.value) startEdit()
  nextTick(() => {
    const [s, e] = sel()
    const selText = text.value.slice(s, e) || ph
    apply(text.value.slice(0, s) + pre + selText + post + text.value.slice(e), s + pre.length, s + pre.length + selText.length)
  })
}

function linePrefix(fn: (l: string) => string) {
  if (preview.value) { toast('请先退出预览再使用工具栏'); return }
  if (!editing.value) startEdit()
  nextTick(() => {
    const el = taRef.value
    if (!el) return
    let s = text.value.lastIndexOf('\n', el.selectionStart - 1) + 1
    let e = text.value.indexOf('\n', el.selectionEnd)
    if (e === -1) e = text.value.length
    const seg = text.value.slice(s, e).split('\n').map(fn).join('\n')
    apply(text.value.slice(0, s) + seg + text.value.slice(e), s, s + seg.length)
  })
}

const showHeadMenu = ref(false)
function setHeading(level: number) {
  linePrefix(l => {
    const bare = l.replace(/^#{1,6}\s+/, '')
    return level > 0 ? `${'#'.repeat(level)} ${bare}` : bare
  })
  showHeadMenu.value = false
}
function toggleUl() {
  linePrefix(l => /^\s*[-*+]\s/.test(l) ? l.replace(/^(\s*)[-*+]\s/, '$1') : `- ${l.replace(/^\s*(\d+\.\s+|[-*+]\s+)?/, '')}`)
}
function toggleOl() {
  let n = 0
  linePrefix(l => {
    n++
    return /^\s*\d+\.\s/.test(l) ? l.replace(/^(\s*)\d+\.\s/, '$1') : `${n}. ${l.replace(/^\s*(\d+\.\s+|[-*+]\s+)?/, '')}`
  })
}
function insertTable() {
  if (preview.value) { toast('请先退出预览再使用工具栏'); return }
  if (!editing.value) startEdit()
  nextTick(() => {
    const el = taRef.value
    const at = el ? el.selectionStart : text.value.length
    const t = '| 列1 | 列2 |\n| --- | --- |\n|  |  |\n|  |  |'
    const head = text.value.slice(0, at)
    const join = head && !head.endsWith('\n\n') ? (head.endsWith('\n') ? '\n' : '\n\n') : ''
    apply(text.value.slice(0, at) + join + t + '\n' + text.value.slice(at), at + join.length, at + join.length + t.length)
  })
}

/* ---------- 插入文件 ---------- */
const showInsert = ref(false)
const insertList = ref<Asset[]>([])
const filterText = ref('')
const filteredInsert = computed(() => {
  const q = filterText.value.trim().toLowerCase()
  return q ? insertList.value.filter(a => a.original_name.toLowerCase().includes(q)) : insertList.value
})

async function openInsert() {
  showHeadMenu.value = false
  showInsert.value = !showInsert.value
  if (showInsert.value && !insertList.value.length) {
    try {
      insertList.value = await api.listAssets(props.projectId, 'file')
    } catch (e) {
      toast((e as ApiError).message, true)
    }
  }
}

function insertRef(name: string) {
  showInsert.value = false
  const token = `[[${name}]]`
  if (preview.value) {
    text.value += (text.value.endsWith('\n') || !text.value ? '' : '\n') + token + '\n'
    dirty.value = true
    return
  }
  if (!editing.value) startEdit()
  nextTick(() => {
    const [s, e] = sel()
    apply(text.value.slice(0, s) + token + text.value.slice(e), s + token.length, s + token.length)
  })
}

/* ---------- 预览里的引用点击 ---------- */
function onPreviewClick(e: MouseEvent) {
  const el = (e.target as HTMLElement).closest?.('[data-ref]') as HTMLElement | null
  if (!el) return
  const name = decodeURIComponent(el.dataset.ref || '')
  const r = refs.value.find(x => x.name === name)
  if (r?.matched) window.open(api.downloadUrl(r.matched.id), '_blank')
  else toast(`引用不存在：${name}`, true)
}
</script>

<template>
  <div class="ov-mask" @click.self="close">
    <div class="ov">
      <header class="ov-head">
        <strong>{{ asset.original_name }}</strong>
        <span v-if="dirty" class="dot" title="未保存" />
        <span style="flex:1"></span>
        <template v-if="editing">
          <button class="btn btn-sm" @click="preview = !preview">{{ preview ? '继续编辑' : '预览' }}</button>
          <button class="btn btn-sm btn-primary" :disabled="!dirty || busy" @click="save">保存 <span class="kbd">Ctrl+S</span></button>
          <button v-if="dirty" class="btn btn-sm" @click="cancelEdit">放弃修改</button>
        </template>
        <template v-else>
          <button class="btn btn-sm" @click="startEdit">编辑</button>
          <a class="btn btn-sm" :href="api.downloadUrl(asset.id)">下载</a>
        </template>
        <button class="btn btn-sm" @click="close">关闭</button>
      </header>

      <div v-if="editing && !preview" class="toolbar" @mousedown="protectToolbar">
        <div class="tb-item">
          <button class="tb-btn" @click="showHeadMenu = !showHeadMenu; showInsert = false">标题 ▾</button>
          <div v-if="showHeadMenu" class="tb-menu">
            <div class="tb-menu-item" @click="setHeading(0)">正文</div>
            <div v-for="l in 6" :key="l" class="tb-menu-item" @click="setHeading(l)">H{{ l }} 标题</div>
          </div>
        </div>
        <span class="tb-sep" />
        <button class="tb-btn" title="加粗 Ctrl+B" @click="wrapSel('**', '**', '粗体')"><b>B</b></button>
        <button class="tb-btn" title="斜体 Ctrl+I" @click="wrapSel('*', '*', '斜体')"><i>I</i></button>
        <button class="tb-btn" title="行内代码" @click="wrapSel('`', '`', 'code')">&lt;/&gt;</button>
        <button class="tb-btn" title="代码块" @click="wrapSel('```\n', '\n```', '代码')">代码块</button>
        <span class="tb-sep" />
        <button class="tb-btn" title="无序列表" @click="toggleUl">• 列表</button>
        <button class="tb-btn" title="有序列表" @click="toggleOl">1. 列表</button>
        <button class="tb-btn" title="引用" @click="linePrefix(l => l.startsWith('>') ? l.replace(/^>\s?/, '') : `> ${l.replace(/^\s*>\s?/, '')}`)">❝ 引用</button>
        <button class="tb-btn" title="表格" @click="insertTable">表格</button>
        <button class="tb-btn" title="链接" @click="wrapSel('[', '](https://)', '链接文字')">链接</button>
        <span class="tb-sep" />
        <button class="tb-btn" @click="openInsert">插入文件 ▾</button>
        <div v-if="showInsert" class="tb-menu tb-menu-wide">
          <input class="input" v-model="filterText" placeholder="筛选…" style="margin-bottom:6px" />
          <div class="tb-menu-scroll">
            <div v-for="a in filteredInsert" :key="a.id" class="tb-menu-item" @click="insertRef(a.original_name)">[[{{ a.original_name }}]]</div>
            <div v-if="!filteredInsert.length" class="empty" style="padding:16px">项目文件库暂无文件</div>
          </div>
        </div>
      </div>

      <div v-if="editing && !preview" class="ov-body">
        <textarea ref="taRef" class="editor-ta" :value="text" spellcheck="false"
          @input="onInput" @keydown="onKey" />
      </div>
      <div v-else class="ov-body" @click="onPreviewClick">
        <div v-if="text.trim()" class="md-body" v-html="rendered" />
        <div v-else class="empty">空记录</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.ov-mask {
  position: fixed;
  inset: 0;
  background: var(--ui-overlay);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 100;
  animation: fade-in 0.18s ease-out;
}
.ov {
  background: var(--ui-background);
  border-radius: var(--radius-large);
  box-shadow: var(--shadow-lg);
  width: min(860px, calc(100vw - 32px));
  height: min(640px, 88vh);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  animation: pop-in 0.22s cubic-bezier(0.2, 0.9, 0.3, 1.2);
}
.ov-head {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 18px;
  border-bottom: 1px solid var(--ui-divider);
  flex-shrink: 0;
}
.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--ui-warning); flex-shrink: 0; }
.ov-body { padding: 12px 18px 16px; overflow-y: auto; flex: 1; min-height: 0; }

.editor-ta {
  display: block;
  width: 100%;
  height: 100%;
  border: none;
  background: transparent;
  color: var(--ui-content1);
  font-family: ui-monospace, 'Cascadia Code', Consolas, monospace;
  font-size: 13.5px;
  line-height: 1.8;
  padding: 0;
  resize: none;
  outline: none;
}

.kbd { font-size: 10px; opacity: 0.75; border: 1px solid rgba(255, 255, 255, 0.4); border-radius: 4px; padding: 0 4px; margin-left: 4px; }

.toolbar {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 6px 16px;
  border-bottom: 1px solid var(--ui-divider);
  background: var(--ui-content-background1);
  flex-shrink: 0;
  position: relative;
  flex-wrap: wrap;
}
.tb-item { position: relative; }
.tb-btn {
  border: none;
  background: none;
  font-family: inherit;
  font-size: 13px;
  color: var(--ui-content2);
  padding: 4px 9px;
  border-radius: var(--radius-small);
  cursor: pointer;
}
.tb-btn:hover { background: var(--ui-content-background2); color: var(--ui-content1); }
.tb-sep { width: 1px; height: 16px; background: var(--ui-default-300); margin: 0 6px; }
.tb-menu {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  z-index: 20;
  min-width: 130px;
  background: var(--ui-background);
  border: 1px solid var(--ui-default-200);
  border-radius: var(--radius-medium);
  box-shadow: var(--shadow-md);
  padding: 4px;
}
.tb-menu-wide { width: 280px; padding: 10px; }
.tb-menu-item { padding: 6px 10px; border-radius: var(--radius-small); cursor: pointer; font-size: 13px; }
.tb-menu-item:hover { background: var(--ui-content-background1); }
.tb-menu-scroll { max-height: 260px; overflow-y: auto; }
</style>
