import { SESSION_TOKEN_KEY } from '../services/session/SessionService';
import { USER_KEY } from '../types';

export function authenticateGithubFake() {
  sessionStorage.setItem(SESSION_TOKEN_KEY, 'sess_test');
  sessionStorage.setItem(
    USER_KEY,
    JSON.stringify({ name: 'twoGiants', avatarUrl: 'https://valid.url' }),
  );
}

export function logoutGithubFake() {
  sessionStorage.clear();
}
