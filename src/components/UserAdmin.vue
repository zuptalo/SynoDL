<script setup lang="ts">
import {
  alertController,
  IonBadge,
  IonButton,
  IonButtons,
  IonChip,
  IonContent,
  IonHeader,
  IonIcon,
  IonInput,
  IonItem,
  IonItemOption,
  IonItemOptions,
  IonItemSliding,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { addOutline, closeCircle, folderOutline, shieldCheckmarkOutline, speedometerOutline } from 'ionicons/icons';
import { computed, onMounted, ref } from 'vue';
import { api, ApiError, type AdminUser } from '@/services/api';
import { appToast } from '@/services/toast';
import { messageForError } from '@/services/syno-errors';
import { useSession } from '@/composables/useSession';
import FolderPickerModal from '@/components/FolderPickerModal.vue';

const { user } = useSession();
const currentUserId = computed(() => user.value?.id ?? -1);

const users = ref<AdminUser[]>([]);
const error = ref('');

// The owner (the first account) sorts first, then the other admins, then
// everyone else — each group alphabetically.
const sortedUsers = computed(() =>
  [...users.value].sort((a, b) => {
    const rank = (u: AdminUser): number => (u.isOwner ? 0 : u.isAdmin ? 1 : 2);
    return rank(a) - rank(b) || a.username.localeCompare(b.username);
  }),
);
const currentUserIsOwner = computed(() => users.value.find((u) => u.isOwner)?.id === currentUserId.value);

// Owner protection. The owner can never be demoted, disabled, or deleted — not
// even by themselves — so a full-access account always survives. And only the
// owner may reset the owner's password; other admins can't touch it. The server
// enforces all of this too (these guards just hide the affordances).
function canToggleAdmin(u: AdminUser): boolean {
  return !u.isOwner && u.id !== currentUserId.value;
}
function canToggleEnabled(u: AdminUser): boolean {
  return !u.isOwner && u.id !== currentUserId.value;
}
function canReset(u: AdminUser): boolean {
  return !u.isOwner || currentUserIsOwner.value;
}
function canDelete(u: AdminUser): boolean {
  return !u.isOwner && u.id !== currentUserId.value;
}
function hasEndActions(u: AdminUser): boolean {
  return canToggleEnabled(u) || canReset(u) || canDelete(u);
}

// Add-user form. New users are always non-admin; elevate them afterwards via the
// per-row Admin toggle.
const newUsername = ref('');
const newPassword = ref('');
const showPassword = ref(false);
const adding = ref(false);

// Folder-scope editor
const folderUserId = ref<number | null>(null);
const folderUsername = ref('');
const folderList = ref<string[]>([]);
const folderOpen = ref(false);
const pickerOpen = ref(false);

async function load(): Promise<void> {
  error.value = '';
  try {
    users.value = (await api.listUsers()).users;
  } catch (e) {
    error.value = messageForError(e);
  }
}
onMounted(load);

// A strong, unambiguous password. Avoids look-alike characters (0/O, 1/l/I) so
// it survives being read aloud or retyped if the clipboard hand-off fails.
function generatePassword(len = 16): string {
  const alphabet = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789@#%?';
  const bytes = new Uint32Array(len);
  crypto.getRandomValues(bytes);
  let out = '';
  for (let i = 0; i < len; i += 1) out += alphabet[bytes[i] % alphabet.length];
  return out;
}

function fillGeneratedPassword(): void {
  newPassword.value = generatePassword();
  showPassword.value = true; // reveal it so the admin can double-check before adding
}

// The install/onboarding blurb an admin can hand a user verbatim.
function onboardingText(username: string, password: string): string {
  const url = window.location.origin;
  return [
    'Your SynoDL account is ready.',
    '',
    `Username: ${username}`,
    `Password: ${password}`,
    '',
    `Open ${url} on your phone, then use your browser's "Add to Home Screen" to install the app and sign in.`,
  ].join('\n');
}

async function toast(message: string, color = 'primary'): Promise<void> {
  await appToast({ message, duration: 4000, color });
}

// Copy the onboarding message to the clipboard; returns whether it worked so the
// caller can fall back to showing the password in a toast.
async function copyOnboarding(username: string, password: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(onboardingText(username, password));
    return true;
  } catch {
    return false;
  }
}

