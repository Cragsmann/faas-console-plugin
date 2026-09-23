import { consoleFetch, consoleFetchJSON } from '@openshift-console/dynamic-plugin-sdk';
import { SESSION_EXPIRED_EVENT, SESSION_HEADER, SessionService } from './SessionService';

const sessionService = new SessionService();

function withSessionHeader(headers?: HeadersInit): Headers {
  const merged = new Headers(headers || {});
  const token = sessionService.getSessionToken();
  if (token) {
    // Not Authorization: the console proxy uses that header for the OCP user token.
    merged.set(SESSION_HEADER, token);
  }
  return merged;
}

// consoleFetch rejects with an HttpError on a non-2xx response instead of
// returning it, so an expired session arrives here as a thrown 401.
function reportExpiredSession(err: unknown): never {
  if ((err as { status?: number })?.status === 401) {
    sessionService.clearSession();
    window.dispatchEvent(new CustomEvent(SESSION_EXPIRED_EVENT));
  }
  throw err;
}

export function sessionFetch(url: string, options: RequestInit = {}): Promise<Response> {
  return consoleFetch(url, { ...options, headers: withSessionHeader(options.headers) }).catch(
    reportExpiredSession,
  );
}

export function sessionFetchJSON<T>(url: string, method = 'GET', options: RequestInit = {}) {
  return consoleFetchJSON(url, method, {
    ...options,
    headers: withSessionHeader(options.headers),
  }).catch(reportExpiredSession) as Promise<T>;
}
