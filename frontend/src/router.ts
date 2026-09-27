import { createRouter, createWebHistory } from 'vue-router'
import ShellView from './views/ShellView.vue'
import ProjectView from './views/ProjectView.vue'
import SettingsView from './views/SettingsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ShellView, children: [
      { path: '', redirect: () => {
          // 月度项目的对外 id 与后端 model.MonthlyKey 同规则：'-' + YYYYMM
          const d = new Date()
          const ym = `${d.getFullYear()}${String(d.getMonth() + 1).padStart(2, '0')}`
          return { name: 'project', params: { id: `-${ym}` } }
        } },
      { path: 'p/:id', name: 'project', component: ProjectView },
      { path: 'settings', name: 'settings', component: SettingsView },
    ] },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export default router
