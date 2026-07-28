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
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import {
  addOutline,
  closeCircle,
  folderOutline,
  shieldCheckmarkOutline,
  speedometerOutline,
  trashOutline,
} from 'ionicons/icons';
import { onMounted, ref } from 'vue';
import { api, ApiError, type AdminUser } from '@/services/api';
import { messageForError } from '@/services/syno-errors';
import FolderPickerModal from '@/components/FolderPickerModal.vue';

const users = ref<AdminUser[]>([]);
const error = ref('');

// Add-user form
const newUsername = ref('');
const newPassword = ref('');
const newIsAdmin = ref(false);
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

async function addUser(): Promise<void> {
  adding.value = true;
  error.value = '';
  try {
    await api.createUser(newUsername.value.trim(), newPassword.value, newIsAdmin.value);
    newUsername.value = '';
    newPassword.value = '';
    newIsAdmin.value = false;
    await load();
  } catch (e) {
    error.value = e instanceof ApiError && e.status === 409 ? 'That username is already taken.' : messageForError(e);
  } finally {
    adding.value = false;
  }
}

async function toggleEnabled(u: AdminUser): Promise<void> {
  try {
    await api.updateUser(u.id, { isEnabled: !u.isEnabled });
    await load();
  } catch (e) {
    error.value = messageForError(e);
  }
}

async function resetPassword(u: AdminUser): Promise<void> {
  const alert = await alertController.create({
    header: `Reset password for ${u.username}`,
    inputs: [{ name: 'pw', type: 'password', placeholder: 'New password (min 8 characters)' }],
    buttons: [
      { text: 'Cancel', role: 'cancel' },
      { text: 'Set', role: 'confirm' },
    ],
  });
  await alert.present();
  const { role, data } = await alert.onDidDismiss();
  if (role !== 'confirm') return;
  const pw = data?.values?.pw ?? '';
  if (pw.length < 8) {
    error.value = 'Password must be at least 8 characters.';
    return;
  }
  try {
    await api.updateUser(u.id, { password: pw });
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
    inputs: [
      { name: 'n', type: 'number', value: String(u.dailyDownloadLimit ?? 0), min: 0, placeholder: '0' },
    ],
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

async function removeUser(u: AdminUser): Promise<void> {
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
    <ion-list-header><ion-label>Users</ion-label></ion-list-header>

    <ion-item v-for="u in users" :key="u.id" data-testid="admin-user">
      <ion-label>
        <h3>
          {{ u.username }}
          <ion-badge v-if="u.isAdmin" color="primary">admin</ion-badge>
        </h3>
        <p v-if="!u.isEnabled" class="disabled">disabled</p>
        <p v-else class="limits">
          <span v-if="u.contentRating">only {{ u.contentRating }}</span>
          <span v-if="u.dailyDownloadLimit">{{ u.dailyDownloadLimit }}/day</span>
        </p>
      </ion-label>
      <ion-buttons slot="end">
        <ion-button :title="`Content rating for ${u.username}`" @click="setRating(u)">
          <ion-icon slot="icon-only" :icon="shieldCheckmarkOutline" />
        </ion-button>
        <ion-button :title="`Daily download limit for ${u.username}`" @click="setDailyLimit(u)">
          <ion-icon slot="icon-only" :icon="speedometerOutline" />
        </ion-button>
        <ion-button :title="`Folders for ${u.username}`" @click="openFolders(u)">
          <ion-icon slot="icon-only" :icon="folderOutline" />
        </ion-button>
        <ion-button size="small" @click="resetPassword(u)">Reset</ion-button>
        <ion-toggle
          :checked="u.isEnabled"
          :aria-label="`Enabled: ${u.username}`"
          @ionChange="toggleEnabled(u)"
        />
        <ion-button color="danger" @click="removeUser(u)">
          <ion-icon slot="icon-only" :icon="trashOutline" />
        </ion-button>
      </ion-buttons>
    </ion-item>

    <ion-list-header><ion-label>Add a user</ion-label></ion-list-header>
    <ion-item>
      <ion-input v-model="newUsername" label="Username" label-placement="stacked" autocapitalize="off" data-testid="admin-new-username" />
    </ion-item>
    <ion-item>
      <ion-input v-model="newPassword" label="Password (min 8)" label-placement="stacked" type="password" data-testid="admin-new-password" />
    </ion-item>
    <ion-item>
      <ion-toggle v-model="newIsAdmin" data-testid="admin-new-isadmin">Administrator</ion-toggle>
    </ion-item>
    <ion-item lines="none">
      <ion-button
        :disabled="adding || !newUsername || newPassword.length < 8"
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
.disabled {
  color: var(--app-status-error);
}
.limits {
  display: flex;
  gap: 10px;
  color: var(--app-text-dim);
  font-size: 0.8rem;
}
.err {
  display: block;
  margin: 0.5rem 1rem;
}
.hint {
  color: var(--app-text-dim);
  font-size: 0.85rem;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem;
  margin-bottom: 1rem;
}
</style>
