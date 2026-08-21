import { createRouter, createWebHistory } from 'vue-router'
import { routes } from './routes'
import { setupGuards } from './guards'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

setupGuards(router)
