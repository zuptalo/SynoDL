<script setup lang="ts">
/**
 * Download statistics (spec 0006). Per-category counts + average sizes and a
 * historical downloads graph, filterable by source (catalog / direct / all) and
 * bucket (day / week / month / year / all-time). A regular user sees only their
 * own data; an admin gets a user picker (any user, or all combined). Built from
 * stock Ionic controls around one bespoke SVG chart (DownloadsChart).
 */
import { computed, onMounted, ref, watch } from 'vue';
import {
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonNote,
  IonSegment,
  IonSegmentButton,
  IonSelect,
  IonSelectOption,
  IonSpinner,
} from '@ionic/vue';
import { avgSize, type StatCategory, type StatSource } from '@/services/api';
import { bucketize, type Bucket } from '@/services/stats-buckets';
import { formatBytes } from '@/utils/format';
import { useStatistics } from '@/composables/useStatistics';
import DownloadsChart from '@/components/DownloadsChart.vue';

const props = defineProps<{ isAdmin: boolean }>();

const { summary, days, loadingSummary, loadSummary, loadTimeseries } = useStatistics();

const source = ref<StatSource>('all');
const bucket = ref<Bucket>('month');
const selectedUser = ref<number | 'all'>('all'); // admin only; ignored for regular users

const CATEGORIES: { key: StatCategory; label: string }[] = [
  { key: 'movie', label: 'Movies' },
  { key: 'series', label: 'TV series' },
  { key: 'anime', label: 'Anime' },
  { key: 'musicVideo', label: 'Music videos' },
  { key: 'music', label: 'Music' },
  { key: 'other', label: 'Other' },
];

type Agg = { count: number; completed: number; sumBytes: number };
const zero = (): Agg => ({ count: 0, completed: 0, sumBytes: 0 });

// The users whose data is currently shown: one for a regular user, the picked
// user for an admin, or everyone when "all".
const shownUsers = computed(() => {
  if (!props.isAdmin || selectedUser.value === 'all') return summary.value;
  return summary.value.filter((u) => u.userId === selectedUser.value);
});

// Per-category aggregate across the shown users and the selected source(s).
const perCategory = computed<Record<StatCategory, Agg>>(() => {
  const out = Object.fromEntries(CATEGORIES.map((c) => [c.key, zero()])) as Record<StatCategory, Agg>;
  const sources = source.value === 'all' ? (['catalog', 'direct'] as const) : [source.value];
  for (const u of shownUsers.value) {
    for (const src of sources) {
      const bucketStats = u.bySource[src];
      if (!bucketStats) continue;
      for (const c of CATEGORIES) {
        const s = bucketStats[c.key];
        if (!s) continue;
        out[c.key].count += s.count;
        out[c.key].completed += s.completed;
        out[c.key].sumBytes += s.sumBytes;
      }
    }
  }
  return out;
});

const overall = computed<Agg>(() => {
  const o = zero();
  for (const c of CATEGORIES) {
    o.count += perCategory.value[c.key].count;
    o.completed += perCategory.value[c.key].completed;
    o.sumBytes += perCategory.value[c.key].sumBytes;
  }
  return o;
});

const points = computed(() => bucketize(days.value, bucket.value));

function fmtAvg(a: Agg): string {
  const avg = avgSize(a);
  return avg === null ? '—' : formatBytes(avg);
}

async function refreshSeries(): Promise<void> {
  await loadTimeseries({
    source: source.value,
    userId: props.isAdmin ? selectedUser.value : undefined,
  });
}

onMounted(async () => {
  await loadSummary();
  await refreshSeries();
});

// Source or selected-user change needs fresh daily counts; bucket change is
// pure client-side re-aggregation (no refetch).
watch([source, selectedUser], refreshSeries);
</script>

