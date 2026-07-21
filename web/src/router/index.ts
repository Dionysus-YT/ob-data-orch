import { createRouter, createWebHistory } from 'vue-router'

import EngineeringShellView from '@/views/EngineeringShellView.vue'
import NotFoundView from '@/views/NotFoundView.vue'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'engineering-shell',
      component: EngineeringShellView,
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: NotFoundView,
    },
  ],
})
