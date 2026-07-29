/**
 * Turns the provider's live filter facets (from GET /v1/source/parameters) into
 * English-labelled options for the filter UI. The provider's own names are
 * Persian, but most facets carry a language-neutral value we can localize:
 *   - genre: an English slug ("sci-fi") we title-case
 *   - country / language: ISO codes resolved via Intl.DisplayNames
 *   - type: a small numeric map
 *   - quality: already English
 *   - score: numeric bands we format
 * Anything we can't localize falls back to the provider's own name, then the raw
 * value — so a new facet value still shows something sensible.
 */
import type { SourceFacet } from '@/services/api';

export interface Option {
  value: string;
  label: string;
}

// Intl.DisplayNames is constructed once (it's not free). Guarded because very old
// engines lack it.
const regionNames = safeDisplayNames('region');
const languageNames = safeDisplayNames('language');
function safeDisplayNames(type: 'region' | 'language'): Intl.DisplayNames | null {
  try {
    return new Intl.DisplayNames(['en'], { type });
  } catch {
    return null;
  }
}
// .of() throws a RangeError on a structurally invalid code (e.g. "XWG", "SUHH"),
// so guard every lookup and let the caller fall back.
function safeOf(dn: Intl.DisplayNames | null, code: string): string {
  if (!dn) return '';
  try {
    return dn.of(code) ?? '';
  } catch {
    return '';
  }
}

function titleCase(slug: string): string {
  return slug
    .split('-')
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join('-');
}

// The provider's numeric/compound type codes → friendly English names.
const TYPE_LABELS: Record<string, string> = {
  '15': 'Movies',
  '16': 'Series',
  '67378': 'Mini-series',
  '17': 'Anime (film)',
  '17&124913': 'Anime (series)',
  '3357': 'Animation',
  '3361': 'Documentary',
  '246': 'Concert',
  '241': 'Ceremony',
};

function scoreLabel(value: string): string {
  if (value.startsWith('-')) return `Under ${Math.abs(parseFloat(value)).toFixed(1)}`;
  const n = parseFloat(value);
  return Number.isNaN(n) ? value : `${n.toFixed(1)}+`;
}

function localize(f: SourceFacet, resolve: (v: string) => string): Option {
  const label = resolve(f.value) || f.name || f.value;
  return { value: f.value, label };
}

export function genreOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => ({ value: f.value, label: f.slug ? titleCase(f.slug) : f.name || f.value }));
}
export function typeOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => localize(f, (v) => TYPE_LABELS[v] ?? ''));
}
export function qualityOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => ({ value: f.value, label: f.value }));
}
export function scoreOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => ({ value: f.value, label: scoreLabel(f.value) }));
}
export function languageOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => localize(f, (v) => safeOf(languageNames, v)));
}
export function countryOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => localize(f, (v) => safeOf(regionNames, v)));
}
// Channel, encoder and age are already display-ready (network name, release
// group, content rating) — the value is the label.
export function passthroughOptions(facets: SourceFacet[]): Option[] {
  return facets.map((f) => ({ value: f.value, label: f.name || f.value }));
}
