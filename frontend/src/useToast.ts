import { ref } from 'vue'

export interface Toast { id: number; text: string; error?: boolean }

export const toasts = ref<Toast[]>([])

let timer: number | undefined
let nextId = 0

export function toast(text: string, error = false) {
  toasts.value = [{ id: ++nextId, text, error }]
  clearTimeout(timer)
  timer = window.setTimeout(() => { toasts.value = [] }, error ? 4000 : 2200)
}

export function errToast(e: unknown) {
  toast(e instanceof Error ? e.message : String(e), true)
}
