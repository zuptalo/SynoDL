/**
 * Prompt-based PWA updates (constitution Principle V): a new deploy never
 * silently reloads. vite-plugin-pwa's registerType:'prompt' parks the fresh
 * service worker in "waiting"; we surface that as `updateAvailable` and only
 * message SKIP_WAITING when the user accepts, then reload once it activates.
 */
import { ref } from 'vue';
import { useRegisterSW } from 'virtual:pwa-register/vue';

export function useAppUpdate() {
  const applying = ref(false);
  const { needRefresh, updateServiceWorker } = useRegisterSW({
    immediate: true,
  });

  async function applyUpdate(): Promise<void> {
    applying.value = true;
    // updateServiceWorker(true) posts SKIP_WAITING and reloads on activation.
    await updateServiceWorker(true);
  }

  return { updateAvailable: needRefresh, applying, applyUpdate };
}
