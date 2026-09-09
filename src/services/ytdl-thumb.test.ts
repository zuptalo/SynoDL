import { describe, expect, it } from 'vitest';
import { ytdlThumbSrc, ytdlThumbUrl } from './ytdl-thumb';

const HQ = 'https://i.ytimg.com/vi/zSGhyrF7YVo/hqdefault.jpg';

describe('ytdlThumbUrl', () => {
  it('asks for the 16:9 size for a row, so no letterbox bands are cropped in', () => {
    expect(ytdlThumbUrl(HQ, 'mq')).toBe('https://i.ytimg.com/vi/zSGhyrF7YVo/mqdefault.jpg');
  });

  it('asks for the large size for a detail sheet', () => {
    expect(ytdlThumbUrl(HQ, 'maxres')).toBe(
      'https://i.ytimg.com/vi/zSGhyrF7YVo/maxresdefault.jpg',
    );
  });

  it('keeps the video id exactly as it was found', () => {
    const odd = 'https://i.ytimg.com/vi/_-aB3xY9zQ1/hqdefault.jpg';
    expect(ytdlThumbUrl(odd, 'mq')).toContain('/vi/_-aB3xY9zQ1/');
  });

  it('handles the webp path the source also publishes', () => {
    expect(ytdlThumbUrl('https://i.ytimg.com/vi_webp/abc/hqdefault.webp', 'mq')).toBe(
      'https://i.ytimg.com/vi_webp/abc/mqdefault.jpg',
    );
  });

  it('drops any query the stored URL carried, since the size is the whole address', () => {
    expect(ytdlThumbUrl(`${HQ}?sqp=xyz`, 'mq')).toBe(
      'https://i.ytimg.com/vi/zSGhyrF7YVo/mqdefault.jpg?sqp=xyz',
    );
  });

  it('leaves a URL it does not recognise alone rather than rejecting it', () => {
    // Artwork comes from a lookup that may one day return something else, and a
    // shape this does not understand is still one the proxy may be able to serve.
    const other = 'https://i.ytimg.com/an/other/shape/entirely.jpg';
    expect(ytdlThumbUrl(other, 'mq')).toBe(other);
    expect(ytdlThumbUrl('not a url at all', 'mq')).toBe('not a url at all');
  });

  it('is empty for a download with no artwork', () => {
    expect(ytdlThumbUrl(undefined, 'mq')).toBe('');
    expect(ytdlThumbUrl('', 'maxres')).toBe('');
  });
});

describe('ytdlThumbSrc', () => {
  it('addresses the proxy, so the viewer never contacts the source', () => {
    const src = ytdlThumbSrc(HQ, 'mq');
    expect(src.startsWith('/v1/ytdl/thumb?u=')).toBe(true);
    expect(decodeURIComponent(src.split('u=')[1])).toBe(
      'https://i.ytimg.com/vi/zSGhyrF7YVo/mqdefault.jpg',
    );
  });

  it('is empty when there is nothing to show, so the icon fallback wins', () => {
    expect(ytdlThumbSrc(undefined, 'mq')).toBe('');
  });
});
