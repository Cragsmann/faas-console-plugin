import { createContext, ReactNode, useCallback, useEffect, useState } from 'react';
import { SESSION_EXPIRED_EVENT } from '../services/session/SessionService';
import { useSessionService } from '../services/session/useSessionService';
import { AuthUser, USER_KEY } from '../types';

const NO_USER: AuthUser = { name: '', avatarUrl: '' };

interface AuthState {
  isAuthenticated: boolean;
  user: AuthUser;
  connectionId: number;
  onLogin: (user: AuthUser) => void;
  onLogout: () => Promise<void>;
}

export const AuthContext = createContext<AuthState>({
  isAuthenticated: false,
  user: NO_USER,
  connectionId: 0,
  onLogin: () => {},
  onLogout: async () => {},
});

export function AuthProvider({ children }: Readonly<{ children: ReactNode }>) {
  const sessionService = useSessionService();
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(() =>
    sessionService.isSessionActive(),
  );
  const [user, setUser] = useState<AuthUser>(readStoredUser);
  const [connectionId, setConnectionId] = useState(0);

  const onLogin = (authUser: AuthUser) => {
    setUser(authUser);
    setIsAuthenticated(true);
    setConnectionId((id) => id + 1);
  };

  const clearAuth = useCallback(() => {
    setUser(NO_USER);
    setIsAuthenticated(false);
  }, []);

  const onLogout = useCallback(async () => {
    await sessionService.logout();
    clearAuth();
  }, [sessionService, clearAuth]);

  // A 401 on any backend call means the session is gone server-side; drop the
  // local state so the UI falls back to the connect prompt.
  useEffect(() => {
    window.addEventListener(SESSION_EXPIRED_EVENT, clearAuth);
    return () => window.removeEventListener(SESSION_EXPIRED_EVENT, clearAuth);
  }, [clearAuth]);

  return (
    <AuthContext.Provider value={{ isAuthenticated, user, connectionId, onLogin, onLogout }}>
      {children}
    </AuthContext.Provider>
  );
}

function readStoredUser(): AuthUser {
  const userJson = sessionStorage.getItem(USER_KEY);
  if (!userJson) return NO_USER;
  try {
    return JSON.parse(userJson) as AuthUser;
  } catch {
    return NO_USER;
  }
}
