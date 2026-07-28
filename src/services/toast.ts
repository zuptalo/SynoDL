/**
 * The one place in-app notifications (toasts) are created, so every transient
 * confirmation ("Sent to your NAS"), error ("Couldn't reconnect") and status
 * notice shares a single look, position and feel: a themed pill that slides in
 * from the top and can be swiped away. This mirrors the sibling app's in-app
 * notification style — the app's own colours (primary green, danger red) rather
 * than Ionic's default always-dark toast — so notifications feel native here.
 *
 * The visual styling lives in App.vue's global `.app-toast` rules; this helper
 * just picks the right tone class and consistent behaviour for each call.
 */
import { toastController } from '@ionic/vue';

export interface AppToastOptions {
  message: string;
  /** ms on screen; defaults to a short, readable duration. */
  duration?: number;
  /** Tone: 'danger' for errors, 'warning' for cautions, 'success' for wins; else the green theme. */
  color?: 'danger' | 'warning' | 'success' | string;
  /** Optional leading ionicon. */
  icon?: string;
}

function toneClass(color?: string): string {
  switch (color) {
    case 'danger':
      return 'app-toast-danger';
    case 'warning':
      return 'app-toast-warning';
    case 'success':
      return 'app-toast-success';
    default:
      return '';
  }
}

/** Present a functional in-app notification through the shared themed toast. */
export async function appToast(opts: AppToastOptions | string): Promise<void> {
  const o = typeof opts === 'string' ? { message: opts } : opts;
  const tone = toneClass(o.color);
  const t = await toastController.create({
    message: o.message,
    duration: o.duration ?? 3000,
    position: 'top',
    swipeGesture: 'vertical',
    icon: o.icon,
    cssClass: tone ? ['app-toast', tone] : 'app-toast',
  });
  await t.present();
}
