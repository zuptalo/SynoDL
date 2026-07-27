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
}
