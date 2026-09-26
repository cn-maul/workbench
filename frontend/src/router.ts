import { createRouter, createWebHistory } from 'vue-router'
import ShellView from './views/ShellView.vue'
import ProjectView from './views/ProjectView.vue'
import SettingsView from './views/SettingsView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ShellView, children: [
      { path: '', redirect: () => {
          const d = new Date()
          return { name: 'project', params: { id: String(-(d.getFullYear() * 100 + d.getMonth() + 1)) } }
        } },
      { path: 'p/:id', name: 'project', component: ProjectView },
      { path: 'settings', name: 'settings', component: SettingsView },
    ] },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export default router
