import { ref } from 'vue'

export interface Toast { text: string; error?: boolean }

export const toasts = ref<Toast[]>([])

let timer: number | undefined

export function toast(text: string, error = false) {
  toasts.value = [{ text, error }]
  clearTimeout(timer)
  timer = window.setTimeout(() => { toasts.value = [] }, error ? 4000 : 2200)
}
