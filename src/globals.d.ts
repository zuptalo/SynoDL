// Compile-time constants injected by vite.config.ts `define`.
declare const __APP_VERSION__: string;
declare const __RELEASE_NOTES__: Array<{ sha: string; subject: string }>;

declare module '*.vue' {
  import type { DefineComponent } from 'vue';
  const component: DefineComponent<object, object, unknown>;
  export default component;
}

// Bundled source marks (spec 1021 follow-up). Vite resolves an image import to
// its emitted URL; declaring it here keeps vue-tsc happy without pulling in the
// whole vite/client type surface.
declare module '*.png' {
  const src: string;
  export default src;
}
