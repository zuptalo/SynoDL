import { createRouter, createWebHistory } from '@ionic/vue-router';
import type { RouteRecordRaw } from 'vue-router';
import { restoreSession } from '@/composables/useSession';
import { useSession } from '@/composables/useSession';
import TabsPage from '@/views/TabsPage.vue';

// The five tab roots are statically imported via TabsPage so first tab-switch
// never stalls on a chunk fetch; only the login page is a separate lazy chunk.
const routes: Array<RouteRecordRaw> = [
  { path: '/', redirect: '/tabs/tasks' },
  {
    path: '/login',
    component: () => import('@/views/LoginPage.vue'),
  },
  {
    path: '/tabs',
    component: TabsPage,
    children: [
      { path: '', redirect: '/tabs/tasks' },
      { path: 'tasks', component: () => import('@/views/tabs/TasksPage.vue') },
      { path: 'search', component: () => import('@/views/tabs/SearchPage.vue') },
      { path: 'browser', component: () => import('@/views/tabs/BrowserPage.vue') },
      { path: 'rss', component: () => import('@/views/tabs/RssPage.vue') },
      { path: 'settings', component: () => import('@/views/tabs/SettingsPage.vue') },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

// Session gate: everything except /login needs a sid; /login bounces away once
// authenticated. restoreSession() resolves from IndexedDB before the first
// navigation so a reload keeps you signed in.
router.beforeEach(async (to) => {
  await restoreSession();
  const { isAuthenticated } = useSession();
  if (to.path !== '/login' && !isAuthenticated.value) {
    return { path: '/login' };
  }
  if (to.path === '/login' && isAuthenticated.value) {
    return { path: '/tabs/tasks' };
  }
  return true;
});

export default router;
