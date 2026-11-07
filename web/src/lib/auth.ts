const TOKEN_KEY = "afterglow_token"; // Read only during legacy-session migration.
const USER_KEY = "aptex_session_user";

async function sessionRequest(path: string, token: string | null): Promise<Response> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 12_000);
  try {
    return await fetch(path, { method: "POST", credentials: "same-origin", cache: "no-store",
      signal: controller.signal, headers: token ? { Authorization: `Bearer ${token}` } : {} });
  } finally { clearTimeout(timeout); }
}

export interface CurrentUser {
  id: string; name: string; username: string; email: string; avatar: string; role: string;
}

function userFor(username: string): CurrentUser {
  return { id: "administrator", name: username, username, email: "", avatar: "", role: "operator" };
}

function legacyUser(token: string): CurrentUser | null {
  try {
    const encoded = token.split(".")[1].replace(/-/g, "+").replace(/_/g, "/");
    const payload = JSON.parse(atob(encoded.padEnd(Math.ceil(encoded.length / 4) * 4, "=")));
    return typeof payload.sub === "string" && payload.sub ? userFor(payload.sub) : null;
  } catch { return null; }
}

export const auth = {
  getToken(): string | null {
    if (typeof window === "undefined") return null;
    return localStorage.getItem(TOKEN_KEY);
  },

  // The server sets the HttpOnly cookie. Persist display data, never the token.
  setToken(token: string): void {
    const user = legacyUser(token) ?? (process.env.NEXT_PUBLIC_MOCK === "1" ? userFor("Afterglow") : null);
    if (!user) throw new Error("Invalid sign-in response");
    localStorage.setItem(USER_KEY, JSON.stringify(user));
    localStorage.removeItem(TOKEN_KEY);
  },

  async restoreSession(): Promise<boolean> {
    if (process.env.NEXT_PUBLIC_MOCK === "1") return true;
    const token = auth.getToken();
    const response = await sessionRequest("/api/v1/auth/restore", token);
    if (response.status === 401) { auth.clearToken(); return false; }
    if (!response.ok) throw new Error("Could not restore the server session");
    const session = await response.json() as { user?: { username?: string } };
    if (!session.user?.username) throw new Error("Invalid session response");
    localStorage.setItem(USER_KEY, JSON.stringify(userFor(session.user.username)));
    localStorage.removeItem(TOKEN_KEY);
    return true;
  },

  async logout(): Promise<void> {
    if (process.env.NEXT_PUBLIC_MOCK !== "1") {
      const token = auth.getToken();
      const response = await sessionRequest("/api/v1/auth/logout", token);
      if (!response.ok) throw new Error("Could not sign out of the server");
    }
    auth.clearToken();
  },

  clearToken(): void {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    document.cookie = `${TOKEN_KEY}=; path=/; max-age=0; SameSite=Lax`;
  },

  getCurrentUser(): CurrentUser | null {
    if (typeof window === "undefined") return null;
    try {
      const saved = localStorage.getItem(USER_KEY);
      if (saved) {
        const user = JSON.parse(saved) as CurrentUser;
        if (typeof user.username === "string" && user.username) return userFor(user.username);
      }
    } catch { /* Server validation replaces invalid display metadata. */ }
    const token = auth.getToken();
    return token ? legacyUser(token) : null;
  },
};
