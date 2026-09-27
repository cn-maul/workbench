<script setup lang="ts">
import { ref } from 'vue'
import ProjectTree from '../components/ProjectTree.vue'
import SearchBox from '../components/SearchBox.vue'
import { api, triggerUnauthorized } from '../api'
import { isDark, toggleTheme } from '../theme'

const dark = ref(isDark())
function onToggle() {
  toggleTheme()
  dark.value = isDark()
}

const locked = ref(false)
// has_password 只在启动/登录后生效，导航切换不会变，无需重复查询
refreshLock()
async function refreshLock() {
  try { locked.value = (await api.authStatus()).has_password } catch { /* 忽略 */ }
}

async function doLogout() {
  await api.logout()
  triggerUnauthorized()
}
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <span class="title">工作台</span>
      <SearchBox />
      <span class="spacer" />
      <button class="btn btn-icon" :title="dark ? '切换浅色' : '切换暗色'" @click="onToggle">{{ dark ? '☾' : '☀' }}</button>
      <button v-if="locked" class="btn" title="退出后需重新输入密码" @click="doLogout">退出</button>
      <router-link :to="{ name: 'settings' }" class="btn">设置</router-link>
    </header>
    <div class="main">
      <aside class="sidebar"><ProjectTree /></aside>
      <main class="content"><router-view /></main>
    </div>
  </div>
</template>
