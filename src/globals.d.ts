// Compile-time constants injected by vite.config.ts `define`.
declare const __APP_VERSION__: string;
declare const __RELEASE_NOTES__: Array<{ sha: string; subject: string }>;

declare module '*.vue' {
  import type { DefineComponent } from 'vue';
  const component: DefineComponent<object, object, unknown>;
  export default component;
}
