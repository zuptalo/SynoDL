/**
 * The Discover filter/sort option lists, shared by the filter sheet (which picks
 * them) and the Browser page (which renders the active-filter chips and the sort
 * dropdown). Kept in one place so a chip and its picker never drift, and so a
 * stored code (genre "3362", country "GB") can be shown human-readably.
 *
 * Values are the provider's own codes where the API needs them (genre, language,
 * country, type-name→code mapping happens server-side); the labels are ours.
 */
export interface Option {
  value: string;
  label: string;
}

// Sort orders the provider actually honors (orderby, always descending).
export const SORTS: Option[] = [
  { value: 'favorite', label: 'Most popular' },
  { value: 'imdb', label: 'IMDb rating' },
  { value: 'year', label: 'Release year' },
  { value: 'date', label: 'Recently added' },
];

// These lists mirror what the live facets produce (see facet-labels.ts): sorted
// by label, so the sheet reads the same whether or not the provider's parameters
// have loaded yet. Only SCORES keeps a scale order rather than an alphabetical one.
export const TYPES: Option[] = [
  { value: '', label: 'All types' },
  { value: 'anime', label: 'Anime' },
  { value: 'movie', label: 'Movies' },
  { value: 'series', label: 'Series' },
];

export const SCORES: Option[] = [
  { value: '', label: 'Any rating' },
  { value: '9', label: '9.0+' },
  { value: '8.5', label: '8.5+' },
  { value: '8', label: '8.0+' },
  { value: '7.5', label: '7.5+' },
  { value: '7', label: '7.0+' },
  { value: '6', label: '6.0+' },
  { value: '5', label: '5.0+' },
];

export const QUALITIES: string[] = [
  '4K',
  'BluRay',
  'BluRay Full HD',
  'CAM',
  'DVDRip',
  'HDRip',
  'HDTV',
  'Remux',
  'WEB-DL',
  'WEBRip',
];

export const GENRES: Option[] = [
  { value: '3355', label: 'Action' },
  { value: '3356', label: 'Adventure' },
  { value: '3357', label: 'Animation' },
  { value: '3358', label: 'Biography' },
  { value: '3359', label: 'Comedy' },
  { value: '3360', label: 'Crime' },
  { value: '3361', label: 'Documentary' },
  { value: '3362', label: 'Drama' },
  { value: '3363', label: 'Family' },
  { value: '3364', label: 'Fantasy' },
  { value: '3366', label: 'History' },
  { value: '3367', label: 'Horror' },
  { value: '3368', label: 'Music' },
  { value: '3370', label: 'Mystery' },
  { value: '3372', label: 'Romance' },
  { value: '3373', label: 'Sci-Fi' },
  { value: '3374', label: 'Short' },
  { value: '3375', label: 'Sport' },
  { value: '3377', label: 'Superhero' },
  { value: '3378', label: 'Thriller' },
  { value: '3379', label: 'War' },
  { value: '3380', label: 'Western' },
];

// English leads (it's the overwhelmingly common pick); the rest are alphabetical.
export const LANGUAGES: Option[] = [
  { value: 'en', label: 'English' },
  { value: 'ar', label: 'Arabic' },
  { value: 'zh', label: 'Chinese' },
  { value: 'fr', label: 'French' },
  { value: 'de', label: 'German' },
  { value: 'hi', label: 'Hindi' },
  { value: 'it', label: 'Italian' },
  { value: 'ja', label: 'Japanese' },
  { value: 'ko', label: 'Korean' },
  { value: 'fa', label: 'Persian' },
  { value: 'ru', label: 'Russian' },
  { value: 'es', label: 'Spanish' },
];

export const COUNTRIES: Option[] = [
  { value: 'AU', label: 'Australia' },
  { value: 'CA', label: 'Canada' },
  { value: 'CN', label: 'China' },
  { value: 'FR', label: 'France' },
  { value: 'DE', label: 'Germany' },
  { value: 'IN', label: 'India' },
  { value: 'IR', label: 'Iran' },
  { value: 'IT', label: 'Italy' },
  { value: 'JP', label: 'Japan' },
  { value: 'KR', label: 'South Korea' },
  { value: 'ES', label: 'Spain' },
  { value: 'TR', label: 'Turkey' },
  { value: 'GB', label: 'United Kingdom' },
  { value: 'US', label: 'United States' },
];

/** Look up a human label for a stored code, falling back to the code itself. */
function labelOf(list: Option[], value: string | undefined): string {
  if (!value) return '';
  return list.find((o) => o.value === value)?.label ?? value;
}

export const genreLabel = (v?: string): string => labelOf(GENRES, v);
export const languageLabel = (v?: string): string => labelOf(LANGUAGES, v);
export const countryLabel = (v?: string): string => labelOf(COUNTRIES, v);
export const sortLabel = (v?: string): string => labelOf(SORTS, v || 'year');
