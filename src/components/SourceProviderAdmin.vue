<script setup lang="ts">
/**
 * Admin panel for the download sources (spec 0007).
 *
 * This manages a LIST of sources rather than one, and the paste form for each is
 * generated from the fields that source's driver declares — so adding a driver
 * on the server needs no change here.
 *
 * Session material is write-only throughout: the server never returns a stored
 * value, so secret inputs start blank and a blank input means "keep what is
 * stored" rather than "clear it".
 */
import { computed, ref, watch } from 'vue';
import {
  IonButton,
  IonButtons,
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
  IonSelect,
  IonSelectOption,
  IonSpinner,
  IonTextarea,
  IonTitle,
  IonToggle,
  IonToolbar,
} from '@ionic/vue';
import { addOutline, trashOutline } from 'ionicons/icons';
import {
  api,
  ApiError,
  type SourceKind,
  type SourceProvider,
  type SourceSessionField,
} from '@/services/api';
import { appToast } from '@/services/toast';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void }>();

const providers = ref<SourceProvider[]>([]);
const kinds = ref<SourceKind[]>([]);
const loading = ref(false);
const saving = ref(false);
const errorMsg = ref('');
const maxSizeGB = ref('');
const savingPolicy = ref(false);

// Which source is open in the editor. null = the list; 'new' = the add form.
const editing = ref<number | 'new' | null>(null);

// Editor fields.
const formKind = ref('');
const displayName = ref('');
const moviesParent = ref('');
const tvParent = ref('');
const enabled = ref(true);
const altBase = ref('');
const sessionValues = ref<Record<string, string>>({});
const userAgent = ref('');

const editingProvider = computed(() =>
  typeof editing.value === 'number' ? providers.value.find((p) => p.id === editing.value) : undefined,
);
const isNew = computed(() => editing.value === 'new');
const currentKind = computed(() => kinds.value.find((k) => k.kind === formKind.value));
const sessionFields = computed<SourceSessionField[]>(() => currentKind.value?.sessionFields ?? []);
// A stored source keeps its secrets when a field is left blank.
const secretPlaceholder = computed(() => (isNew.value ? '' : 'Stored — leave blank to keep'));

watch(
  () => props.isOpen,
  async (open) => {
    if (!open) return;
    errorMsg.value = '';
    editing.value = null;
    await refresh();
  },
);

async function refresh(): Promise<void> {
  loading.value = true;
  try {
    const list = await api.listSourceProviders();
    providers.value = list.providers;
    kinds.value = list.kinds;
    const status = await api.getSourceStatus();
    maxSizeGB.value = status.maxDownloadMB ? String(+(status.maxDownloadMB / 1024).toFixed(1)) : '';
  } catch {
    providers.value = [];
    kinds.value = [];
  } finally {
    loading.value = false;
  }
}

function stateLabel(p: SourceProvider): string {
  if (p.state === 'needs_refresh') return 'Needs signing in again';
  // Distinct from "needs refreshing" on purpose: the session is fine, the
  // account just can't download. Telling the operator to re-paste would send
  // them in circles.
  if (p.state === 'unsubscribed') return 'No active subscription';
  if (p.state === 'active') return p.enabled ? 'Active' : 'Disabled';
  return p.state;
}

function stateColor(p: SourceProvider): string {
  if (p.state === 'active' && p.enabled) return 'success';
  if (p.state === 'unsubscribed') return 'warning';
  if (p.state === 'needs_refresh') return 'warning';
  return 'medium';
}

function openNew(): void {
  errorMsg.value = '';
  editing.value = 'new';
  formKind.value = kinds.value[0]?.kind ?? '';
  displayName.value = currentKind.value?.name ?? '';
  moviesParent.value = '';
  tvParent.value = '';
  enabled.value = true;
  // Start from the mirror SynoDL knows about, so the common case needs no
  // research — the operator only edits it when the site changes address.
  altBase.value = currentKind.value?.defaultAltBase ?? '';
  sessionValues.value = {};
  userAgent.value = '';
}

function openEdit(p: SourceProvider): void {
  errorMsg.value = '';
  editing.value = p.id;
  formKind.value = p.kind;
  displayName.value = p.displayName;
  moviesParent.value = p.moviesParent;
  tvParent.value = p.tvParent;
  enabled.value = p.enabled;
  altBase.value = p.altBase ?? '';
  // Never prefilled — the server does not return stored session material.
  sessionValues.value = {};
  userAgent.value = '';
}

