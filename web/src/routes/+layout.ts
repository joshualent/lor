// Single-page app: no SSR, no prerendering. lor-server serves index.html for
// every non-file path and the client-side router takes over.
export const ssr = false;
export const prerender = false;
