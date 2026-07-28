<script setup lang="ts">
/**
 * Update page (spec 1003). Shown when a new version is ready. It fetches the
 * incoming version + release notes from /v1/config (the server is already the
 * new version once deployed, so this is genuinely "what's about to update") and
 * offers a single OK that applies the update and reloads. Not dismissible — OK
 * is the only way forward.
 */
import {
  IonButton,
  IonContent,
  IonFooter,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { ref } from 'vue';
import { api } from '@/services/api';
import { whatsNew } from '@/services/release-notes';

defineProps<{ isOpen: boolean; applying?: boolean }>();
const emit = defineEmits<{ (e: 'confirm', version: string): void }>();

const loading = ref(true);
const version = ref('');
// Only the changes new since the running build, as plain-language lines.
const notes = ref<string[]>([]);

async function onPresent(): Promise<void> {
  loading.value = true;
  try {
    const c = await api.config();
    version.value = c.version;
    notes.value = whatsNew(c.releaseNotes ?? [], __RELEASE_NOTES__);
  } catch {
    version.value = '';
    notes.value = [];
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" :backdrop-dismiss="false" @will-present="onPresent">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Update available</ion-title>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true" class="ion-padding">
      <div v-if="loading" class="center"><ion-spinner name="crescent" /></div>
      <template v-else>
        <h2 class="lead" data-testid="update-version">SynoDL{{ version ? ` v${version}` : '' }} is ready</h2>
        <p class="sub">Here's what's new — tap OK to update now.</p>
        <ion-list v-if="notes.length" inset data-testid="update-notes">
          <ion-item v-for="(line, i) in notes" :key="i">
            <ion-label class="ion-text-wrap">{{ line }}</ion-label>
          </ion-item>
        </ion-list>
        <p v-else class="sub">Under-the-hood improvements and fixes.</p>
      </template>
    </ion-content>
    <ion-footer :translucent="true">
      <ion-toolbar>
        <ion-button
          expand="block"
          class="ok"
          :disabled="loading || applying"
          data-testid="update-ok"
          @click="emit('confirm', version)"
        >
          <ion-spinner v-if="applying" name="crescent" />
          <span v-else>OK</span>
        </ion-button>
      </ion-toolbar>
    </ion-footer>
  </ion-modal>
</template>

<style scoped>
.center {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
.lead {
  font-size: 1.2rem;
  font-weight: 700;
  margin: 0.5rem 0 0.25rem;
}
.sub {
  color: var(--app-text-dim);
  margin: 0 0 1rem;
}
.ok {
  margin: 0.4rem 0.6rem;
}
</style>
