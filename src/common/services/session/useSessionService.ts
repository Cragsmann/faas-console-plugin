import { SessionService } from './SessionService';

let sessionService: SessionService | null = null;

export function useSessionService(): SessionService {
  if (!sessionService) {
    sessionService = new SessionService();
  }
  return sessionService;
}
