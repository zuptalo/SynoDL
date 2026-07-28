<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonItem,
  IonLabel,
  IonList,
  IonModal,
  IonNote,
  IonRadio,
  IonRadioGroup,
  IonSpinner,
  IonThumbnail,
  IonTitle,
  IonToolbar,
  toastController,
} from '@ionic/vue';
import { cloudDownloadOutline } from 'ionicons/icons';
import { api, ApiError, type QualityOption } from '@/services/api';
import { useSourceCatalog } from '@/composables/useSourceCatalog';

const props = defineProps<{
  isOpen: boolean;
  titleId: string;
  titleName: string;
  posterUrl: string;
}>();
const emit = defineEmits<{
  (e: 'dismiss'): void;
  (e: 'needs-refresh'): void;
}>();

const { preferredQuality } = useSourceCatalog();

const loading = ref(false);
const sending = ref(false);
const sendable = ref(true);
const qualities = ref<QualityOption[]>([]);
const selected = ref('');
const errorMsg = ref('');

watch(
  () => props.isOpen,
  async (open) => {
    if (!open) return;
    loading.value = true;
    errorMsg.value = '';
    qualities.value = [];
    selected.value = '';
    try {
      const detail = await api.getSourceTitle(props.titleId);
      sendable.value = detail.sendable;
      qualities.value = detail.qualities;
      // Preselect the user's preferred quality when this title offers it,
      // otherwise the first available quality.
      const preferred = detail.qualities.find((q) =>
        q.label.toLowerCase().includes(preferredQuality.value.toLowerCase()),
      );
      selected.value = preferred?.id ?? detail.qualities[0]?.id ?? '';
    } catch (e) {
      if (e instanceof ApiError && e.code === 'source_needs_refresh') {
        emit('needs-refresh');
        emit('dismiss');
        return;
      }
      errorMsg.value = 'Could not load this title.';
    } finally {
      loading.value = false;
    }
  },
);

async function toast(message: string): Promise<void> {
  const t = await toastController.create({ message, duration: 2500, position: 'bottom' });
  await t.present();
}

async function send(): Promise<void> {
  if (!selected.value || sending.value) return;
  sending.value = true;
  errorMsg.value = '';
  try {
    const res = await api.sendSource(props.titleId, selected.value, props.titleName);
    await toast(`Sending to ${res.destination}`);
    emit('dismiss');
  } catch (e) {
    if (e instanceof ApiError) {
      if (e.code === 'source_needs_refresh') {
        emit('needs-refresh');
        emit('dismiss');
        return;
      }
      if (e.code === 'destination_forbidden') {
        errorMsg.value = "You can't download to that folder.";
      } else if (e.code === 'send_failed') {
        errorMsg.value = 'The download link could not be used. Try again.';
      } else {
        errorMsg.value = 'Could not send to the NAS.';
      }
    } else {
      errorMsg.value = 'Could not send to the NAS.';
    }
  } finally {
    sending.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title class="ion-text-nowrap">{{ titleName }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <div v-if="loading" class="centered"><ion-spinner /></div>

      <template v-else>
        <div class="poster-row" v-if="posterUrl">
          <ion-thumbnail class="poster"><img :src="posterUrl" :alt="titleName" /></ion-thumbnail>
          <div>
            <h2>{{ titleName }}</h2>
            <ion-note v-if="sendable">Choose a quality to send to your NAS.</ion-note>
          </div>
        </div>

        <ion-note v-if="!sendable" color="medium" class="unavailable">
          Sending isn't available for this title yet — series and anime are browse-only for now.
        </ion-note>

        <ion-radio-group v-else v-model="selected">
          <ion-list :inset="true">
            <ion-item v-for="q in qualities" :key="q.id">
              <ion-radio :value="q.id" label-placement="end" justify="start">
                <ion-label>
                  <h3>{{ q.label }}</h3>
                  <p>{{ q.size }} · {{ q.resolution }}{{ q.encoder ? ' · ' + q.encoder : '' }}</p>
                </ion-label>
              </ion-radio>
            </ion-item>
          </ion-list>
        </ion-radio-group>

        <ion-note v-if="errorMsg" color="danger" class="error">{{ errorMsg }}</ion-note>

        <ion-button
          v-if="sendable"
          expand="block"
          class="send-btn"
          :disabled="!selected || sending"
          @click="send"
        >
          <ion-spinner v-if="sending" slot="start" name="crescent" />
          <ion-icon v-else slot="start" :icon="cloudDownloadOutline" />
          Send to NAS
        </ion-button>
      </template>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.centered {
  display: flex;
  justify-content: center;
  padding-top: 30vh;
}
.poster-row {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 8px;
}
.poster {
  --size: 84px;
  flex: 0 0 auto;
}
.poster img {
  object-fit: cover;
  border-radius: 8px;
}
.poster-row h2 {
  margin: 0 0 4px;
  font-size: 1.1rem;
}
.unavailable {
  display: block;
  padding: 24px 8px;
  text-align: center;
}
.error {
  display: block;
  margin: 8px 4px;
}
.send-btn {
  margin-top: 16px;
}
</style>
