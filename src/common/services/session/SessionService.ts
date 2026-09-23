import { consoleFetch, consoleFetchJSON } from '@openshift-console/dynamic-plugin-sdk';
import { AuthUser, PROXY_BASE, USER_KEY } from '../../types';

interface SessionServiceInterface {
  login(pat: string): Promise<AuthUser>;
  logout(): Promise<void>;
  isSessionActive(): boolean;
  getSessionToken(): string | null;
}

interface LoginResponse {
  token: string;
  login: string;
  avatarUrl: string;
}

export const SESSION_TOKEN_KEY = 'faas-console-session-token';
export const SESSION_HEADER = 'X-FUNC-SESSION';
export const SESSION_EXPIRED_EVENT = 'session-expired';

export class SessionService implements SessionServiceInterface {
  async login(pat: string): Promise<AuthUser> {
    const { token, login, avatarUrl }: LoginResponse = await consoleFetchJSON.post(
      `${PROXY_BASE}/api/v1/auth/login`,
      { pat },
    );
    const user: AuthUser = { name: login, avatarUrl };
    sessionStorage.setItem(SESSION_TOKEN_KEY, token);
    sessionStorage.setItem(USER_KEY, JSON.stringify(user));
    return user;
  }

  // Revokes the session on the backend, then clears local state. The local
  // clear happens even if the request fails, so the user is never stuck
  // appearing connected with a token the backend has already rejected.
  async logout(): Promise<void> {
    const token = this.getSessionToken();
    try {
      if (token) {
        await consoleFetch(`${PROXY_BASE}/api/v1/auth/logout`, {
          method: 'POST',
          headers: { [SESSION_HEADER]: token },
        });
      }
    } catch {
      // Best effort: the stored credential expires on its own, and leaving the
      // caller connected because revocation failed is the worse outcome.
    } finally {
      this.clearSession();
    }
  }

  clearSession(): void {
    sessionStorage.removeItem(SESSION_TOKEN_KEY);
    sessionStorage.removeItem(USER_KEY);
  }

  isSessionActive(): boolean {
    return sessionStorage.getItem(SESSION_TOKEN_KEY) !== null;
  }

  getSessionToken(): string | null {
    return sessionStorage.getItem(SESSION_TOKEN_KEY);
  }
}
