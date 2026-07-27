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
    // First-run setup wizard (stateful mode only); the gate steers here.
    path: '/setup',
    component: () => import('@/views/SetupWizard.vue'),
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

// Setup + session gate. restoreSession() resolves mode + any persisted session
// from IndexedDB before the first navigation, so a reload keeps you where you
// belong. Order: unconfigured stateful instance → wizard; otherwise → login
// unless authenticated.
router.beforeEach(async (to) => {
  await restoreSession();
  const { isAuthenticated, needsSetup } = useSession();

  if (needsSetup.value) {
    return to.path === '/setup' ? true : { path: '/setup' };
  }
  // Setup is done (or legacy mode): the wizard is off-limits.
  if (to.path === '/setup') {
    return { path: isAuthenticated.value ? '/tabs/tasks' : '/login' };
  }
  if (to.path !== '/login' && !isAuthenticated.value) {
    return { path: '/login' };
  }
  if (to.path === '/login' && isAuthenticated.value) {
    return { path: '/tabs/tasks' };
  }
  return true;
});

export default router;