// Picking a different kind while adding swaps the whole form, since the fields
// come from the driver.
watch(formKind, () => {
  if (!isNew.value) return;
  displayName.value = currentKind.value?.name ?? '';
  altBase.value = currentKind.value?.defaultAltBase ?? '';
  sessionValues.value = {};
});

async function toast(message: string): Promise<void> {
  await appToast({ message, duration: 2500 });
}

function verifyMessage(reason: string | undefined): string {
  switch (reason) {
    case 'unsubscribed':
      return 'Signed in, but that account has no active subscription — so there is nothing it can download.';
    case 'invalid_token':
    case 'needs_refresh':
      return 'That session did not work. Capture it again from a browser where you are signed in.';
    case 'challenge':
      return 'The site challenged the request. Capture a fresh session and try again.';
    default:
      return 'Could not reach the source. Check that it is online and try again.';
  }
}

async function save(): Promise<void> {
  errorMsg.value = '';
  if (!moviesParent.value.trim()) {
    errorMsg.value = 'A movies folder is required.';
    return;
  }
  if (isNew.value) {
    const missing = sessionFields.value.filter((f) => f.required && !sessionValues.value[f.key]?.trim());
    if (missing.length) {
      errorMsg.value = `${missing.map((f) => f.label).join(' and ')} required.`;
      return;
    }
  }
  saving.value = true;
  try {
    const session: Record<string, string> = { user_agent: userAgent.value.trim() };
    for (const f of sessionFields.value) {
      session[f.key] = (sessionValues.value[f.key] ?? '').trim();
    }
    const input = {
      kind: formKind.value,
      displayName: displayName.value.trim(),
      moviesParent: moviesParent.value.trim(),
      tvParent: tvParent.value.trim(),
      altBase: altBase.value.trim(),
      enabled: enabled.value,
      session,
    };
    if (isNew.value) await api.createSourceProvider(input);
    else await api.updateSourceProvider(editing.value as number, input);
    await toast('Source verified and saved.');
    // Drop the pasted secrets from memory as soon as they are accepted.
    sessionValues.value = {};
    userAgent.value = '';
    editing.value = null;
    await refresh();
  } catch (e) {
    if (e instanceof ApiError && e.code === 'verify_failed') {
      errorMsg.value = verifyMessage(e.reason);
    } else if (e instanceof ApiError && e.code === 'bad_alt_base') {
      errorMsg.value = e.reason ?? 'That alternate address is not usable.';
    } else if (e instanceof ApiError && e.code === 'unknown_provider') {
      errorMsg.value = 'That source type is not supported.';
    } else {
      errorMsg.value = 'Could not save the source.';
    }
  } finally {
    saving.value = false;
  }
}

async function remove(p: SourceProvider): Promise<void> {
  saving.value = true;
  errorMsg.value = '';
  try {
    await api.deleteSourceProvider(p.id);
    await toast(`${p.displayName} removed.`);
    editing.value = null;
    await refresh();
  } catch {
    errorMsg.value = 'Could not remove the source.';
  } finally {
    saving.value = false;
  }
}

async function saveMaxSize(): Promise<void> {
  savingPolicy.value = true;
  errorMsg.value = '';
  try {
    const gb = parseFloat(maxSizeGB.value) || 0;
    await api.setSourcePolicy(Math.round(gb * 1024));
    await toast(gb > 0 ? `Max download size set to ${gb} GB.` : 'Download size limit removed.');
    await refresh();
  } catch {
    errorMsg.value = 'Could not save the size limit.';
  } finally {
    savingPolicy.value = false;
  }
}
</script>

