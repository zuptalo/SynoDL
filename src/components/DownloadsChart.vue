<script setup lang="ts">
/**
 * Download-history bar chart (spec 0006, redesigned in spec 1017). A small,
 * dependency-free CSS/flex bar chart (Ionic has no chart primitive and the
 * project keeps its dependency surface minimal). Bars are bounded in width so a
 * single bucket reads as one normal bar instead of a stretched block; values are
 * labelled directly when few and via a hover title otherwise, with a baseline and
 * a max reference for scale. Themed with app tokens so it's correct light/dark.
 */
import { computed } from 'vue';
import type { BucketPoint } from '@/services/stats-buckets';

const props = defineProps<{ points: BucketPoint[] }>();

const max = computed(() => Math.max(1, ...props.points.map((p) => p.count)));
const total = computed(() => props.points.reduce((s, p) => s + p.count, 0));

const bars = computed(() => {
  const n = props.points.length;
  // Thin the x-labels (and value labels) so a long series doesn't crowd; hover
  // still shows every bar's exact value.
  const labelEvery = n <= 8 ? 1 : Math.ceil(n / 6);
  return props.points.map((p, i) => ({
    ...p,
    // Cap bar height at 88% of the plot so the value label above it has headroom.
    pct: (p.count / max.value) * 88,
    showVal: n <= 16,
    showLabel: i % labelEvery === 0,
  }));
});
</script>

<template>
  <div v-if="total" class="chart">
    <div class="scale">max {{ max }}</div>
    <div
      class="bars"
      role="img"
      :aria-label="`Downloads over time, ${total} total across ${points.length} periods`"
    >
      <div
        v-for="b in bars"
        :key="b.key"
        class="col"
        :title="`${b.label}: ${b.count}`"
        data-testid="chart-bar"
      >
        <span v-if="b.showVal" class="val">{{ b.count }}</span>
        <div class="bar" :style="{ height: `${b.pct}%` }" />
      </div>
    </div>
    <div class="xaxis" aria-hidden="true">
      <div v-for="b in bars" :key="b.key" class="xcol">
        <span v-if="b.showLabel">{{ b.label }}</span>
      </div>
    </div>
  </div>
  <p v-else class="empty">No downloads in this range yet.</p>
</template>

<style scoped>
.chart {
  width: 100%;
  /* Few bars spread to fill the width (flex:1); many bars hold their min-width and
     overflow, so the parent .chart-wrap scrolls horizontally instead of crushing. */
}
.scale {
  font-size: 0.7rem;
  color: var(--ion-color-medium);
  text-align: right;
  margin-bottom: 2px;
}
.bars {
  display: flex;
  align-items: flex-end;
  gap: 6px;
  height: 150px;
  border-bottom: 1px solid var(--app-border, rgba(var(--ion-text-color-rgb, 0, 0, 0), 0.12));
}
.col {
  flex: 1 1 0;
  min-width: 28px;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: flex-end;
}
.val {
  font-size: 0.7rem;
  font-weight: 600;
  color: var(--ion-text-color);
  margin-bottom: 3px;
}
.bar {
  width: 62%;
  max-width: 34px;
  min-height: 3px;
  border-radius: 5px 5px 0 0;
  background: var(--app-accent, var(--ion-color-primary));
}
.xaxis {
  display: flex;
  gap: 6px;
  margin-top: 5px;
}
.xcol {
  flex: 1 1 0;
  min-width: 28px;
  text-align: center;
  font-size: 0.68rem;
  color: var(--ion-color-medium);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.empty {
  text-align: center;
  color: var(--ion-color-medium);
  font-size: 0.85rem;
  margin: 1rem 0;
}
</style>
