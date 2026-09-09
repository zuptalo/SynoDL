<script setup lang="ts">
/**
 * Where music lives on the NAS (spec 1040).
 *
 * Film and TV parents are inherited from a download source, because a source is
 * what puts films there. Music has no source: tracks arrive from YouTube, via a
 * worker that mounts a Kubernetes volume the server itself never sees — and a
 * claim name is not a path anything can be uploaded to. So these two are an
 * operator decision with nowhere else to come from.
 *
 * Leaving one empty is how that upload kind is turned off, which is why the
 * sheet says so rather than treating empty as an error.
 */
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonInput,
  IonItem,
  IonList,
  IonModal,
  IonNote,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { api } from '@/services/api';
import { appToast } from '@/services/toast';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'saved'): void }>();

const music = ref('');
const musicVideo = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');

async function load(): Promise<void> {
  loading.value = true;
  error.value = '';
  try {
    const libs = await api.getMusicLibraries();
    music.value = libs.music;
    musicVideo.value = libs.musicVideo;
  } catch {
    error.value = 'Could not read the current settings.';
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.isOpen,
  (open) => {
    if (open) void load();
  },
  { immediate: true },
);

async function save(): Promise<void> {
  saving.value = true;
  try {
    await api.setMusicLibraries(music.value.trim(), musicVideo.value.trim());
    await appToast({ message: 'Music libraries saved.', duration: 1800 });
    emit('saved');
    emit('dismiss');
  } catch {
    await appToast({
      message: 'Could not save that. A folder path has no leading slash and no "..".',
      color: 'danger',
      duration: 2600,
    });
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-title>Music libraries</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="music-libs-close" @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <ion-note v-if="error" color="danger">{{ error }}</ion-note>

      <ion-list>
        <ion-item>
          <ion-input
            v-model="music"
            label="Music folder"
            label-placement="stacked"
            autocapitalize="off"
            autocorrect="off"
            :spellcheck="false"
            data-testid="music-libs-music"
            placeholder="music/Music"
          />
        </ion-item>
        <ion-item>
          <ion-input
            v-model="musicVideo"
            label="Music video folder"
            label-placement="stacked"
            autocapitalize="off"
            autocorrect="off"
            :spellcheck="false"
            data-testid="music-libs-video"
            placeholder="video/MusicVideos"
          />
        </ion-item>
      </ion-list>

      <ion-note class="help">
        These are folders on the NAS, without a leading slash. Point each one at the
        <strong>same library the YouTube downloads write to</strong>, so an uploaded track and a
        downloaded one sit together. Leave one empty to leave that upload option off.
      </ion-note>

      <ion-button
        expand="block"
        class="go"
        :disabled="loading || saving"
        data-testid="music-libs-save"
        @click="save"
      >
        Save
      </ion-button>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.help {
  display: block;
  margin: 0.75rem 0.25rem;
  font-size: 0.8rem;
  line-height: 1.4;
}
.go {
  margin-top: 1rem;
}
</style>
