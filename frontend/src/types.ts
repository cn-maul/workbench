export interface Project {
  // 对外项目标识：自定义项目是目录短码，月度项目是 -YYYYMM
  id: string
  name: string
  type: 'monthly' | 'custom'
  year_month?: string
  created_at: string
  pending?: boolean
}

export interface ProjectGroups {
  current_monthly: Project
  history_monthly: Project[]
  custom: Project[]
}

export interface Asset {
  id: number
  project_id: string
  category: 'record' | 'file'
  original_name: string
  stored_name: string
  stored_path: string
  ext: string
  size: number
  uploaded_at: string
}

export interface Settings {
  workspace: string
  workspace_exists: boolean
  needs_select: boolean
  has_password?: boolean
  lan_enabled?: boolean
  port?: number
  migrated?: number
  skipped?: number
}

export interface RefResult {
  name: string
  exists: boolean
  matched?: Asset
  candidates: Asset[]
}

export interface SearchHit {
  id: number
  project_id: string
  project_name: string
  category: 'record' | 'file'
  original_name: string
  ext: string
  size: number
  uploaded_at: string
}

export interface TabItem<T extends string = string> {
  value: T
  label: string
}
