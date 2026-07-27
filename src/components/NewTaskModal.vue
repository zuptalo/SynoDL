<script setup lang="ts">
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonInput,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonTextarea,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import { computed, ref, watch } from 'vue';
import { api, ApiError } from '@/services/api';
import { extractUrls } from '@/services/url-detect';
import { messageForError } from '@/services/syno-errors';
import FolderPickerModal from '@/components/FolderPickerModal.vue';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{
  (e: 'created'): void;
  (e: 'dismiss'): void;
}>();

const text = ref('');
const destination = ref('');
const pickerOpen = ref(false);
const withCredentials = ref(false);
const username = ref('');
const password = ref('');
const withExtract = ref(false);
const unzipPassword = ref('');
const file = ref<File | null>(null);
const busy = ref(false);
const error = ref('');

const urls = computed(() => extractUrls(text.value));
const canSubmit = computed(() => !busy.value && (urls.value.length > 0 || file.value !== null));

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      text.value = '';
      destination.value = '';
      withCredentials.value = false;
      username.value = '';
      password.value = '';
      withExtract.value = false;
      unzipPassword.value = '';
      file.value = null;
      error.value = '';
    }
  },
);

async function pasteFromClipboard(): Promise<void> {
  try {
    const clip = await navigator.clipboard.readText();
    if (clip) text.value = text.value ? `${text.value}\n${clip}` : clip;
  } catch {
    // Permission denied / unsupported: the textarea still works by hand.
  }
}

function onFileChosen(event: Event): void {
  const input = event.target as HTMLInputElement;
  file.value = input.files?.[0] ?? null;
}

async function submit(): Promise<void> {
  busy.value = true;
  error.value = '';
  try {
    if (urls.value.length > 0) {
      await api.createTaskURIs(urls.value, {
        destination: destination.value || undefined,
        username: withCredentials.value ? username.value : undefined,
        password: withCredentials.value ? password.value : undefined,
        unzipPassword: withExtract.value ? unzipPassword.value : undefined,
      });
    }
    if (file.value) {
      await api.createTaskFile(file.value, {
        destination: destination.value || undefined,
        unzipPassword: withExtract.value ? unzipPassword.value : undefined,
      });
    }
    emit('created');
  } catch (e) {
    error.value =
      e instanceof ApiError && e.status === 413
        ? 'That torrent file is too large to upload.'
        : messageForError(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-button data-testid="newtask-cancel" @click="emit('dismiss')">Cancel</ion-button>
        </ion-buttons>
        <ion-title>New task</ion-title>
        <ion-buttons slot="end">
          <ion-button :disabled="!canSubmit" data-testid="newtask-submit" @click="submit">
            Add task
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content>
      <ion-list>
        <ion-list-header><ion-label>Destination</ion-label></ion-list-header>
        <ion-item button :detail="true" data-testid="newtask-destination" @click="pickerOpen = true">
          <ion-label>{{ destination || 'Default folder' }}</ion-label>
        </ion-item>

        <ion-list-header><ion-label>Task URL(s)</ion-label></ion-list-header>
        <ion-item>
          <ion-textarea
            v-model="text"
            :rows="4"
            placeholder="Enter one or many task URLs"
            data-testid="newtask-urls"
          />
        </ion-item>
        <ion-item lines="none">
          <ion-button fill="clear" size="small" data-testid="newtask-paste" @click="pasteFromClipboard">
            Paste link from clipboard
          </ion-button>
          <ion-note slot="end" data-testid="newtask-count">
            {{ urls.length }} link{{ urls.length === 1 ? '' : 's' }} detected
          </ion-note>
        </ion-item>

        <ion-list-header><ion-label>Task file</ion-label></ion-list-header>
        <ion-item>
          <input
            type="file"
            accept=".torrent,.nzb,application/x-bittorrent"
            data-testid="newtask-file"
            @change="onFileChosen"
          />
        </ion-item>

        <ion-list-header><ion-label>Extra</ion-label></ion-list-header>
        <ion-item>
          <ion-toggle v-model="withCredentials" data-testid="newtask-credentials-toggle">
            Add credentials
          </ion-toggle>
        </ion-item>
        <template v-if="withCredentials">
          <ion-item>
            <ion-input v-model="username" label="Username" label-placement="stacked" autocapitalize="off" />
          </ion-item>
          <ion-item>
            <ion-input v-model="password" label="Password" label-placement="stacked" type="password" />
          </ion-item>
        </template>
        <ion-item>
          <ion-toggle v-model="withExtract" data-testid="newtask-extract-toggle">
            Extract password
          </ion-toggle>
        </ion-item>
        <ion-item v-if="withExtract">
          <ion-input v-model="unzipPassword" label="Archive password" label-placement="stacked" />
        </ion-item>
      </ion-list>

      <ion-note v-if="error" color="danger" class="ion-padding" data-testid="newtask-error">
        {{ error }}
      </ion-note>
    </ion-content>

    <FolderPickerModal
      :is-open="pickerOpen"
      @pick="(d) => { destination = d; pickerOpen = false; }"
      @dismiss="pickerOpen = false"
    />
  </ion-modal>
</template>
