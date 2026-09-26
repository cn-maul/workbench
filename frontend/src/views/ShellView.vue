<script setup lang="ts">
import { ref } from 'vue'
import ProjectTree from '../components/ProjectTree.vue'
import { isDark, toggleTheme } from '../theme'

const dark = ref(isDark())
function onToggle() {
  toggleTheme()
  dark.value = isDark()
}
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <span class="title">工作台</span>
      <span class="spacer" />
      <button class="btn btn-icon" :title="dark ? '切换浅色' : '切换暗色'" @click="onToggle">{{ dark ? '☾' : '☀' }}</button>
      <router-link :to="{ name: 'settings' }" class="btn">设置</router-link>
    </header>
    <div class="main">
      <aside class="sidebar"><ProjectTree /></aside>
      <main class="content"><router-view /></main>
    </div>
  </div>
</template>