<template>
  <ion-modal :is-open="isOpen" @didDismiss="emit('dismiss')">
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-title>Download sources</ion-title>
        <ion-buttons slot="end">
          <ion-button v-if="editing !== null" @click="editing = null">Back</ion-button>
          <ion-button v-else @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <div v-if="loading" class="centered"><ion-spinner /></div>

      <!-- The list of configured sources. -->
      <template v-else-if="editing === null">
        <ion-list :inset="true">
          <ion-list-header><ion-label>Sources</ion-label></ion-list-header>
          <ion-item v-if="providers.length === 0">
            <ion-label>
              <p>No sources configured yet.</p>
            </ion-label>
          </ion-item>
          <ion-item v-for="p in providers" :key="p.id" button @click="openEdit(p)">
            <ion-label>
              <h3>{{ p.displayName }}</h3>
              <p>{{ p.kind }}</p>
            </ion-label>
            <ion-note slot="end" :color="stateColor(p)">{{ stateLabel(p) }}</ion-note>
          </ion-item>
          <ion-item button :disabled="kinds.length === 0" @click="openNew()">
            <ion-icon slot="start" :icon="addOutline" />
            <ion-label>Add a source</ion-label>
          </ion-item>
        </ion-list>

        <ion-list :inset="true">
          <ion-list-header><ion-label>Limits</ion-label></ion-list-header>
          <ion-item>
            <ion-input
              v-model="maxSizeGB"
              label="Max download size (GB)"
              label-placement="stacked"
              placeholder="e.g. 10"
              inputmode="decimal"
            />
          </ion-item>
          <ion-item>
            <ion-button :disabled="savingPolicy" @click="saveMaxSize()">Save limit</ion-button>
          </ion-item>
          <ion-note class="hint">
            Applies to every source. Leave blank for no limit.
          </ion-note>
        </ion-list>
      </template>

      <!-- Add / edit one source. -->
      <template v-else>
        <ion-list :inset="true">
          <ion-list-header>
            <ion-label>{{ isNew ? 'Add a source' : editingProvider?.displayName }}</ion-label>
          </ion-list-header>

          <ion-item v-if="isNew">
            <ion-select v-model="formKind" label="Source type" label-placement="stacked" interface="popover">
              <ion-select-option v-for="k in kinds" :key="k.kind" :value="k.kind">
                {{ k.name }}
              </ion-select-option>
            </ion-select>
          </ion-item>

          <ion-item>
            <ion-input v-model="displayName" label="Display name" label-placement="stacked" />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="moviesParent"
              label="Movies parent"
              label-placement="stacked"
              placeholder="e.g. video/movies"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="tvParent"
              label="TV / series parent"
              label-placement="stacked"
              placeholder="e.g. video/tv"
            />
          </ion-item>
          <ion-item>
            <ion-input
              v-model="altBase"
              label="Alternate address (optional)"
              label-placement="stacked"
              placeholder="https://example.com"
            />
          </ion-item>
          <ion-note class="hint">
            Used only when the main site is unreachable — these sites are blocked
            periodically and publish a mirror. Your saved sign-in for this source will be
            sent to this address too, so only enter one the site itself published.
          </ion-note>
          <ion-item v-if="!isNew">
            <ion-toggle v-model="enabled">Enabled</ion-toggle>
          </ion-item>
        </ion-list>

        <!-- Generated from the driver's declared fields, so a new source type
             needs no change here. -->
        <ion-list :inset="true">
          <ion-list-header><ion-label>Session</ion-label></ion-list-header>
          <template v-for="f in sessionFields" :key="f.key">
            <ion-item>
              <ion-textarea
                v-model="sessionValues[f.key]"
                :label="f.label"
                label-placement="stacked"
                :placeholder="f.secret ? secretPlaceholder : ''"
                :auto-grow="true"
                :rows="1"
              />
            </ion-item>
            <ion-note v-if="f.help" class="hint">{{ f.help }}</ion-note>
          </template>
          <ion-item>
            <ion-textarea
              v-model="userAgent"
              label="User-Agent"
              label-placement="stacked"
              :placeholder="secretPlaceholder"
              :auto-grow="true"
              :rows="1"
            />
          </ion-item>
          <ion-note class="hint">
            Captured from a browser where you are already signed in. Values are stored
            encrypted and are never shown again — leave a field blank to keep what is stored.
          </ion-note>
        </ion-list>

        <ion-note v-if="errorMsg" color="danger" class="error">{{ errorMsg }}</ion-note>

        <div class="actions">
          <ion-button :disabled="saving" @click="save()">
            {{ isNew ? 'Verify and add' : 'Verify and save' }}
          </ion-button>
          <ion-button
            v-if="!isNew && editingProvider"
            color="danger"
            fill="outline"
            :disabled="saving"
            @click="remove(editingProvider)"
          >
            <ion-icon slot="start" :icon="trashOutline" />
            Remove
          </ion-button>
        </div>
      </template>

      <ion-note v-if="errorMsg && editing === null" color="danger" class="error">{{ errorMsg }}</ion-note>
    </ion-content>
  </ion-modal>
</template>

<style scoped>
.centered {
  display: flex;
  justify-content: center;
  padding: 2rem 0;
}
.hint {
  display: block;
  padding: 0 1rem 0.75rem;
  font-size: 0.78rem;
  line-height: 1.35;
}
.error {
  display: block;
  padding: 0.5rem 1rem;
}
.actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  padding: 0 1rem 1rem;
}
</style>