<template>
  <div class="settings-cards">
    <div v-if="loadingSummary && !summary.length" class="loading">
      <ion-spinner name="crescent" />
    </div>

    <template v-else>
      <!-- Filters card: the user picker (admin) and the source segment, grouped in
           one inset card and wrapped in items so the segment sits in the card like
           the Settings screen's segments (not floating between cards). -->
      <ion-list inset>
        <ion-item v-if="isAdmin">
          <ion-select
            v-model="selectedUser"
            label="User"
            interface="popover"
            data-testid="stats-user"
          >
            <ion-select-option :value="'all'">All users</ion-select-option>
            <ion-select-option v-for="u in summary" :key="u.userId" :value="u.userId">
              {{ u.username }}
            </ion-select-option>
          </ion-select>
        </ion-item>
        <ion-item lines="none">
          <ion-segment v-model="source" class="seg" data-testid="stats-source">
            <ion-segment-button value="all"><ion-label>All sources</ion-label></ion-segment-button>
            <ion-segment-button value="catalog"><ion-label>Discover</ion-label></ion-segment-button>
            <ion-segment-button value="direct"><ion-label>Direct</ion-label></ion-segment-button>
          </ion-segment>
        </ion-item>
      </ion-list>

      <!-- Per-category counts + average sizes. -->
      <ion-list inset>
        <ion-list-header>Downloads by type</ion-list-header>
        <ion-item v-for="c in CATEGORIES" :key="c.key" :data-testid="`stats-cat-${c.key}`">
          <ion-label>{{ c.label }}</ion-label>
          <ion-note slot="end" class="stat">
            <span class="count">{{ perCategory[c.key].count }}</span>
            <span class="avg">avg {{ fmtAvg(perCategory[c.key]) }}</span>
          </ion-note>
        </ion-item>
        <ion-item lines="none">
          <ion-label><strong>Overall</strong></ion-label>
          <ion-note slot="end" class="stat">
            <span class="count">{{ overall.count }}</span>
            <span class="avg">avg {{ fmtAvg(overall) }}</span>
          </ion-note>
        </ion-item>
      </ion-list>

      <!-- Historical graph with client-side bucket switching. -->
      <ion-list inset>
        <ion-list-header>History</ion-list-header>
        <!-- Headline total for the current user + source selection, so the size of
             what's being charted is legible at a glance (spec 1017). -->
        <div class="range-total" data-testid="stats-total">
          <span class="rt-count">{{ overall.count }}</span>
          <span class="rt-label">
            {{ overall.count === 1 ? 'download' : 'downloads' }} ·
            {{ overall.sumBytes ? formatBytes(overall.sumBytes) : '—' }} total
          </span>
        </div>
        <ion-item lines="none">
          <ion-segment v-model="bucket" class="seg" data-testid="stats-bucket">
            <ion-segment-button value="day"><ion-label>Day</ion-label></ion-segment-button>
            <ion-segment-button value="week"><ion-label>Week</ion-label></ion-segment-button>
            <ion-segment-button value="month"><ion-label>Month</ion-label></ion-segment-button>
            <ion-segment-button value="year"><ion-label>Year</ion-label></ion-segment-button>
            <ion-segment-button value="all"><ion-label>All</ion-label></ion-segment-button>
          </ion-segment>
        </ion-item>
        <div class="chart-wrap">
          <DownloadsChart :points="points" />
        </div>
      </ion-list>
    </template>
  </div>
</template>

<style scoped>
.loading {
  display: flex;
  justify-content: center;
  padding: 2rem;
}
.stat {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  line-height: 1.2;
}
.stat .count {
  font-weight: 600;
  color: var(--ion-text-color);
}
.stat .avg {
  font-size: 0.8rem;
  color: var(--ion-color-medium);
}
.range-total {
  display: flex;
  align-items: baseline;
  gap: 0.5rem;
  padding: 0.1rem 1rem 0.4rem;
}
.range-total .rt-count {
  font-size: 1.7rem;
  font-weight: 700;
  color: var(--ion-text-color);
  line-height: 1;
}
.range-total .rt-label {
  font-size: 0.85rem;
  color: var(--ion-color-medium);
}
.chart-wrap {
  padding: 0.5rem 1rem 1rem;
  overflow-x: auto;
}
/* Segments fill their card item (like the Settings "Open to" segment) rather than
   floating between cards. */
.seg {
  width: 100%;
}
</style>
