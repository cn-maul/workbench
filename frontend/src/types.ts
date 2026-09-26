export interface Project {
  id: number
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
  project_id: number
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
}

export interface RefResult {
  name: string
  exists: boolean
  matched?: Asset
  candidates: Asset[]
}
