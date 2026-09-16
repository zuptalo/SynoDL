<script setup lang="ts">
/**
 * Who made a title (spec 0014).
 *
 * Sits BELOW the download options on the detail sheet, deliberately: sending a
 * download is what the sheet is for, and putting faces above it would push the
 * one action a phone can see below the fold.
 *
 * The tile is the unit of the design, and it has three states in descending
 * order of what is known about somebody:
 *
 *   1. the source's own photograph, through the existing poster proxy;
 *   2. a photograph the server looks up and caches, for the very common case of
 *      a source that has none — every crew member on one source, and about half
 *      of a typical cast;
 *   3. their initials, for a person nobody has a picture of.
 *
 * The third is a designed state, not a failure. It is also where a photograph
 * that fails to load lands, so a dead URL leaves a tile rather than a gap.
 */
import { IonAvatar, IonIcon, IonListHeader, IonLabel } from '@ionic/vue';
import { openOutline } from 'ionicons/icons';
import { computed, reactive } from 'vue';
import type { Person } from '@/services/api';
import { personPhotoSrc, posterSrc } from '@/services/api';
import { imdbPersonUrl } from '@/services/imdb-link';
import { initials } from '@/services/person';

const props = defineProps<{
  cast?: Person[];
  directors?: Person[];
  creators?: Person[];
  writers?: Person[];
}>();

/**
 * The roles, in a fixed order, and only the ones anybody is in.
 *
 * A role the source publishes nobody for is dropped entirely rather than shown
 * empty (FR-004a) — which matters more than it sounds: one source leaves the
 * director empty for most series and names a creator instead, so a hardcoded
 * "Director" heading would be blank on half the series in the catalog.
 */
const sections = computed(() =>
  [
    { key: 'cast', label: 'Cast', people: props.cast },
    { key: 'directors', label: 'Director', people: props.directors },
    { key: 'creators', label: 'Creator', people: props.creators },
    { key: 'writers', label: 'Writer', people: props.writers },
  ].filter((s): s is { key: string; label: string; people: Person[] } => !!s.people?.length),
);

/**
 * How far down the fallback chain each tile has fallen. Held here, keyed per
 * tile, rather than written back onto the person — the person objects belong to
 * the server's response, and a component that edits its own props is a component
 * whose state disappears the next time that response is replaced.
 */
const fellBack = reactive<Record<string, 'source' | 'none'>>({});

/** A stable key for one person within one role. Two people who share a name are
 *  different people, so the id leads where there is one. */
function tileKey(role: string, p: Person, i: number): string {
  return `${role}:${p.imdbId || p.name}:${i}`;
}

/**
 * Where a tile's picture comes from, in descending order of what is known: the
 * source's own photograph (already in hand, no lookup), then the one the server
 * resolves, then nothing — which the template turns into initials.
 */
function photoFor(key: string, p: Person): string {
  const stage = fellBack[key];
  if (stage === 'none') return '';
  if (p.photoUrl && stage !== 'source') return posterSrc(p.photoUrl);
  return personPhotoSrc(p.imdbId);
}

/**
 * A photograph that will not load drops to the next source rather than leaving a
 * broken glyph (FR-032). A source URL that 404s still gets its person the lookup
 * they deserve, which is the case that matters: a stale cover URL is exactly the
 * kind of thing these sites leave lying around.
 */
function onPhotoError(key: string, p: Person) {
  fellBack[key] = fellBack[key] !== 'source' && p.photoUrl && p.imdbId ? 'source' : 'none';
}
</script>

<template>
  <section v-if="sections.length" class="credits" data-testid="title-credits">
    <div v-for="section in sections" :key="section.key" class="role">
      <ion-list-header>
        <ion-label>{{ section.label }}</ion-label>
      </ion-list-header>
      <!-- A horizontal scroller, which is the one thing here Ionic has no
           primitive for. The row scrolls inside itself so a long cast never
           makes the SHEET scroll sideways (FR-009). -->
      <div class="row" :data-testid="`credits-${section.key}`">
        <component
          :is="imdbPersonUrl(person.imdbId) ? 'a' : 'div'"
          v-for="(person, i) in section.people"
          :key="tileKey(section.key, person, i)"
          class="person"
          :class="{ linked: !!imdbPersonUrl(person.imdbId) }"
          :href="imdbPersonUrl(person.imdbId) || undefined"
          :target="imdbPersonUrl(person.imdbId) ? '_blank' : undefined"
          :rel="imdbPersonUrl(person.imdbId) ? 'noopener noreferrer' : undefined"
          :aria-label="
            imdbPersonUrl(person.imdbId) ? `Open ${person.name} on IMDb` : undefined
          "
        >
          <ion-avatar class="face">
            <img
              v-if="photoFor(tileKey(section.key, person, i), person)"
              :src="photoFor(tileKey(section.key, person, i), person)"
              :alt="person.name"
              loading="lazy"
              @error="onPhotoError(tileKey(section.key, person, i), person)"
            />
            <!-- Same size and shape as a photograph, so a cast nobody has
                 pictures of is a tidy row rather than a broken one. -->
            <div v-else class="face-fallback" aria-hidden="true">
              {{ initials(person.name) || '·' }}
            </div>
          </ion-avatar>
          <!-- dir="auto" for the same reason the synopsis has it: a name arrives
               in whatever script the source publishes, and the browser decides
               per string from its first strong character. -->
          <span class="name" dir="auto">
            {{ person.name }}
            <ion-icon
              v-if="imdbPersonUrl(person.imdbId)"
              :icon="openOutline"
              aria-hidden="true"
            />
          </span>
          <span v-if="person.character" class="character" dir="auto">{{ person.character }}</span>
        </component>
      </div>
    </div>
  </section>
</template>

<style scoped>
.credits {
  margin-top: 18px;
}
.role + .role {
  margin-top: 6px;
}
ion-list-header {
  padding-inline-start: 0;
  min-height: 28px;
  font-size: 0.82rem;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--app-text-dim);
}
ion-list-header ion-label {
  font-size: inherit;
  letter-spacing: inherit;
  text-transform: inherit;
}

/* The row scrolls, the sheet does not. -webkit-overflow-scrolling keeps the
   momentum feel on iOS, where this is most likely to be used. */
.row {
  display: flex;
  gap: 14px;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  padding: 2px 0 8px;
  scrollbar-width: none;
}
.row::-webkit-scrollbar {
  display: none;
}

.person {
  flex: 0 0 auto;
  width: 76px;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  text-decoration: none;
  color: inherit;
}
.person.linked:active {
  opacity: 0.6;
}

.face {
  width: 64px;
  height: 64px;
  margin-bottom: 6px;
}
.face img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
/* The initials tile is the same circle, filled with the app's own colours —
   never a broken-image glyph and never a collapsed box. */
.face-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--ion-color-step-150, rgba(127, 127, 127, 0.18));
  color: var(--app-text-dim);
  font-size: 1.05rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.name {
  font-size: 0.76rem;
  line-height: 1.25;
  color: var(--app-text);
  overflow-wrap: anywhere;
}
.name ion-icon {
  font-size: 0.66rem;
  vertical-align: baseline;
  opacity: 0.55;
}
.character {
  margin-top: 2px;
  font-size: 0.7rem;
  line-height: 1.2;
  color: var(--app-text-dim);
  overflow-wrap: anywhere;
}
</style>
