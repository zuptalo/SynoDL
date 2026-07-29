<script setup lang="ts">
/**
 * Hand-rolled SVG bar chart for the download-history graph (spec 0006). Ionic
 * has no chart primitive and the project keeps its dependency surface minimal,
 * so this is a small bespoke SVG — styled with the app's theme tokens (so it's
 * correct in light/dark) and paired with an accessible text summary. It renders
 * whatever bucketed series it's handed; the parent does the bucketing.
 */
import { computed } from 'vue';
import type { BucketPoint } from '@/services/stats-buckets';

const props = defineProps<{ points: BucketPoint[] }>();

// Viewbox in abstract units; the SVG scales responsively to its container.
const W = 320;
const H = 120;
const pad = { top: 8, right: 4, bottom: 18, left: 4 };

const max = computed(() => Math.max(1, ...props.points.map((p) => p.count)));
const total = computed(() => props.points.reduce((s, p) => s + p.count, 0));

// Bar geometry. Bars share the plot width with a small gap; height maps count→px.
const bars = computed(() => {
  const n = props.points.length || 1;
  const plotW = W - pad.left - pad.right;
  const plotH = H - pad.top - pad.bottom;
  const bw = plotW / n;
  return props.points.map((p, i) => {
    const h = (p.count / max.value) * plotH;
    return {
      x: pad.left + i * bw + bw * 0.12,
      y: pad.top + (plotH - h),
      w: bw * 0.76,
      h,
      point: p,
      // Label only a few ticks so the axis never crowds.
      showLabel: n <= 12 || i % Math.ceil(n / 8) === 0,
      cx: pad.left + i * bw + bw / 2,
    };
  });
});
</script>

<template>
  <div class="chart">
    <svg
      :viewBox="`0 0 ${W} ${H}`"
      preserveAspectRatio="none"
      role="img"
      :aria-label="`Downloads over time, ${total} total across ${points.length} periods`"
    >
      <g v-for="b in bars" :key="b.point.key">
        <rect :x="b.x" :y="b.y" :width="b.w" :height="b.h" rx="1.5" class="bar">
          <title>{{ b.point.label }}: {{ b.point.count }}</title>
        </rect>
        <text v-if="b.showLabel" :x="b.cx" :y="H - 6" text-anchor="middle" class="tick">
          {{ b.point.label }}
        </text>
      </g>
    </svg>
    <!-- Screen-reader / empty-state text summary alongside the visual. -->
    <p v-if="!total" class="empty">No downloads in this range yet.</p>
  </div>
</template>

<style scoped>
.chart {
  width: 100%;
}
svg {
  width: 100%;
  height: 140px;
  display: block;
}
.bar {
  fill: var(--app-accent, var(--ion-color-primary));
}
.tick {
  fill: var(--ion-color-medium);
  font-size: 6px;
}
.empty {
  text-align: center;
  color: var(--ion-color-medium);
  font-size: 0.85rem;
  margin: 0.5rem 0 0;
}
</style>
