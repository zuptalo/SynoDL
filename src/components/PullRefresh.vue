<script setup lang="ts">
/**
 * Pull to refresh, drawn as one shape.
 *
 * A dot fades in as the pull begins, is drawn out into a tall narrow droplet as
 * it deepens, and pinches off into a ring at the threshold — so the thing that
 * ends up spinning is the thing that appeared under the finger, in the place it
 * appeared. Green dashed stroke throughout: the shape is the only thing that
 * changes.
 *
 * It lives INSIDE the refresher rather than floating over the content, and that
 * is the point of the layout: Ionic holds the list down for exactly as long as
 * the refresh runs, so the ring sits in space made for it instead of covering
 * the rows underneath. `pull-min` is raised to fit it.
 *
 * There is deliberately NO <ion-refresher-content>, and that is load-bearing
 * rather than tidy. Ionic chooses between a native refresher and its own JS one
 * by inspecting that element's spinners — an ASYNC check it re-runs on every
 * state change, so on the first pull it could still be making up its mind, tear
 * one implementation down and stand the other up mid-gesture. That is what made
 * the content snap back and the animation restart, exactly once, early in the
 * first pull. With no content element the check returns false immediately and
 * synchronously, so the JS refresher is the only one there ever is.
 *
 * There is no success state. Finishing IS the animation going away — a tick that
 * appears for a moment and then leaves says the same thing twice, and the second
 * time is noise. A failure is the page's to report, where it can say what went
 * wrong rather than showing a mark and clearing it.
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { IonRefresher, type RefresherCustomEvent } from '@ionic/vue';
import { pullDistance } from '@/services/pull-distance';

const emit = defineEmits<{ (e: 'refresh', ev: RefresherCustomEvent): void; (e: 'cancel'): void }>();

/**
 * How far the content travels before the ring closes — and, deliberately, the
 * same number Ionic arms at.
 *
 * Bigger than Ionic's default 60 because the held-open gap has to FIT the ring
 * and its label. That is the trade: a slightly longer pull, in exchange for an
 * indicator that never sits on top of a row.
 */
const PULL_MIN = 92;

const pull = ref(0); // 0–1
const pulling = ref(false);
/**
 * Whether Ionic is holding the refresh open.
 *
 * Read off the refresher's own class rather than tracked alongside it: the
 * caller decides when the work is done by calling complete(), and this component
 * is not told. Watching the class means the ring stops exactly when the gap
 * closes, instead of a moment either side of it.
 */
const refreshing = ref(false);
let classWatch: MutationObserver | null = null;
const refresherEl = ref<{ $el?: HTMLElement } | null>(null);
let scrollEl: HTMLElement | null = null;
let raf = 0;

/**
 * The pull distance, taken from ONE source: Ionic's own transform.
 *
 * Why one source rather than two, and why that is enforced by `pullDistance`'s
 * signature rather than by care here, is written up in `pull-distance.ts` — it
 * is the reason that module exists.
 */
function measure(): number {
  const el = scrollEl;
  return el ? pullDistance(getComputedStyle(el).transform) : 0;
}

function watchState(host: HTMLElement): void {
  if (classWatch) return;
  const sync = () => {
    refreshing.value = host.classList.contains('refresher-refreshing');
  };
  classWatch = new MutationObserver(sync);
  classWatch.observe(host, { attributes: true, attributeFilter: ['class'] });
  sync();
}

async function attach(): Promise<void> {
  const el = refresherEl.value?.$el;
  if (el) watchState(el);
  if (scrollEl) return;
  // Stop the browser's own rubber-band competing with the gesture: without this
  // the content visibly travels before Ionic has claimed the pull, then settles
  // back as it takes over. Contained here rather than globally, so only a view
  // that actually has a refresher gives up its overscroll.
  // IonRefresher is a Vue WRAPPER, so the DOM node is on `$el` — reaching for
  // `closest` on the instance itself throws, and the shape stays a dot.
  const host = refresherEl.value?.$el?.closest('ion-content') as HTMLIonContentElement | null;
  if (host) {
    scrollEl = await host.getScrollElement();
    scrollEl.style.overscrollBehaviorY = 'contain';
  }
}

