import type { Asset, Project, ProjectGroups, RefResult, SearchHit, Settings } from './types'

const BASE = '/api/v1'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// 428 时全局回调（App 挂载后注册，用于跳转引导页）
let onWorkspaceRequired: (() => void) | null = null
export function setWorkspaceRequiredHandler(fn: () => void) {
  onWorkspaceRequired = fn
}

// 401 时全局回调（App 挂载后注册，用于弹出登录层）
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}
export function triggerUnauthorized() {
  onUnauthorized?.()
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(BASE + path, init)
  if (resp.status === 428) {
    onWorkspaceRequired?.()
    throw new ApiError(428, '请先选择工作目录')
  }
  if (resp.status === 401 && !path.startsWith('/auth/')) {
    onUnauthorized?.()
    throw new ApiError(401, '需要密码')
  }
  if (!resp.ok) {
    let msg = resp.statusText
    try {
      const body = await resp.json()
      msg = body.error || msg
    } catch { /* 非 JSON 响应 */ }
    throw new ApiError(resp.status, msg)
  }
  if (resp.status === 204) return undefined as T
  return resp.json() as Promise<T>
}

function json(body: unknown): RequestInit {
  return { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

export const api = {
  authStatus: () => request<{ has_password: boolean; authorized: boolean }>('/auth/status'),
  login: (password: string) => request<{ ok: boolean }>('/auth/login', { method: 'POST', ...json({ password }) }),
  logout: () => request<{ ok: boolean }>('/auth/logout', { method: 'POST' }),
  putAccess: (password: string | null, lan: boolean | null) =>
    request<{ has_password: boolean; lan_enabled: boolean }>('/access', { method: 'PUT', ...json({ password, lan }) }),
  getSettings: () => request<Settings>('/settings'),
  putSettings: (workspace: string) => request<Settings>('/settings', { method: 'PUT', ...json({ workspace }) }),
  pickFolder: () => request<{ path: string }>('/pick-folder'),

  listProjects: () => request<ProjectGroups>('/projects'),
  getProject: (id: string) => request<{ project: Project; pending: boolean }>(`/projects/${id}`),
  createProject: (name: string) => request<Project>('/projects', { method: 'POST', ...json({ name }) }),
  renameProject: (id: string, name: string) => request<Project>(`/projects/${id}`, { method: 'PUT', ...json({ name }) }),
  deleteProject: (id: string) => request<void>(`/projects/${id}`, { method: 'DELETE' }),
  listAssets: (projectId: string, category?: 'record' | 'file') =>
    request<Asset[]>(`/projects/${projectId}/assets${category ? `?category=${category}` : ''}`),

  // 保持串行：同名文件的排重依赖前一个已重命名落盘，并行会撞名
  async uploadFiles(projectId: string, category: string, files: File[]): Promise<Asset[]> {
    const out: Asset[] = []
    for (const f of files) {
      const fd = new FormData()
      fd.append('category', category)
      fd.append('file', f)
      out.push(await request<Asset>(`/projects/${projectId}/upload`, { method: 'POST', body: fd }))
    }
    return out
  },
  createBlank: (projectId: string, title: string) =>
    request<Asset>(`/projects/${projectId}/records/blank`, { method: 'POST', ...json({ title }) }),

  getText: async (assetId: number): Promise<{ text: string; size: number }> => {
    const resp = await fetch(`${BASE}/assets/${assetId}/text`)
    if (!resp.ok) throw new ApiError(resp.status, (await resp.json().catch(() => ({}))).error || '读取失败')
    return { text: await resp.text(), size: Number(resp.headers.get('X-File-Size')) }
  },
  putText: (assetId: number, text: string, ifMatch: number) =>
    request<{ ok: boolean; size: number }>(`/assets/${assetId}/text`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain; charset=utf-8', 'If-Match': String(ifMatch) },
      body: text,
    }),

  getReferences: (recordId: number) => request<RefResult[]>(`/records/${recordId}/references`),

  batchDelete: (ids: number[]) => request<{ deleted: number }>('/batch/delete', { method: 'POST', ...json({ ids }) }),
  batchDownloadUrl: (ids: number[]) => `${BASE}/batch/download?ids=${ids.join(',')}`,

  search: (q: string, project?: string) =>
    request<SearchHit[]>(`/search?q=${encodeURIComponent(q)}${project ? `&project=${encodeURIComponent(project)}` : ''}`),

  downloadUrl: (assetId: number, inline = false) => `${BASE}/assets/${assetId}/download${inline ? '?inline=1' : ''}`,
}

// 图片与 PDF 可在浮层内预览，其余类型维持下载打开
const PREVIEWABLE = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'pdf']
export function isPreviewable(ext: string): boolean {
  return PREVIEWABLE.includes(ext.toLowerCase().replace(/^\./, ''))
}

export function formatSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
