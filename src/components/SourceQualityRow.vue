<script setup lang="ts">
/**
 * One download option in the title sheet.
 *
 * Extracted so the same row can be rendered flat (a movie, or a source that
 * lists its releases without seasons) and inside a season group, without the
 * markup existing twice and drifting.
 *
 * The radio stays a plain descendant of the sheet's <ion-radio-group>, which is
 * how Ionic wires selection — so this component holds no selection state of its
 * own and emits nothing.
 */
import { IonBadge, IonItem, IonLabel, IonRadio } from '@ionic/vue';
import type { QualityOption } from '@/services/api';

defineProps<{
  option: QualityOption;
  /** Over the instance-wide download cap, so it cannot be sent. */
  tooLarge: boolean;
  /** Draw the season label inline — only when the rows are NOT already grouped. */
  showSeason?: boolean;
}>();
</script>

<template>
  <ion-item class="quality-row">
    <ion-radio :value="option.id" label-placement="end" justify="start">
      <ion-label>
        <h3>
          <span class="release">
            <span v-if="showSeason && option.season" class="season">{{ option.season }} · </span>
            {{ option.label }}
          </span>
          <!-- Badges never shrink and never wrap internally: a clipped "Have i…"
               is worse than a line break, and this row is read on a phone. -->
          <ion-badge v-if="tooLarge" color="warning" class="chip too-large">Too large</ion-badge>
          <ion-badge
            v-if="option.owned"
            color="success"
            class="chip have"
            data-testid="option-owned"
          >
            Have it
          </ion-badge>
        </h3>
        <p class="meta">
          {{ option.size }}<template v-if="option.resolution"> · {{ option.resolution }}</template
          >{{ option.encoder ? ' · ' + option.encoder : ''
          }}<template v-if="option.episodes"> · {{ option.episodes }} eps</template>
        </p>
      </ion-label>
    </ion-radio>
  </ion-item>
</template>

<style scoped>
/* The row has to stay readable at 360px, where it used to clip the badge and cut
   the line off. The title line wraps instead, and the badges are held whole. */
.quality-row h3 {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  white-space: normal;
}
.release {
  min-width: 0;
  overflow-wrap: anywhere;
}
.chip {
  flex: 0 0 auto;
  font-size: 0.7rem;
  white-space: nowrap;
}
.season {
  color: var(--ion-color-primary, #3dc2ff);
  font-weight: 600;
}
.meta {
  white-space: normal;
  overflow-wrap: anywhere;
}
/* Ionic clamps label text to one line by default; these rows are allowed to grow
   instead, which is the whole point of the change. */
.quality-row ion-label {
  white-space: normal;
}
</style>