function onStart(): void {
  void attach();
  if (pulling.value) return;
  pulling.value = true;
  const step = () => {
    if (!pulling.value) return;
    const distance = measure();
    // A pull let go of before the threshold fires no event at all — Ionic simply
    // retracts. Without this the loop would run for the life of the page,
    // measuring styles every frame for a gesture that ended long ago.
    if (distance <= 0 && !refreshing.value) {
      pulling.value = false;
      pull.value = 0;
      return;
    }
    pull.value = Math.max(0, Math.min(1, distance / PULL_MIN));
    raf = requestAnimationFrame(step);
  };
  cancelAnimationFrame(raf);
  raf = requestAnimationFrame(step);
}

function onRefresh(ev: RefresherCustomEvent): void {
  cancelAnimationFrame(raf);
  pulling.value = false;
  pull.value = 0;
  emit('refresh', ev);
}

/**
 * Cancelling retracts the indicator here, in the component that owns it.
 *
 * The page is told, so it can call off whatever it started, but it is not made
 * responsible for finding the refresher again: the pages that tried reached for
 * `document.querySelector('ion-refresher')`, and with five tabs alive in the DOM
 * at once that can just as easily close another tab's.
 */
function onCancel(): void {
  void (refresherEl.value?.$el as HTMLIonRefresherElement | undefined)?.complete();
  emit('cancel');
}

onUnmounted(() => {
  cancelAnimationFrame(raf);
  classWatch?.disconnect();
});

// The refresher exists before the first pull, so start watching immediately —
// otherwise a refresh triggered any other way would draw nothing.
onMounted(() => void attach());

const ease = (x: number) => (x <= 0 ? 0 : x >= 1 ? 1 : x * x * (3 - 2 * x));

/**
 * The whole animation, as one shape.
 *
 * The bulb reaches full size EARLY and the tail keeps growing after it, so what
 * you look at for most of the pull is a proper droplet — a round body under a
 * narrow neck — rather than a shape still on its way to being one. An earlier
 * version grew both together and started retracting at 72%, which left the last
 * third of the pull showing a blob that was neither. The tail now pinches off
 * over the last stretch only, and briefly.
 */
const CX = 60;
const CY = 46;
const R_MIN = 3;
const R_MAX = 15;
/** The bulb is full by here, so the droplet has a body while the tail draws out. */
const BULB_DONE = 0.5;
/** The tail is at its longest here; past it the neck pinches off into the ring. */
const TAIL_PEAK = 0.86;
const TAIL_MAX = 40;

const dropPath = computed(() => {
  const p = pull.value;
  const r = R_MIN + (R_MAX - R_MIN) * ease(Math.min(1, p / BULB_DONE));
  const tail =
    p <= TAIL_PEAK
      ? TAIL_MAX * ease(p / TAIL_PEAK)
      : TAIL_MAX * (1 - ease((p - TAIL_PEAK) / (1 - TAIL_PEAK)));
  const apex = CY - r - tail;

  // The control points slide between two shapes as the tail retracts, so the
  // LAST frame of the droplet is a true circle — the same circle the ring is.
  //
  // 0.5523 is the cubic Bézier constant for a quarter arc; with the tail gone,
  // these two curves are the top semicircle exactly. Hand-picked values that
  // merely looked round left the top visibly flat, so the droplet did not match
  // the ring it turned into.
  const K = 0.5523;
  const drawn = tail / TAIL_MAX; // 0 = circle, 1 = full droplet

  // NECK is how wide the shape is where it leaves the apex, and LIFT is how long
  // it stays that narrow before flaring into the bulb. Together they are the
  // difference between a fat teardrop and something being drawn down off a
  // surface it is still attached to: a thin stem held almost parallel for most
  // of its length, widening only as it reaches the body.
  const neck = r * (K + (0.09 - K) * drawn);
  const shoulder = r * (K + (1.12 - K) * drawn);
  const lift = tail * 0.82;
  return [
    'M', CX, apex,
    'C', CX - neck, apex + lift, CX - r, CY - shoulder, CX - r, CY,
    'A', r, r, 0, 1, 0, CX + r, CY,
    'C', CX + r, CY - shoulder, CX + neck, apex + lift, CX, apex,
    'Z',
  ].join(' ');
});
</script>