async function addUser(): Promise<void> {
  adding.value = true;
  error.value = '';
  if (!newPassword.value) newPassword.value = generatePassword();
  const username = newUsername.value.trim();
  const password = newPassword.value;
  try {
    await api.createUser(username, password, false);
    const copied = await copyOnboarding(username, password);
    newUsername.value = '';
    newPassword.value = '';
    showPassword.value = false;
    await load();
    await toast(
      copied
        ? `Added ${username} — username, password & install link copied to the clipboard.`
        : `Added ${username}. Password: ${password} (couldn't copy — save it now).`,
      copied ? 'primary' : 'warning',
    );
  } catch (e) {
    error.value = e instanceof ApiError && e.status === 409 ? 'That username is already taken.' : messageForError(e);
  } finally {
    adding.value = false;
  }
}

// Slide the row back to its resting state after an option is chosen, so a picked
// swipe action never leaves the item stuck open.
function closeSlider(ev: Event): void {
  const el = (ev.target as HTMLElement | null)?.closest('ion-item-sliding') as
    | (HTMLElement & { close?: () => Promise<void> })
    | null;
  void el?.close?.();
}

async function confirmAction(
  header: string,
  message: string,
  confirmText: string,
  destructive = false,
): Promise<boolean> {
  const alert = await alertController.create({
    header,
    message,
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: confirmText, role: destructive ? 'destructive' : 'confirm' },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  return role === 'confirm' || role === 'destructive';
}

async function toggleEnabled(u: AdminUser, ev: Event): Promise<void> {
  closeSlider(ev);
  const disabling = u.isEnabled;
  const ok = await confirmAction(
    disabling ? `Disable ${u.username}?` : `Enable ${u.username}?`,
    disabling ? "They won't be able to sign in until you re-enable them." : 'They will be able to sign in again.',
    disabling ? 'Disable' : 'Enable',
    disabling,
  );
  if (!ok) return;
  try {
    await api.updateUser(u.id, { isEnabled: !u.isEnabled });
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

// Elevate/demote a user's admin role (you can't change your own).
async function toggleAdmin(u: AdminUser, ev: Event): Promise<void> {
  closeSlider(ev);
  if (u.id === currentUserId.value) return;
  const removing = u.isAdmin;
  const ok = await confirmAction(
    removing ? `Remove admin from ${u.username}?` : `Make ${u.username} an admin?`,
    removing
      ? 'They lose access to user management, the NAS connection and the download source.'
      : 'They get full access to user management, the NAS connection and the download source.',
    removing ? 'Remove admin' : 'Make admin',
    removing,
  );
  if (!ok) return;
  try {
    await api.updateUser(u.id, { isAdmin: !u.isAdmin });
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

// Reset generates a fresh password (after a confirm) and copies the same
// username + password + install guide the add-user flow produces.
async function resetPassword(u: AdminUser, ev: Event): Promise<void> {
  closeSlider(ev);
  const alert = await alertController.create({
    header: `Reset ${u.username}'s password?`,
    message: "A new password is generated and copied to your clipboard with the sign-in guide, ready to share.",
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Reset', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  if (role !== 'confirm') return;
  const pw = generatePassword();
  try {
    await api.updateUser(u.id, { password: pw });
    const copied = await copyOnboarding(u.username, pw);
    await toast(
      copied
        ? `Reset ${u.username} — new password & sign-in guide copied to the clipboard.`
        : `New password for ${u.username}: ${pw} (couldn't copy — save it now).`,
      copied ? 'primary' : 'warning',
    );
  } catch (e) {
    error.value = messageForError(e);
  }
}

// Friendly content-rating tiers. The provider filters by an EXACT rating, so a
// tier maps to one rating value the capped user will be limited to.
const RATINGS = [
  { label: 'Unrestricted', value: '' },
  { label: 'Young kids — G', value: 'G' },
  { label: 'Kids — PG', value: 'PG' },
  { label: 'Teens — PG-13', value: 'PG-13' },
  { label: 'Mature — R', value: 'R' },
];

async function setRating(u: AdminUser): Promise<void> {
  const alert = await alertController.create({
    header: `Content rating for ${u.username}`,
    subHeader: 'Caps what they can see and download in Discover.',
    inputs: RATINGS.map((r) => ({
      type: 'radio' as const,
      label: r.label,
      value: r.value,
      checked: (u.contentRating ?? '') === r.value,
    })),
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Set', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role, data } = await alert.onDidDismiss();
  if (role !== 'confirm') return;
  try {
    await api.updateUser(u.id, { contentRating: (data?.values as string) ?? '' });
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

async function setDailyLimit(u: AdminUser): Promise<void> {
  const alert = await alertController.create({
    header: `Daily download limit for ${u.username}`,
    subHeader: 'Downloads they can start per 24 hours (0 = unlimited).',
    inputs: [{ name: 'n', type: 'number', value: String(u.dailyDownloadLimit ?? 0), min: 0, placeholder: '0' }],
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Set', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role, data } = await alert.onDidDismiss();
  if (role !== 'confirm') return;
  const n = Math.max(0, Math.floor(Number(data?.values?.n) || 0));
  try {
    await api.updateUser(u.id, { dailyDownloadLimit: n });
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

async function removeUser(u: AdminUser, ev: Event): Promise<void> {
  closeSlider(ev);
  const alert = await alertController.create({
    header: `Delete ${u.username}?`,
    message: 'This removes the account and its folder access. Downloads already on the NAS are unaffected.',
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Delete', role: 'destructive' },
    ],
  });
  await alert.present();
  const { role } = await alert.onDidDismiss();
  if (role !== 'destructive') return;
  try {
    await api.deleteUser(u.id);
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

async function openFolders(u: AdminUser): Promise<void> {
  folderUserId.value = u.id;
  folderUsername.value = u.username;
  error.value = '';
  try {
    folderList.value = (await api.getUserFolders(u.id)).folders;
    folderOpen.value = true;
  } catch (e) {
    error.value = messageForError(e);
  }
}

function onPick(dest: string): void {
  if (!folderList.value.includes(dest)) folderList.value.push(dest);
  pickerOpen.value = false;
}

function removeFolder(path: string): void {
  folderList.value = folderList.value.filter((p) => p !== path);
}

async function saveFolders(): Promise<void> {
  if (folderUserId.value === null) return;
  try {
    await api.setUserFolders(folderUserId.value, folderList.value);
    folderOpen.value = false;
  } catch (e) {
    error.value = messageForError(e);
  }
}
</script>

<template>
  <ion-list inset>
    <ion-list-header><ion-label>Existing users</ion-label></ion-list-header>
    <ion-note class="hint">
      Swipe a user right to change their admin role; left for Reset, Enable/Disable and Delete. The
      owner account is protected — only the owner can change it.
    </ion-note>

    <ion-item-sliding v-for="u in sortedUsers" :key="u.id">
      <!-- Swipe right: elevate/demote admin (not for yourself or the owner). -->
      <ion-item-options v-if="canToggleAdmin(u)" side="start">
        <ion-item-option color="primary" data-testid="admin-role" @click="toggleAdmin(u, $event)">
          {{ u.isAdmin ? 'Remove admin' : 'Make admin' }}
        </ion-item-option>
      </ion-item-options>

      <ion-item data-testid="admin-user">
        <ion-label class="ion-text-wrap">
          <!-- Row 1: role in front of the username. -->
          <h2 class="uname">
            <ion-badge v-if="u.isOwner" color="success">Owner</ion-badge>
            <ion-badge v-else-if="u.isAdmin" color="primary">Admin</ion-badge>
            <span :class="{ off: !u.isEnabled }">{{ u.username }}</span>
            <ion-badge v-if="!u.isEnabled" color="medium">disabled</ion-badge>
          </h2>
          <!-- Row 2: per-user controls — right-aligned, labelled buttons so it's
               clear they're tappable. -->
          <div class="row-actions">
            <ion-button size="small" fill="outline" :title="`Content rating for ${u.username}`" @click="setRating(u)">
              <ion-icon slot="start" :icon="shieldCheckmarkOutline" />
              Rating
            </ion-button>
            <ion-button size="small" fill="outline" :title="`Daily download limit for ${u.username}`" @click="setDailyLimit(u)">
              <ion-icon slot="start" :icon="speedometerOutline" />
              Limit
            </ion-button>
            <ion-button size="small" fill="outline" :title="`Folders for ${u.username}`" @click="openFolders(u)">
              <ion-icon slot="start" :icon="folderOutline" />
              Folders
            </ion-button>
          </div>
          <!-- Row 3: the current caps as read-only info. -->
          <p class="limits">
            <span>{{ u.contentRating ? `Rating ${u.contentRating}` : 'No rating cap' }}</span>
            <span>·</span>
            <span>{{ u.dailyDownloadLimit ? `${u.dailyDownloadLimit}/day` : 'No daily limit' }}</span>
          </p>
        </ion-label>
      </ion-item>
      <!-- Swipe left: management actions (all hidden on the protected owner
           account for anyone but the owner). -->
      <ion-item-options v-if="hasEndActions(u)" side="end">
        <ion-item-option v-if="canToggleEnabled(u)" @click="toggleEnabled(u, $event)">
          {{ u.isEnabled ? 'Disable' : 'Enable' }}
        </ion-item-option>
        <ion-item-option v-if="canReset(u)" color="medium" data-testid="admin-reset" @click="resetPassword(u, $event)">Reset</ion-item-option>
        <ion-item-option v-if="canDelete(u)" color="danger" @click="removeUser(u, $event)">Delete</ion-item-option>
      </ion-item-options>
    </ion-item-sliding>
  </ion-list>

  <ion-list inset>
    <ion-list-header><ion-label>Add a user</ion-label></ion-list-header>
    <ion-item>
      <ion-input v-model="newUsername" label="Username" label-placement="stacked" autocapitalize="off" data-testid="admin-new-username" />
    </ion-item>
    <ion-item>
      <ion-input
        v-model="newPassword"
        label="Password"
        label-placement="stacked"
        :type="showPassword ? 'text' : 'password'"
        placeholder="Leave blank to auto-generate"
        data-testid="admin-new-password"
      />
      <ion-button slot="end" fill="clear" size="small" title="Generate a strong password" @click="fillGeneratedPassword">
        <ion-icon slot="icon-only" :icon="addOutline" />
      </ion-button>
    </ion-item>
    <ion-item lines="none">
      <ion-note color="medium" class="pw-hint">
        On add, the username, password and an install link are copied to your clipboard to share.
        New users start as non-admins — use the Admin toggle to elevate them.
      </ion-note>
    </ion-item>
    <ion-item lines="none">
      <ion-button
        :disabled="adding || !newUsername.trim() || (newPassword.length > 0 && newPassword.length < 8)"
        data-testid="admin-add-user"
        @click="addUser"
      >
        <ion-icon slot="start" :icon="addOutline" /> Add user
      </ion-button>
    </ion-item>

    <ion-note v-if="error" color="danger" class="err" data-testid="admin-error">{{ error }}</ion-note>
  </ion-list>

  <!-- Per-user folder scope editor -->
  <ion-modal :is-open="folderOpen" @didDismiss="folderOpen = false">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Folders · {{ folderUsername }}</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="folders-save" @click="saveFolders">Save</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <p class="hint">
        This user can download only into these NAS folders (and their subfolders). No folders means no
        downloads. Admins can use any folder.
      </p>
      <div class="chips">
        <ion-chip v-for="p in folderList" :key="p" data-testid="folder-grant">
          {{ p }}
          <ion-icon :icon="closeCircle" @click="removeFolder(p)" />
        </ion-chip>
        <ion-note v-if="folderList.length === 0" color="medium">No folders granted.</ion-note>
      </div>
      <ion-button expand="block" fill="outline" data-testid="folders-add" @click="pickerOpen = true">
        <ion-icon slot="start" :icon="addOutline" /> Add a folder
      </ion-button>
    </ion-content>
    <FolderPickerModal :is-open="pickerOpen" @pick="onPick" @dismiss="pickerOpen = false" />
  </ion-modal>
</template>

<style scoped>
.hint {
  display: block;
  padding: 0 1rem 0.25rem;
  color: var(--app-text-dim);
  font-size: 0.8rem;
}
.uname {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.uname .off {
  color: var(--app-text-dim);
  text-decoration: line-through;
}
.row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 6px;
  margin: 6px 0 2px;
}
.row-actions ion-button {
  --padding-start: 10px;
  --padding-end: 10px;
  font-size: 0.8rem;
  height: 30px;
  margin: 0;
}
.limits {
  display: flex;
  gap: 8px;
  color: var(--app-text-dim);
  font-size: 0.8rem;
}
.err {
  display: block;
  margin: 0.5rem 1rem;
}
.pw-hint {
  font-size: 0.8rem;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-bottom: 1rem;
}
</style>
