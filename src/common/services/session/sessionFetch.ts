import { consoleFetch, consoleFetchJSON } from '@openshift-console/dynamic-plugin-sdk';
import { SESSION_EXPIRED_EVENT, SESSION_HEADER, SessionService } from './SessionService';

const sessionService = new SessionService();

// Returns a plain object, not a Headers instance. The SDK merges options.headers
// with lodash defaultsDeep, which only copies own enumerable properties, and a
// Headers instance has none: hand it one and every header is silently dropped.
function withSessionHeader(headers?: HeadersInit): Record<string, string> {
  const merged = new Headers(headers || {});
  const token = sessionService.getSessionToken();
  if (token) {
    // Not Authorization: the console proxy uses that header for the OCP user token.
    merged.set(SESSION_HEADER, token);
  }
  return Object.fromEntries(merged.entries());
}

// The status lives in different places depending on which error the SDK raises:
// the plain Error thrown by coFetch carries only the Response, while HttpError
// also copies the code onto itself. Reading just one of the two silently misses
// half the failures.
function statusOf(err: unknown): number | undefined {
  const e = err as { status?: number; response?: { status?: number } };
  return e?.status ?? e?.response?.status;
}

// consoleFetch rejects on a non-2xx response instead of returning it, so an
// expired session arrives here as a thrown 401.
function reportExpiredSession(err: unknown): never {
  if (statusOf(err) === 401) {
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