<template>
  <ion-refresher
    ref="refresherEl"
    slot="fixed"
    class="pr-refresher"
    :class="{ 'pr-live': refreshing }"
    :pull-min="PULL_MIN"
    @ionStart="onStart"
    @ionPull="onStart"
    @ionRefresh="onRefresh"
  >
    <!-- Drawn in the gap the refresher holds open, so nothing is covered. -->
    <div class="pr-stage">
      <!-- Spinning: a COMPACT box, so the label sits just above the ring.
           Sharing the droplet's tall viewBox left ~60px of empty space over the
           circle, which pushed the label away from it and up under the toolbar. -->
      <template v-if="refreshing">
        <div class="pr-label">Cancel Loading</div>
        <button
          type="button"
          class="pr-hit"
          aria-label="Cancel loading"
          data-testid="pull-refresh"
          @click="onCancel"
        >
          <svg viewBox="0 0 60 38" width="60" height="38" aria-hidden="true">
            <g class="pr-ring">
              <circle
                cx="30"
                cy="19"
                :r="R_MAX"
                fill="none"
                stroke="var(--ion-color-primary)"
                stroke-width="2.5"
                stroke-dasharray="5 5"
                stroke-linecap="round"
              />
            </g>
            <!-- Dead centre of the ring, and the only part that takes a tap. -->
            <path
              d="M24 13 L36 25 M36 13 L24 25"
              stroke="var(--ion-color-primary)"
              stroke-width="2.2"
              stroke-linecap="round"
              fill="none"
            />
          </svg>
        </button>
      </template>

      <!-- Pulling: a TALL box, because the tail needs somewhere to be drawn.
           Rendered only WHILE pulling — at rest the shape is a 3px dot, which is
           what was being left behind on the screen after a refresh finished. -->
      <svg
        v-else-if="pulling"
        viewBox="0 -22 120 90"
        width="110"
        height="82"
        style="overflow: visible"
        aria-hidden="true"
      >
        <path
          :d="dropPath"
          fill="none"
          stroke="var(--ion-color-primary)"
          stroke-width="2.5"
          stroke-dasharray="5 5"
          stroke-linecap="round"
        />
      </svg>
    </div>

  </ion-refresher>
</template>

<style scoped>
/* The refresher IS the gap. It parks above the content edge and comes into view
   as the pull opens, so anything drawn inside it occupies the band the list has
   vacated rather than sitting on top of a row.
   Anchored to the BOTTOM of that band rather than centred in it: the band's
   height is Ionic's to decide, and only the bottom edge is guaranteed to line up
   with the top of the list. The droplet's tail is free to reach up out of the
   band, which is empty space either way. */
.pr-refresher {
  min-height: 92px;
  overflow: visible;
}
/* Ionic parks the refresher at `z-index: -1`, BEHIND the content — which is why
   the cancel button did nothing: every tap landed on the list painted over it.
   Lifted only while the refresh is running, when the band is open and the list
   has been pushed clear of it, so normal scrolling is never intercepted. */
.pr-refresher.pr-live {
  z-index: 2;
}

.pr-stage {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 8px;
  display: flex;
  flex-direction: column;
  align-items: center;
  pointer-events: none;
  overflow: visible;
}

/* Tight to the ring: the gap between them is this margin and nothing else, so
   the pair reads as one thing rather than a caption stranded above it. */
.pr-label {
  font-size: 12px;
  line-height: 15px;
  color: var(--app-text-dim);
  margin-bottom: 3px;
}

/* Block, so the svg contributes no baseline gap under the label. */
.pr-hit svg {
  display: block;
}

.pr-hit {
  appearance: none;
  background: none;
  border: none;
  /* The ring is 38px tall, under the 44px a finger needs. The padding grows the
     TAP TARGET and the negative margin takes the growth back out of the layout,
     so the target is comfortable without the label drifting away from the ring. */
  padding: 8px 14px;
  margin: -8px -14px;
  line-height: 0;
  pointer-events: auto;
  cursor: pointer;
  touch-action: manipulation;
}
.pr-hit:disabled {
  cursor: default;
  pointer-events: none;
}

/* The origin MUST match the circle's centre in this viewBox. It was left at the
   droplet box's centre after the ring got a compact box of its own, which put it
   outside the view box entirely — so the ring orbited that point instead of
   spinning on itself, landing ~27px low and over the first row. */
.pr-ring {
  transform-box: view-box;
  transform-origin: 30px 19px;
  animation: pr-spin 1.15s linear infinite;
}
@keyframes pr-spin {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .pr-ring {
    animation-duration: 2.4s;
  }
}
</style>
