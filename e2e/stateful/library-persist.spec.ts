/**
 * The reading of what is on the NAS is kept, not rebuilt from nothing (spec 0011).
 *
 * It used to live only in memory, so it was thrown away on every deploy and
 * replaced with an EMPTY reading whenever the NAS was briefly unreachable — and
 * an empty reading is, to a user, indistinguishable from "you own nothing". Both
 * of those blanked every ownership marker at once.
 *
 * These specs drive the NAS away and check the markers stay.
 */
import { expect, test } from '@playwright/test';
import {
  addSource,
  apiSearch,
  apiToken,
  clearSources,
  folderNameFor,
  refreshLibrary,
  seedLibrary,
  seedLibraryFiles,
  setFileStation,
  setSourceState,
} from './helpers';

let token = '';
const parentFor = (type: string) => (type === 'series' || type === 'anime' ? '/tv-show' : '/movie');

test.beforeEach(async () => {
  token = await apiToken();
  await clearSources(token);
  await setSourceState('reset');
});

test('ownership survives the NAS going away', async () => {
  const id = await addSource(token, 'Only Source', 0);
  const items = await apiSearch(token);
  const movie = items.find((i) => i.type === 'movie');
  test.skip(!movie, 'catalog served no movie');

  const folder = folderNameFor(movie!.title);
  const parent = parentFor(movie!.type);
  await seedLibrary({ [parent]: [folder] });
  const slug = movie!.id.split(':').pop();
  await seedLibraryFiles({ [`${parent}/${folder}`]: [`${slug}.1080p.WEBRip.x264.MockSite.mkv`] });
  await refreshLibrary(token, id, 'Only Source');

  const before = await apiSearch(token);
  expect(
    before.find((i) => i.id === movie!.id)?.ownership,
    'precondition: the seeded title is owned',
  ).toBe('owned');

  // The NAS stops answering. Before this spec, the next reading was empty and
  // every marker vanished with it.
  await setFileStation('down');
  try {
    const during = await apiSearch(token);
    const own = during.find((i) => i.id === movie!.id)?.ownership;
    expect(own, 'an unreachable NAS must not report the title as not owned').not.toBe('absent');
    expect(own, 'the last good reading should still answer').toBe('owned');
  } finally {
    await setFileStation('up');
  }
});
