<script setup lang="ts">
/**
 * Music library repair, run from Settings by an admin (spec 1053).
 *
 * The repair itself is spec 1052's tool. This is only the place an admin starts it,
 * watches it and reads the answer — instead of `kubectl`. Everything on this screen
 * is something the SERVER decoded and bounded from what the worker reported; the
 * server never mounts the library, so the complete plan stays on the share and the
 * screen says so.
 *
 * What may be done next (apply, continue, undo) is decided by the server and only
 * displayed here. The snapshot tick-box in the confirmation is a courtesy: the
 * server refuses an apply that does not carry it, whatever this screen did.
 */
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import {
  IonAccordion,
  IonAccordionGroup,
  IonAlert,
  IonBadge,
  IonButton,
  IonButtons,
  IonCheckbox,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonModal,
  IonNote,
  IonProgressBar,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from '@ionic/vue';
import { ApiError, api, type RepairRun, type RepairSnapshot } from '@/services/api';
import { createPollLoop } from '@/services/poll-loop';
import {
  busyMessage,
  checkRows,
  confirmLines,
  errorMessage,
  expiryText,
  formatCount,
  leftAloneView,
  offers,
  progressPercent,
  progressText,
  relativeTime,
  stateLabel,
  unavailableText,
} from '@/services/repair-state';
import { appToast } from '@/services/toast';

const props = defineProps<{ isOpen: boolean }>();
const emit = defineEmits<{ (e: 'dismiss'): void }>();

const snap = ref<RepairSnapshot | null>(null);
const loading = ref(false);
const failed = ref(false);
const working = ref(false);
const now = ref(Math.floor(Date.now() / 1000));

const confirmOpen = ref(false);
const confirmMode = ref<'apply' | 'continue'>('apply');
const snapshotAck = ref(false);
const undoOpen = ref(false);

const view = computed(() => (snap.value ? offers(snap.value, now.value) : null));
const plan = computed(() => snap.value?.plan ?? null);
const rows = computed(() => (plan.value?.summary?.check ? checkRows(plan.value.summary.check) : []));
const left = computed(() => (plan.value?.summary?.check ? leftAloneView(plan.value.summary.check.leftAlone) : null));
const percent = computed(() => progressPercent(snap.value?.current?.progress));
const latest = computed(() => snap.value?.latest ?? null);
// The latest run's details are worth a card only when they are about an apply or an
// undo: a check's answer is the plan card above it.
const outcome = computed(() => {
  const l = latest.value;
  if (!l?.summary || l.kind === 'check') return null;
  return l;
});

async function load(): Promise<void> {
  try {
    snap.value = await api.getMusicRepair();
    failed.value = false;
  } catch {
    failed.value = true;
  } finally {
    loading.value = false;
    now.value = Math.floor(Date.now() / 1000);
  }
}

// One request a few seconds apart while the screen is open. A repair is one run seen
// by one admin at a time, so the download list's live stream would be out of
// proportion; the server answers this cheaply and caches the progress reading.
const poll = createPollLoop(load, 3000);

watch(
  () => props.isOpen,
  (open) => {
    if (open) {
      loading.value = snap.value === null;
      void load();
      poll.start();
    } else {
      poll.stop();
      confirmOpen.value = false;
      undoOpen.value = false;
      snapshotAck.value = false;
    }
  },
  { immediate: true },
);
onBeforeUnmount(() => poll.stop());

async function refused(e: unknown): Promise<void> {
  const code = e instanceof ApiError ? e.code : 'start_failed';
  const message =
    code === 'busy' && e instanceof ApiError
      ? busyMessage(e.detail, Math.floor(Date.now() / 1000))
      : errorMessage(code);
  await appToast({ message, color: 'danger', duration: 3500 });
  await load();
}

async function startCheck(): Promise<void> {
  working.value = true;
  try {
    await api.startMusicRepairCheck();
    await load();
  } catch (e) {
    await refused(e);
  } finally {
    working.value = false;
  }
}

function askApply(mode: 'apply' | 'continue'): void {
  confirmMode.value = mode;
  snapshotAck.value = false; // never remembered: every apply is acknowledged afresh
  confirmOpen.value = true;
}

async function doApply(): Promise<void> {
  if (!plan.value || !snapshotAck.value) return;
  working.value = true;
  try {
    await api.applyMusicRepair(plan.value.id, snapshotAck.value);
    confirmOpen.value = false;
    snapshotAck.value = false;
    await load();
  } catch (e) {
    confirmOpen.value = false;
    await refused(e);
  } finally {
    working.value = false;
  }
}

async function doUndo(): Promise<void> {
  const target = snap.value?.undo;
  if (!target) return;
  working.value = true;
  try {
    await api.undoMusicRepair(target.planId);
    await load();
  } catch (e) {
    await refused(e);
  } finally {
    working.value = false;
  }
}

const KIND_LABEL: Record<RepairRun['kind'], string> = { check: 'Check', apply: 'Apply', undo: 'Undo' };
function stateColor(s: RepairRun['state']): string {
  return s === 'finished' ? 'success' : s === 'running' ? 'primary' : s === 'refused' ? 'warning' : 'medium';
}

const planNote = computed(() => {
  const p = plan.value;
  if (!p) return '';
  switch (p.status) {
    case 'applied':
      return 'This plan was applied.';
    case 'undone':
      return 'This plan was applied and then undone. Run a new check to repair again.';
    case 'expired':
      return 'This check is more than 24 hours old, so it can no longer be applied. Run it again.';
    case 'apply_unfinished':
      return 'Applying this plan did not finish. Continuing resumes where it stopped.';
    case 'applying':
      return 'This plan is being applied now.';
    default:
      return expiryText(p, now.value);
  }
});
</script>

<template>
  <ion-modal :is-open="isOpen" @did-dismiss="emit('dismiss')">
    <ion-header>
      <ion-toolbar>
        <ion-title>Music library repair</ion-title>
        <ion-buttons slot="end">
          <ion-button data-testid="repair-close" @click="emit('dismiss')">Close</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding" data-testid="repair-content">
      <div v-if="loading && !snap" class="center"><ion-spinner /></div>

      <ion-note v-else-if="failed && !snap" color="danger" data-testid="repair-error">
        Could not read the repair status. Check your connection and try again.
      </ion-note>

      <template v-else-if="snap && view">
        <!-- Cannot run here: say why and change nothing else. -->
        <ion-note v-if="view.mode === 'unavailable'" class="block" data-testid="repair-unavailable">
          {{ unavailableText(snap.reason) }}
        </ion-note>

        <!-- Running: progress, and that it survives closing this screen. -->
        <template v-else-if="view.mode === 'running' && snap.current">
          <ion-list inset lines="none" data-testid="repair-progress">
            <ion-list-header>
              <ion-label>{{ KIND_LABEL[snap.current.kind] }} in progress</ion-label>
            </ion-list-header>
            <ion-item>
              <ion-label class="ion-text-wrap">
                <h3>{{ progressText(snap.current.progress) }}</h3>
                <p>
                  Started by {{ snap.current.startedBy }}
                  {{ relativeTime(snap.current.startedAt, now) }}
                </p>
              </ion-label>
            </ion-item>
            <ion-item>
              <ion-progress-bar
                :type="percent === null ? 'indeterminate' : 'determinate'"
                :value="percent === null ? undefined : percent / 100"
              />
            </ion-item>
          </ion-list>
          <ion-note class="block">
            You can close this screen. The repair keeps running and this page will show
            where it got to when you come back.
          </ion-note>
        </template>

        <!-- Idle. -->
        <template v-else>
          <ion-list inset lines="none">
            <ion-list-header><ion-label>Check the library</ion-label></ion-list-header>
            <ion-item>
              <ion-label class="ion-text-wrap">
                <p>
                  Reads the music library and works out what a repair would change: one copy
                  of each song, playlists as files, clean names, and album details where a
                  public source is sure. A check changes nothing.
                </p>
              </ion-label>
            </ion-item>
            <ion-item lines="none">
              <ion-button
                expand="block"
                class="fill"
                :disabled="working || !view.canCheck"
                data-testid="repair-check"
                @click="startCheck"
              >
                Check library
              </ion-button>
            </ion-item>
          </ion-list>

          <!-- What the last check found, and what can be done with it. -->
          <template v-if="view.showPlan && plan">
            <ion-list inset data-testid="repair-plan" :data-status="plan.status">
              <ion-list-header>
                <ion-label>What a repair would do</ion-label>
              </ion-list-header>
              <ion-item lines="none">
                <ion-label class="ion-text-wrap">
                  <p>Checked {{ relativeTime(plan.checkedAt, now) }}. {{ planNote }}</p>
                </ion-label>
              </ion-item>
              <ion-item v-for="r in rows" :key="r.key" :data-testid="`repair-row-${r.key}`">
                <ion-label class="ion-text-wrap">
                  <h3>{{ r.label }}</h3>
                  <p v-if="r.hint" :class="{ warn: r.warn }">{{ r.hint }}</p>
                </ion-label>
                <ion-note slot="end" :color="r.warn ? 'danger' : undefined" class="value">
                  {{ r.value }}
                </ion-note>
              </ion-item>

              <ion-accordion-group v-if="left && left.total > 0" data-testid="repair-left-alone">
                <ion-accordion value="left">
                  <ion-item slot="header">
                    <ion-label>Left alone ({{ formatCount(left.total) }})</ion-label>
                  </ion-item>
                  <div slot="content" class="ion-padding">
                    <ul class="plain">
                      <li v-for="g in left.groups" :key="g.reason">
                        {{ formatCount(g.count) }} × {{ g.reason }}
                      </li>
                    </ul>
                    <ul v-if="left.examples.length" class="plain paths">
                      <li v-for="(x, i) in left.examples" :key="i">{{ x.path }}</li>
                    </ul>
                    <p v-if="left.hidden > 0" class="hint">
                      … and {{ formatCount(left.hidden) }} more. The complete list is in the plan.
                    </p>
                  </div>
                </ion-accordion>
              </ion-accordion-group>

              <ion-item lines="none">
                <ion-label class="ion-text-wrap">
                  <p class="hint" data-testid="repair-plan-file">
                    The complete plan is in the <strong>.repair</strong> folder of your music
                    library on the NAS<span v-if="plan.planFile"> ({{ plan.planFile }})</span>.
                  </p>
                </ion-label>
              </ion-item>
            </ion-list>

            <ion-button
              v-if="view.canApply"
              expand="block"
              class="fill"
              :disabled="working"
              data-testid="repair-apply"
              @click="askApply('apply')"
            >
              Apply this plan
            </ion-button>
            <ion-button
              v-if="view.canContinue"
              expand="block"
              class="fill"
              :disabled="working"
              data-testid="repair-continue"
              @click="askApply('continue')"
            >
              Continue applying
            </ion-button>
          </template>

          <!-- How the last apply or undo went. -->
          <ion-list v-if="outcome && outcome.summary" inset data-testid="repair-outcome">
            <ion-list-header>
              <ion-label>Last {{ outcome.kind === 'apply' ? 'apply' : 'undo' }}</ion-label>
            </ion-list-header>
            <ion-item v-if="outcome.summary.apply">
              <ion-label class="ion-text-wrap">
                <h3>
                  {{ formatCount(outcome.summary.apply.done) }} done,
                  {{ formatCount(outcome.summary.apply.skipped) }} skipped,
                  {{ formatCount(outcome.summary.apply.failed) }} failed
                </h3>
                <p v-for="r in outcome.summary.apply.skippedByReason" :key="r.reason">
                  {{ formatCount(r.count) }} skipped: {{ r.reason }}
                </p>
                <p v-for="(f, i) in outcome.summary.apply.failedExamples" :key="'f' + i" class="warn">
                  {{ f.path }} — {{ f.note }}
                </p>
              </ion-label>
            </ion-item>
            <ion-item v-else-if="outcome.summary.undo">
              <ion-label class="ion-text-wrap">
                <h3>
                  {{ formatCount(outcome.summary.undo.restored) }} restored,
                  {{ formatCount(outcome.summary.undo.skipped) }} skipped
                </h3>
                <p v-for="(f, i) in outcome.summary.undo.skippedExamples" :key="i">
                  {{ f.path }} — {{ f.note }}
                </p>
              </ion-label>
            </ion-item>
          </ion-list>

          <ion-button
            v-if="view.canUndo"
            expand="block"
            fill="outline"
            color="medium"
            class="fill"
            :disabled="working"
            data-testid="repair-undo"
            @click="undoOpen = true"
          >
            Undo the last repair
          </ion-button>
        </template>

        <!-- History: who ran what, and how it went. -->
        <ion-list v-if="snap.history.length" inset data-testid="repair-history">
          <ion-list-header><ion-label>History</ion-label></ion-list-header>
          <ion-item v-for="h in snap.history" :key="h.id" :data-testid="`repair-run-${h.kind}`">
            <ion-label class="ion-text-wrap">
              <h3>{{ KIND_LABEL[h.kind] }} · {{ h.startedBy }}</h3>
              <p>{{ relativeTime(h.startedAt, now) }}<span v-if="h.headline"> · {{ h.headline }}</span></p>
            </ion-label>
            <ion-badge slot="end" :color="stateColor(h.state)">{{ stateLabel(h.state) }}</ion-badge>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>

    <!-- The apply confirmation. -->
    <ion-modal
      :is-open="confirmOpen"
      :initial-breakpoint="0.75"
      :breakpoints="[0, 0.75, 1]"
      @did-dismiss="confirmOpen = false"
    >
      <ion-header>
        <ion-toolbar>
          <ion-title>{{ confirmMode === 'continue' ? 'Continue applying?' : 'Apply this plan?' }}</ion-title>
        </ion-toolbar>
      </ion-header>
      <ion-content class="ion-padding" data-testid="repair-confirm">
        <ul class="plain">
          <li v-for="(line, i) in plan?.summary?.check ? confirmLines(plan.summary.check) : []" :key="i">
            {{ line }}
          </li>
        </ul>
        <ion-item lines="none" class="ack">
          <ion-checkbox
            v-model="snapshotAck"
            label-placement="end"
            justify="start"
            data-testid="repair-ack"
          >
            <span class="ion-text-wrap">I have taken a snapshot of the music share</span>
          </ion-checkbox>
        </ion-item>
        <ion-note class="block">
          A snapshot protects you if something goes wrong that this repair cannot undo, such as
          losing the volume. Take one on the NAS first.
        </ion-note>
        <ion-button
          expand="block"
          class="fill"
          :disabled="!snapshotAck || working"
          data-testid="repair-apply-confirm"
          @click="doApply"
        >
          {{ confirmMode === 'continue' ? 'Continue' : 'Apply' }}
        </ion-button>
        <ion-button expand="block" fill="clear" data-testid="repair-apply-cancel" @click="confirmOpen = false">
          Cancel
        </ion-button>
      </ion-content>
    </ion-modal>

    <ion-alert
      :is-open="undoOpen"
      header="Undo the last repair?"
      message="Files and tags it changed are put back. Anything that has since moved into their old place is left alone and reported."
      :buttons="[
        { text: 'Cancel', role: 'cancel', handler: () => { undoOpen = false; } },
        { text: 'Undo', role: 'confirm', cssClass: 'repair-undo-confirm', handler: () => { undoOpen = false; void doUndo(); } },
      ]"
      @did-dismiss="undoOpen = false"
    />
  </ion-modal>
</template>

<style scoped>
.center {
  display: flex;
  justify-content: center;
  padding: 2rem 0;
}
.block {
  display: block;
  margin: 0.75rem 0.25rem;
  font-size: 0.8rem;
  line-height: 1.4;
}
.fill {
  margin: 0.75rem 0;
}
.value {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
}
.hint,
.warn {
  font-size: 0.8rem;
}
.warn {
  color: var(--ion-color-danger);
}
.plain {
  list-style: none;
  margin: 0 0 0.5rem;
  padding: 0;
  line-height: 1.5;
}
.paths {
  font-size: 0.8rem;
  opacity: 0.8;
  overflow-wrap: anywhere;
}
.ack {
  margin: 0.5rem 0;
}
</style>
