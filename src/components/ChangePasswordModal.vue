<script setup lang="ts">
/**
 * Change the signed-in account's own SynoDL password (spec 1002). Uses the
 * existing admin user-update endpoint against the caller's own id. Requires a
 * new password of at least 8 characters and a matching confirmation.
 */
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
import { computed, ref } from 'vue';
import { api } from '@/services/api';
import { messageForError } from '@/services/syno-errors';

const props = defineProps<{ isOpen: boolean; userId: number }>();
const emit = defineEmits<{ (e: 'dismiss'): void; (e: 'saved'): void }>();

const pw = ref('');
const confirm = ref('');
const busy = ref(false);
const error = ref('');

const valid = computed(() => pw.value.length >= 8 && pw.value === confirm.value);

function reset(): void {
  pw.value = '';
  confirm.value = '';
  error.value = '';
}

async function onSave(): Promise<void> {
  if (!valid.value) return;
  busy.value = true;
  error.value = '';
  try {
    await api.updateUser(props.userId, { password: pw.value });
    emit('saved');
    emit('dismiss');
  } catch (e) {
    error.value = messageForError(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @will-present="reset" @did-dismiss="$emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Change password</ion-title>
        <ion-buttons slot="start">
          <ion-button data-testid="pw-cancel" @click="$emit('dismiss')">Cancel</ion-button>
        </ion-buttons>
        <ion-buttons slot="end">
          <ion-button :disabled="!valid || busy" data-testid="pw-save" @click="onSave">Save</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content :fullscreen="true" class="ion-padding">
      <ion-list inset>
        <ion-item>
          <ion-input
            label="New password"
            label-placement="stacked"
            type="password"
            placeholder="At least 8 characters"
            :value="pw"
            data-testid="pw-new"
            @ionInput="pw = String($event.target.value ?? '')"
          />
        </ion-item>
        <ion-item>
          <ion-input
            label="Confirm password"
            label-placement="stacked"
            type="password"
            :value="confirm"
            data-testid="pw-confirm"
            @ionInput="confirm = String($event.target.value ?? '')"
          />
        </ion-item>
      </ion-list>
      <ion-note v-if="confirm && pw !== confirm" color="danger" class="hint">Passwords don't match.</ion-note>
      <ion-note v-if="error" color="danger" class="hint" data-testid="pw-error">{{ error }}</ion-note>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.hint {
  display: block;
  margin: 0 1.5rem;
  font-size: 0.85rem;
}
</style>
