const KEY = 'wb-theme'

export function initTheme() {
  const saved = localStorage.getItem(KEY)
  const dark = saved ? saved === 'dark' : window.matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
}

export function isDark() {
  return document.documentElement.dataset.theme === 'dark'
}

export function toggleTheme() {
  const next = isDark() ? 'light' : 'dark'
  document.documentElement.dataset.theme = next
  localStorage.setItem(KEY, next)
}
