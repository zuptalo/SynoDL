// Wire DTOs shared with the Go handlers (server/internal/syno/client.go).
// Keep field names in lockstep with the server's JSON tags.

export interface Task {
  id: string;
  name: string;
  type: string; // bt, http, ftp, nzb, emule
  status: string; // downloading, paused, finished, seeding, error, …
  size: number;
  downloaded: number;
  uploaded: number;
  downloadSpeed: number;
  uploadSpeed: number;
  peers: number;
  seeders: number;
  createdAt: number; // unix seconds
  destination: string;
  uri?: string; // source URL/magnet, for copy + re-download
  errorDetail?: string; // DSM status_extra.error_detail for errored tasks (raw keyword)
  addedBy?: string; // SynoDL user who created it — sent to admins only (spec 1013)
  // Catalog metadata for downloads sent from Discover (spec 1013, 1016).
  mediaType?: string; // movie / series / anime
  imdbScore?: number;
  year?: string; // release year (movie) or range (series)
  posterUrl?: string; // catalog poster thumbnail for the row (spec 1016)
  catalogId?: string; // catalog title id for "Open in Discover" (spec 1016)
}

export interface Stats {
  downloadSpeed: number;
  uploadSpeed: number;
}

export interface Folder {
  name: string;
  path: string;
}

export interface ServerConfig {
  version: string;
  releaseNotes: Array<{ sha: string; subject: string }>;
  nasHost: string;
  /**
   * Largest single upload the server accepts, in MB (spec 1022). Read from the
   * server rather than duplicated in the client, so the limit the upload screen
   * states cannot drift from the one actually enforced.
   */
  uploadMaxMB?: number;
  /**
   * Whether the app may run in an ordinary browser tab instead of requiring
   * installation (spec 1008 gate). Operator-set; off unless turned on.
   */
  allowBrowserAccess?: boolean;
}
