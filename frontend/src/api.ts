import type { Asset, Project, ProjectGroups, RefResult, Settings } from './types'

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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(BASE + path, init)
  if (resp.status === 428) {
    onWorkspaceRequired?.()
    throw new ApiError(428, '请先选择工作目录')
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
  getSettings: () => request<Settings>('/settings'),
  putSettings: (workspace: string) => request<Settings>('/settings', { method: 'PUT', ...json({ workspace }) }),
  pickFolder: () => request<{ path: string }>('/pick-folder'),

  listProjects: () => request<ProjectGroups>('/projects'),
  getProject: (id: number | string) => request<{ project: Project; pending: boolean }>(`/projects/${id}`),
  createProject: (name: string) => request<Project>('/projects', { method: 'POST', ...json({ name }) }),
  listAssets: (projectId: number | string, category?: 'record' | 'file') =>
    request<Asset[]>(`/projects/${projectId}/assets${category ? `?category=${category}` : ''}`),

  async uploadFiles(projectId: number | string, category: string, files: File[]): Promise<Asset[]> {
    const out: Asset[] = []
    for (const f of files) {
      const fd = new FormData()
      fd.append('category', category)
      fd.append('file', f)
      out.push(await request<Asset>(`/projects/${projectId}/upload`, { method: 'POST', body: fd }))
    }
    return out
  },
  createBlank: (projectId: number | string, title: string) =>
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

  downloadUrl: (assetId: number) => `${BASE}/assets/${assetId}/download`,
}

export function formatSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
