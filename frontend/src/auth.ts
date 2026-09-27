export type AuthUser = {
  username: string;
  role: string;
  permissions?: string[];
};

export type AuthSession = {
  token: string;
  refreshToken: string;
  user: AuthUser;
};

export type AuthCredentials = {
  username: string;
  password: string;
};

export const API_URL =
  import.meta.env.VITE_API_URL ?? "http://localhost:8080/api/v1";

const AUTH_URL = API_URL.replace(/\/api\/v1\/?$/, "");
const SESSION_KEY = "cima.auth";

type StoredSession = Pick<AuthSession, "token" | "refreshToken">;

// readSession recupera la sesión temporal del navegador y descarta datos dañados.
function readSession(): StoredSession | null {
  const stored = sessionStorage.getItem(SESSION_KEY);
  if (!stored) return null;

  try {
    return JSON.parse(stored) as StoredSession;
  } catch {
    sessionStorage.removeItem(SESSION_KEY);
    return null;
  }
}

// storeSession conserva los tokens solo hasta que se cierre la pestaña.
function storeSession(session: StoredSession) {
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(session));
}

// clearSession elimina las credenciales locales al cerrar sesión o expirar el refresh.
export function clearSession() {
  sessionStorage.removeItem(SESSION_KEY);
}

// hasPermission replica la regla del backend para personalizar navegación, nunca para sustituirlo.
export function hasPermission(user: AuthUser | null, permission: string) {
  return Boolean(
    user &&
      (user.role === "admin" || user.permissions?.includes(permission))
  );
}

function endpointUrl(path: string) {
  return path.startsWith("/auth/") ? `${AUTH_URL}${path}` : `${API_URL}${path}`;
}

// refreshAccessToken solicita un nuevo token de acceso usando el refresh token vigente.
async function refreshAccessToken(session: StoredSession) {
  const response = await fetch(`${AUTH_URL}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refreshToken: session.refreshToken }),
  });
  if (!response.ok) return null;

  const data = (await response.json()) as { token?: string };
  if (!data.token) return null;

  const updatedSession = { ...session, token: data.token };
  storeSession(updatedSession);
  return updatedSession;
}

// apiFetch adjunta el bearer token y renueva una sesión vencida antes de repetir la solicitud.
export async function apiFetch(path: string, init: RequestInit = {}) {
  const session = readSession();
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (session?.token) headers.set("Authorization", `Bearer ${session.token}`);

  let response = await fetch(endpointUrl(path), { ...init, headers });
  if (
    response.status !== 401 ||
    !session ||
    path === "/auth/login" ||
    path === "/auth/refresh"
  ) {
    return response;
  }

  let renewedSession: StoredSession | null = null;
  try {
    renewedSession = await refreshAccessToken(session);
  } catch {
    renewedSession = null;
  }
  if (!renewedSession) {
    clearSession();
    window.dispatchEvent(new Event("cima:session-expired"));
    return response;
  }

  headers.set("Authorization", `Bearer ${renewedSession.token}`);
  response = await fetch(endpointUrl(path), { ...init, headers });
  return response;
}

// authenticate inicia sesión y consulta al backend el perfil y los permisos efectivos.
export async function authenticate(credentials: AuthCredentials) {
  const response = await fetch(`${AUTH_URL}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(credentials),
  });
  if (!response.ok) return response;

  const session = (await response.json()) as AuthSession;
  storeSession({ token: session.token, refreshToken: session.refreshToken });
  return apiFetch("/auth/me");
}

// hasStoredSession permite decidir si se debe validar una sesión al iniciar la aplicación.
export function hasStoredSession() {
  return readSession() !== null;
}
