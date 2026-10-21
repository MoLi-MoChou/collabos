import { ApiClient } from "./api/client";

export interface CollabClientOptions {
  /** Server origin, e.g. https://collab.example.com. */
  baseUrl: string;
  /** Personal API token (coa_…) from Settings → API tokens, or an access token. */
  token: string;
  workspaceId: string;
  /** Optional refresh token; when set, 401s rotate the access token transparently. */
  refreshToken?: string;
  fetch?: typeof fetch;
}

/**
 * Open API SDK (PRD V0.4 §open API / SDK). A typed, zod-validated client for
 * scripts and integrations, built on the same ApiClient the web app uses.
 * Uses the /api/v1 prefix so the server can version the contract.
 */
export function createCollabClient(o: CollabClientOptions): ApiClient {
  let token = o.token;
  let refresh = o.refreshToken ?? null;
  return new ApiClient({
    baseUrl: o.baseUrl.replace(/\/$/, ""),
    getToken: () => token,
    getWorkspaceId: () => o.workspaceId,
    getRefreshToken: () => refresh,
    onTokens: (t) => {
      token = t.token;
      refresh = t.refresh_token;
    },
    fetch: o.fetch,
  });
}
