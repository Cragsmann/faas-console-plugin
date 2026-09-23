import { consoleFetch, consoleFetchJSON } from '@openshift-console/dynamic-plugin-sdk';
import { sessionFetch, sessionFetchJSON } from './sessionFetch';
import { SESSION_EXPIRED_EVENT, SESSION_HEADER, SESSION_TOKEN_KEY } from './SessionService';

vi.mock('@openshift-console/dynamic-plugin-sdk', () => ({
  consoleFetch: vi.fn(),
  consoleFetchJSON: Object.assign(vi.fn(), { post: vi.fn() }),
}));

const fetchMock = vi.mocked(consoleFetch);
const fetchJSONMock = vi.mocked(consoleFetchJSON);

function sentHeaders(options: RequestInit | undefined) {
  return options?.headers as Record<string, string>;
}

/**
 * The error the console's coFetch throws: the code is reachable only through
 * the Response it attaches, never as a property of its own.
 */
function coFetchError(status: number, message = 'request failed') {
  return Object.assign(new Error(message), { response: new Response(null, { status }) });
}

/** The SDK's HttpError, which copies the code onto the error as well. */
function httpError(status: number, message = 'request failed') {
  return Object.assign(new Error(message), { status, response: new Response(null, { status }) });
}

describe('sessionFetch', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
    fetchMock.mockResolvedValue(new Response(null, { status: 200 }));
    fetchJSONMock.mockResolvedValue({});
  });

  it('sends the stored session token', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');

    await sessionFetch('/api/v1/func/create', { method: 'POST' });

    const [, options] = fetchMock.mock.calls[0];
    expect(sentHeaders(options)[SESSION_HEADER.toLowerCase()]).toBe('sess_test');
  });

  // The SDK merges options.headers with lodash defaultsDeep, which copies own
  // enumerable properties only. A Headers instance has none, so passing one
  // drops every header and the backend answers 401.
  it('passes headers as a plain object, not a Headers instance', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');

    await sessionFetch('/api/v1/func/create');

    const [, options] = fetchMock.mock.calls[0];
    expect(sentHeaders(options)).not.toBeInstanceOf(Headers);
    expect(Object.keys(sentHeaders(options))).toContain(SESSION_HEADER.toLowerCase());
  });

  it('keeps headers supplied by the caller', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');

    await sessionFetch('/api/v1/func/create', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });

    const [, options] = fetchMock.mock.calls[0];
    expect(sentHeaders(options)['content-type']).toBe('application/json');
  });

  it('omits the session header when there is no session', async () => {
    await sessionFetch('/api/v1/func/create');

    const [, options] = fetchMock.mock.calls[0];
    expect(Object.keys(sentHeaders(options))).not.toContain(SESSION_HEADER.toLowerCase());
  });

  // Regression: the console reports the code only on the attached Response.
  // Checking err.status alone left the user looking connected against a session
  // the backend had already forgotten.
  it('clears the session and announces expiry on a 401', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');
    fetchMock.mockRejectedValue(coFetchError(401, 'authentication required'));
    const onExpired = vi.fn();
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);

    await expect(sessionFetch('/api/v1/func/create')).rejects.toThrow('authentication required');

    expect(sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(onExpired).toHaveBeenCalled();
    window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  });

  it('clears the session when the 401 arrives as an HttpError', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');
    fetchMock.mockRejectedValue(httpError(401, 'Unauthorized'));

    await expect(sessionFetch('/api/v1/func/create')).rejects.toThrow('Unauthorized');

    expect(sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
  });

  it('leaves the session alone on other errors', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');
    fetchMock.mockRejectedValue(coFetchError(500, 'Server Error'));

    await expect(sessionFetch('/api/v1/func/create')).rejects.toThrow('Server Error');

    expect(sessionStorage.getItem(SESSION_TOKEN_KEY)).toBe('sess_test');
  });
});

describe('sessionFetchJSON', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.clearAllMocks();
    fetchJSONMock.mockResolvedValue({});
  });

  it('passes the session token as a plain object header', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');

    await sessionFetchJSON('/api/v1/func/list');

    const [, method, options] = fetchJSONMock.mock.calls[0];
    expect(method).toBe('GET');
    expect(sentHeaders(options)).not.toBeInstanceOf(Headers);
    expect(sentHeaders(options)[SESSION_HEADER.toLowerCase()]).toBe('sess_test');
  });

  it('clears the session and announces expiry on a 401', async () => {
    sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');
    fetchJSONMock.mockRejectedValue(coFetchError(401, 'authentication required'));
    const onExpired = vi.fn();
    window.addEventListener(SESSION_EXPIRED_EVENT, onExpired);

    await expect(sessionFetchJSON('/api/v1/func/list')).rejects.toThrow('authentication required');

    expect(sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(onExpired).toHaveBeenCalled();
    window.removeEventListener(SESSION_EXPIRED_EVENT, onExpired);
  });
});
