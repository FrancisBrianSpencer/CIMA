import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import {
  apiFetch,
  authenticate,
  clearSession,
  hasPermission,
  hasStoredSession,
  type AuthUser,
} from "./auth";
import {
  formatDateForDisplay,
  formatDateInput,
  localDateToISO,
  parseSpanishDate,
} from "./dates";

// ResidentStatus representa el estado operativo del residente dentro del sistema.
type ResidentStatus = "active" | "inactive";

// Resident describe la identidad básica de una persona residente en la institución.
type Resident = {
  id: string;
  firstName: string;
  lastName: string;
  status: ResidentStatus | string;
};

type RoomStatus = "available" | "maintenance" | "closed";

type Room = {
  id: string;
  code: string;
  capacity: number;
  status: RoomStatus;
  occupancy: number;
  occupantIds?: string[];
};

type RoomAssignmentEvent = {
  residentId: string;
  action: "assigned" | "released";
  actor: string;
  changedAt: string;
};

type Contact = {
  name: string;
  relationship: string;
  phone: string;
  email: string;
};

type ResidentProfile = {
  dateOfBirth: string;
  phone: string;
  email: string;
  primaryContact: Contact;
  emergencyContact: Contact;
};

type ApiError = {
  error?: string;
};

function permissionLabel(permission: string) {
  const labels: Record<string, string> = {
    "resident.read": "Consultar residentes",
    "resident.create": "Crear residentes",
    "resident.update": "Actualizar fichas",
    "resident.delete": "Archivar residentes",
    "room.read": "Consultar habitaciones",
    "room.write": "Gestionar habitaciones",
    "medical.read": "Consultar atención clínica",
    "medical.write": "Registrar atención clínica",
    "medication.read": "Consultar medicamentos",
    "medication.write": "Registrar medicamentos",
    "billing.read": "Consultar facturación",
    "billing.write": "Gestionar facturación",
    "document.read": "Consultar documentos",
    "document.write": "Gestionar documentos",
    "user.read": "Consultar usuarios",
    "user.write": "Gestionar usuarios",
    "audit.read": "Consultar auditoría",
    "dashboard.read": "Consultar resumen",
  };
  return labels[permission] ?? permission;
}

// emptyContact crea una estructura base para un contacto nuevo sin datos cargados.
const emptyContact: Contact = {
  name: "",
  relationship: "",
  phone: "",
  email: "",
};

// emptyProfile evita valores nulos al iniciar la edición de una ficha del residente.
const emptyProfile: ResidentProfile = {
  dateOfBirth: "",
  phone: "",
  email: "",
  primaryContact: emptyContact,
  emergencyContact: emptyContact,
};

// getApiError captura el mensaje de error del backend para mostrarlo al usuario en español.
async function getApiError(response: Response, fallback: string) {
  try {
    const data = (await response.json()) as ApiError;
    return data.error ?? fallback;
  } catch {
    return fallback;
  }
}

// userFacingError reemplaza fallos técnicos del navegador por una explicación en español.
function userFacingError(error: unknown, fallback: string) {
  if (error instanceof TypeError) {
    return "No se pudo conectar con el sistema. Revisa tu conexión e inténtalo nuevamente.";
  }
  return error instanceof Error ? error.message : fallback;
}

// profileForDisplay mantiene la fecha de nacimiento en formato chileno dentro de la interfaz.
function profileForDisplay(data: Partial<ResidentProfile>): ResidentProfile {
  return {
    ...emptyProfile,
    ...data,
    dateOfBirth: formatDateForDisplay(data.dateOfBirth ?? ""),
    primaryContact: { ...emptyContact, ...data.primaryContact },
    emergencyContact: { ...emptyContact, ...data.emergencyContact },
  };
}

export default function App() {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [authChecking, setAuthChecking] = useState(true);
  const [loginUsername, setLoginUsername] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [loginSaving, setLoginSaving] = useState(false);
  const [activeView, setActiveView] = useState("dashboard");
  const [dashboardStatus, setDashboardStatus] = useState("");
  const [dashboardError, setDashboardError] = useState("");
  const [residents, setResidents] = useState<Resident[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [roomsLoading, setRoomsLoading] = useState(false);
  const [roomsSaving, setRoomsSaving] = useState(false);
  const [roomError, setRoomError] = useState("");
  const [roomSuccess, setRoomSuccess] = useState("");
  const [roomCode, setRoomCode] = useState("");
  const [roomCapacity, setRoomCapacity] = useState("1");
  const [roomStatus, setRoomStatus] = useState<RoomStatus>("available");
  const [editingRoomId, setEditingRoomId] = useState("");
  const [roomResidentSelection, setRoomResidentSelection] = useState<Record<string, string>>({});
  const [roomHistory, setRoomHistory] = useState<Record<string, RoomAssignmentEvent[]>>({});
  const [visibleRoomHistory, setVisibleRoomHistory] = useState("");
  const [selectedResident, setSelectedResident] = useState<Resident | null>(null);
  const [profile, setProfile] = useState<ResidentProfile>(emptyProfile);
  const datePickerRef = useRef<HTMLInputElement>(null);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [editFirstName, setEditFirstName] = useState("");
  const [editLastName, setEditLastName] = useState("");

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [residentSaving, setResidentSaving] = useState(false);
  const [profileLoading, setProfileLoading] = useState(false);
  const [profileSaving, setProfileSaving] = useState(false);
  const [profileLoaded, setProfileLoaded] = useState(false);

  // Estados de mensajes para feedback visual del usuario en la interfaz de gestión.
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [residentError, setResidentError] = useState("");
  const [residentSuccess, setResidentSuccess] = useState("");
  const [profileError, setProfileError] = useState("");
  const [profileSuccess, setProfileSuccess] = useState("");

  useEffect(() => {
    let mounted = true;
    const expireSession = () => {
      if (mounted) {
        setUser(null);
        setResidents([]);
        setSelectedResident(null);
        setProfile(emptyProfile);
      }
    };
    window.addEventListener("cima:session-expired", expireSession);

    if (!hasStoredSession()) {
      setAuthChecking(false);
    } else {
      void apiFetch("/auth/me")
        .then(async (response) => {
          if (!response.ok) throw new Error("La sesión ya no es válida.");
          const currentUser = (await response.json()) as AuthUser;
          if (mounted) setUser(currentUser);
        })
        .catch(() => clearSession())
        .finally(() => {
          if (mounted) setAuthChecking(false);
        });
    }

    return () => {
      mounted = false;
      window.removeEventListener("cima:session-expired", expireSession);
    };
  }, []);

  // handleLogin autentica al usuario y carga el perfil efectivo que entrega la API.
  async function handleLogin(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLoginError("");
    setLoginSaving(true);
    try {
      const response = await authenticate({
        username: loginUsername.trim(),
        password: loginPassword,
      });
      if (!response.ok) {
        clearSession();
        throw new Error(await getApiError(response, "Credenciales incorrectas."));
      }
      setUser((await response.json()) as AuthUser);
      setLoginPassword("");
      setActiveView("dashboard");
    } catch (err) {
      setLoginError(userFacingError(err, "No se pudo iniciar sesión."));
    } finally {
      setLoginSaving(false);
    }
  }

  // handleLogout cierra la sesión local y elimina de memoria los datos protegidos.
  function handleLogout() {
    clearSession();
    setUser(null);
    setResidents([]);
    setSelectedResident(null);
    setActiveView("dashboard");
  }

  // loadResidents consulta la lista de residentes desde la API del backend.
  const loadResidents = useCallback(async () => {
    if (!hasPermission(user, "resident.read")) {
      setResidents([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");

    try {
      const response = await apiFetch("/residents/");

      if (!response.ok) {
        throw new Error(
          await getApiError(
            response,
            "No se pudieron cargar los residentes."
          )
        );
      }

      const data = (await response.json()) as Resident[];
      setResidents(data);
    } catch (err) {
      setError(userFacingError(err, "No se pudieron cargar los residentes."));
    } finally {
      setLoading(false);
    }
  }, [user]);

  // loadResidents solo consulta datos cuando la sesión tiene el permiso de lectura.
  useEffect(() => {
    if (user && hasPermission(user, "resident.read")) void loadResidents();
  }, [loadResidents]);

  // loadRooms obtiene el catálogo usando el permiso de lectura de habitaciones.
  const loadRooms = useCallback(async () => {
    if (!hasPermission(user, "room.read")) return;
    setRoomsLoading(true);
    setRoomError("");
    try {
      const response = await apiFetch("/rooms/");
      if (!response.ok) {
        throw new Error(await getApiError(response, "No se pudieron cargar las habitaciones."));
      }
      const data = (await response.json()) as Room[];
      setRooms(data.sort((left, right) => left.code.localeCompare(right.code, "es-CL")));
    } catch (err) {
      setRoomError(userFacingError(err, "No se pudieron cargar las habitaciones."));
    } finally {
      setRoomsLoading(false);
    }
  }, [user]);

  useEffect(() => {
    if (user && activeView === "rooms") void loadRooms();
  }, [user, activeView, loadRooms]);

  // updateRoomAssignment asigna o libera residentes mediante las rutas protegidas de habitaciones.
  async function updateRoomAssignment(roomId: string, residentId: string, action: "assign" | "release") {
    if (!residentId) {
      setRoomError("Selecciona un residente activo.");
      return;
    }
    setRoomError("");
    setRoomSuccess("");
    setRoomsSaving(true);
    try {
      const response = await apiFetch(`/rooms/${encodeURIComponent(roomId)}/${action}`, {
        method: "POST",
        body: JSON.stringify({ residentId }),
      });
      if (!response.ok) {
        throw new Error(await getApiError(response, "No se pudo actualizar la asignación."));
      }
      setRoomSuccess(action === "assign" ? "Residente asignado a la habitación." : "Residente liberado de la habitación.");
      setRoomResidentSelection((current) => ({ ...current, [roomId]: "" }));
      await loadRooms();
      if (visibleRoomHistory === roomId) await loadRoomHistory(roomId);
    } catch (err) {
      setRoomError(userFacingError(err, "No se pudo actualizar la asignación."));
    } finally {
      setRoomsSaving(false);
    }
  }

  // loadRoomHistory muestra los eventos históricos autorizados de una habitación.
  async function loadRoomHistory(roomId: string) {
    setRoomError("");
    try {
      const response = await apiFetch(`/rooms/${encodeURIComponent(roomId)}/history`);
      if (!response.ok) {
        throw new Error(await getApiError(response, "No se pudo cargar el historial."));
      }
      const events = (await response.json()) as RoomAssignmentEvent[];
      setRoomHistory((current) => ({ ...current, [roomId]: events }));
      setVisibleRoomHistory(roomId);
    } catch (err) {
      setRoomError(userFacingError(err, "No se pudo cargar el historial."));
    }
  }

  // handleRoomSubmit crea o actualiza una habitación validada por la API.
  async function handleRoomSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const cleanCode = roomCode.trim();
    const parsedCapacity = Number(roomCapacity);
    if (!cleanCode || !Number.isInteger(parsedCapacity) || parsedCapacity < 1) {
      setRoomError("Ingresa un código y una capacidad entera de al menos 1.");
      return;
    }

    setRoomError("");
    setRoomSuccess("");
    setRoomsSaving(true);
    try {
      const editing = editingRoomId !== "";
      const response = await apiFetch(
        editing ? `/rooms/${encodeURIComponent(editingRoomId)}` : "/rooms/",
        {
          method: editing ? "PATCH" : "POST",
          body: JSON.stringify({ code: cleanCode, capacity: parsedCapacity, status: roomStatus }),
        }
      );
      if (!response.ok) {
        throw new Error(await getApiError(response, "No se pudo guardar la habitación."));
      }
      setRoomSuccess(editing ? "Habitación actualizada." : "Habitación creada.");
      setEditingRoomId("");
      setRoomCode("");
      setRoomCapacity("1");
      setRoomStatus("available");
      await loadRooms();
    } catch (err) {
      setRoomError(userFacingError(err, "No se pudo guardar la habitación."));
    } finally {
      setRoomsSaving(false);
    }
  }

  // startRoomEdit carga en el formulario el catálogo de la habitación seleccionada.
  function startRoomEdit(room: Room) {
    setEditingRoomId(room.id);
    setRoomCode(room.code);
    setRoomCapacity(String(room.capacity));
    setRoomStatus(room.status);
    setRoomError("");
    setRoomSuccess("");
  }

  // cancelRoomEdit limpia el formulario de edición sin modificar el registro.
  function cancelRoomEdit() {
    setEditingRoomId("");
    setRoomCode("");
    setRoomCapacity("1");
    setRoomStatus("available");
    setRoomError("");
  }

  // loadDashboard consulta el endpoint protegido para mostrar el estado real del backend.
  useEffect(() => {
    if (!user || activeView !== "dashboard" || !hasPermission(user, "dashboard.read")) return;
    let mounted = true;
    setDashboardError("");
    void apiFetch("/dashboard/")
      .then(async (response) => {
        if (!response.ok) throw new Error(await getApiError(response, "No se pudo cargar el resumen."));
        const data = (await response.json()) as { status?: string };
        if (mounted) setDashboardStatus(data.status ?? "Panel disponible");
      })
      .catch((err: unknown) => {
        if (mounted) setDashboardError(userFacingError(err, "No se pudo cargar el resumen."));
      });
    return () => {
      mounted = false;
    };
  }, [user, activeView]);

  // handleSubmit crea un nuevo residente con validación básica de nombre y apellido.
  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    setError("");
    setSuccess("");

    const cleanFirstName = firstName.trim();
    const cleanLastName = lastName.trim();

    if (!cleanFirstName || !cleanLastName) {
      setError("Nombre y apellido son obligatorios.");
      return;
    }

    setSaving(true);

    try {
      const response = await apiFetch("/residents/", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          firstName: cleanFirstName,
          lastName: cleanLastName,
        }),
      });

      if (!response.ok) {
        throw new Error(
          await getApiError(
            response,
            "No se pudo crear el residente."
          )
        );
      }

      setFirstName("");
      setLastName("");

      setSuccess("Residente creado correctamente.");

      await loadResidents();
    } catch (err) {
      setError(userFacingError(err, "No se pudo crear el residente."));
    } finally {
      setSaving(false);
    }
  }

  // selectResident carga la ficha completa de un residente y deja la edición lista para uso.
  async function selectResident(resident: Resident) {
    setSelectedResident(resident);
    setEditFirstName(resident.firstName);
    setEditLastName(resident.lastName);
    setResidentError("");
    setResidentSuccess("");
    setProfile(emptyProfile);
    setProfileLoaded(false);
    setProfileLoading(true);
    setProfileError("");
    setProfileSuccess("");

    try {
      const response = await apiFetch(
        `/residents/${encodeURIComponent(resident.id)}/profile`
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo cargar la ficha.")
        );
      }

      const data = (await response.json()) as Partial<ResidentProfile>;
      setProfile(profileForDisplay(data));
      setProfileLoaded(true);
    } catch (err) {
      setProfileError(userFacingError(err, "No se pudo cargar la ficha."));
    } finally {
      setProfileLoading(false);
    }
  }

  // handleResidentUpdate actualiza la identidad principal del residente seleccionado.
  async function handleResidentUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedResident) return;

    const cleanFirstName = editFirstName.trim();
    const cleanLastName = editLastName.trim();
    if (!cleanFirstName || !cleanLastName) {
      setResidentError("Nombre y apellido son obligatorios.");
      return;
    }

    setResidentError("");
    setResidentSuccess("");
    setResidentSaving(true);

    try {
      const response = await apiFetch(
        `/residents/${encodeURIComponent(selectedResident.id)}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            firstName: cleanFirstName,
            lastName: cleanLastName,
          }),
        }
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo actualizar el residente.")
        );
      }

      const updatedResident = (await response.json()) as Resident;
      setSelectedResident(updatedResident);
      setResidents((current) =>
        current.map((resident) =>
          resident.id === updatedResident.id ? updatedResident : resident
        )
      );
      setEditFirstName(updatedResident.firstName);
      setEditLastName(updatedResident.lastName);
      setResidentSuccess("Residente actualizado correctamente.");
    } catch (err) {
      setResidentError(userFacingError(err, "No se pudo actualizar el residente."));
    } finally {
      setResidentSaving(false);
    }
  }

  // archiveResident archiva lógicamente al residente sin eliminar el registro físico de la base de datos.
  async function archiveResident(resident: Resident) {
    if (resident.status === "archived") return;
    const fullName = `${resident.firstName} ${resident.lastName}`;
    if (!window.confirm(`¿Archivar a ${fullName}?`)) return;

    setResidentError("");
    setResidentSuccess("");

    try {
      const response = await apiFetch(
        `/residents/${encodeURIComponent(resident.id)}`,
        { method: "DELETE" }
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo archivar el residente.")
        );
      }

      const archivedResident = { ...resident, status: "archived" };
      setResidents((current) =>
        current.map((item) =>
          item.id === archivedResident.id ? archivedResident : item
        )
      );
      setSelectedResident((current) =>
        current?.id === archivedResident.id ? archivedResident : current
      );
      setResidentSuccess("Residente archivado. Sus datos se conservaron.");
    } catch (err) {
      setResidentError(userFacingError(err, "No se pudo archivar el residente."));
    }
  }

  // updateProfile modifica un campo de la ficha del residente sin perder los demás datos.
  function updateProfile(field: "dateOfBirth" | "phone" | "email", value: string) {
    setProfile((current) => ({ ...current, [field]: value }));
  }

  // updateContact modifica el contacto principal o de emergencia dentro del estado de la ficha.
  function updateContact(
    contactType: "primaryContact" | "emergencyContact",
    field: keyof Contact,
    value: string
  ) {
    setProfile((current) => ({
      ...current,
      [contactType]: { ...current[contactType], [field]: value },
    }));
  }

  // handleProfileSubmit guarda la ficha completa del residente, incluyendo contactos de emergencia.
  async function handleProfileSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedResident || !profileLoaded) return;

    const isoDateOfBirth = parseSpanishDate(profile.dateOfBirth);
    if (isoDateOfBirth === null) {
      setProfileError("Ingresa una fecha válida con el formato dd/mm/aaaa.");
      return;
    }
    if (isoDateOfBirth && isoDateOfBirth > localDateToISO(new Date())) {
      setProfileError("La fecha de nacimiento no puede ser posterior a hoy.");
      return;
    }

    setProfileError("");
    setProfileSuccess("");
    setProfileSaving(true);

    try {
      const response = await apiFetch(
        `/residents/${encodeURIComponent(selectedResident.id)}/profile`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ ...profile, dateOfBirth: isoDateOfBirth }),
        }
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo guardar la ficha.")
        );
      }

      const updatedProfile = (await response.json()) as Partial<ResidentProfile>;
      setProfile(profileForDisplay(updatedProfile));
      setProfileSuccess("Ficha actualizada correctamente.");
    } catch (err) {
      setProfileError(userFacingError(err, "No se pudo guardar la ficha."));
    } finally {
      setProfileSaving(false);
    }
  }

  const modules = [
    { id: "dashboard", label: "Resumen", permissions: ["dashboard.read"] },
    { id: "residents", label: "Residentes", permissions: ["resident.read"] },
    { id: "rooms", label: "Habitaciones", permissions: ["room.read"] },
    { id: "clinical", label: "Atención clínica", permissions: ["medical.read", "medication.read"] },
    { id: "billing", label: "Facturación", permissions: ["billing.read"] },
    { id: "documents", label: "Documentos", permissions: ["document.read"] },
    { id: "users", label: "Usuarios", permissions: ["user.read"] },
    { id: "audit", label: "Auditoría", permissions: ["audit.read"] },
  ].filter((module) => module.permissions.some((permission) => hasPermission(user, permission)));
  const currentView = modules.some((module) => module.id === activeView)
    ? activeView
    : modules[0]?.id ?? "none";
  const roleLabels: Record<string, string> = {
    admin: "Administración",
    manager: "Dirección",
    nurse: "Enfermería",
    kitchen: "Alimentación",
    reception: "Recepción",
    accounting: "Contabilidad",
  };
  const roomStatusLabels: Record<RoomStatus, string> = {
    available: "Disponible",
    maintenance: "En mantención",
    closed: "Cerrada",
  };
  const assignedResidentIds = new Set(rooms.flatMap((room) => room.occupantIds ?? []));
  const assignableResidents = residents.filter(
    (resident) => resident.status === "active" && !assignedResidentIds.has(resident.id)
  );

  if (authChecking) {
    return <main className="auth-loading" role="status">Verificando sesión...</main>;
  }

  if (!user) {
    return (
      <main className="login-layout">
        <section className="login-panel" aria-labelledby="login-title">
          <div className="brand-lockup"><span className="brand-mark">C</span><span>CIMA</span></div>
          <p className="eyebrow">Residencia de personas mayores</p>
          <h1 id="login-title">Ingreso al sistema</h1>
          <p className="login-intro">Acceso seguro a la operación y cuidado diario.</p>
          <form className="login-form" onSubmit={handleLogin}>
            <label htmlFor="login-username">Usuario</label>
            <input
              id="login-username"
              autoComplete="username"
              value={loginUsername}
              onChange={(event) => setLoginUsername(event.target.value)}
              required
              disabled={loginSaving}
            />
            <label htmlFor="login-password">Contraseña</label>
            <input
              id="login-password"
              type="password"
              autoComplete="current-password"
              value={loginPassword}
              onChange={(event) => setLoginPassword(event.target.value)}
              required
              disabled={loginSaving}
            />
            {loginError && <p className="error" role="alert">{loginError}</p>}
            <button className="primary-button" type="submit" disabled={loginSaving}>
              {loginSaving ? "Ingresando..." : "Ingresar"}
            </button>
          </form>
          <p className="login-footnote">El acceso y las funciones disponibles dependen de tu perfil.</p>
        </section>
        <aside className="login-aside" aria-label="CIMA">
          <div className="aside-content">
            <span className="aside-kicker">Cuidado coordinado</span>
            <h2>Una residencia.<br />Un equipo conectado.</h2>
            <p>Información operativa organizada para acompañar cada jornada.</p>
          </div>
          <span className="aside-index">GESTIÓN RESIDENCIAL · CHILE</span>
        </aside>
      </main>
    );
  }

  return (
    <main className="workspace">
      <aside className="sidebar">
        <div className="brand-lockup"><span className="brand-mark">C</span><span>CIMA</span></div>
        <p className="sidebar-caption">OPERACIONES</p>
        <nav className="module-nav" aria-label="Módulos">
          {modules.map((module) => (
            <button
              className={currentView === module.id ? "nav-item selected" : "nav-item"}
              key={module.id}
              type="button"
              onClick={() => setActiveView(module.id)}
              aria-current={currentView === module.id ? "page" : undefined}
            >
              <span className={`nav-glyph glyph-${module.id}`} aria-hidden="true" />
              {module.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-user">
          <span className="user-avatar">{user.username.slice(0, 1).toUpperCase()}</span>
          <span className="user-summary"><strong>{user.username}</strong><small>{roleLabels[user.role] ?? user.role}</small></span>
          <button className="logout-button" type="button" onClick={handleLogout} aria-label="Cerrar sesión" title="Cerrar sesión">↪</button>
        </div>
      </aside>
      <section className="main-panel">
        <header className="workspace-header">
          <div>
            <p className="eyebrow">{roleLabels[user.role] ?? user.role}</p>
            <h1 id="app-title">{modules.find((module) => module.id === currentView)?.label ?? "CIMA"}</h1>
          </div>
          <div className="header-user"><span className="online-dot" /> Sesión activa</div>
        </header>

        {currentView === "dashboard" && hasPermission(user, "dashboard.read") ? (
          <section className="dashboard-content" aria-labelledby="dashboard-title">
            <div className="welcome-row">
              <div><p className="eyebrow">PANEL DE CONTROL</p><h2 id="dashboard-title">Buenos días, {user.username}</h2><p>Resumen de tu acceso y operación de la residencia.</p></div>
              <span className="date-stamp">CIMA · CHILE</span>
            </div>
            {dashboardError && <p className="error" role="alert">{dashboardError}</p>}
            <div className="dashboard-metrics">
              <article className="metric-panel metric-residents"><span>Residentes registrados</span><strong>{hasPermission(user, "resident.read") ? residents.length : "—"}</strong><small>{hasPermission(user, "resident.read") ? "Acceso autorizado" : "Sin permiso de lectura"}</small></article>
              <article className="metric-panel metric-active"><span>Residentes activos</span><strong>{hasPermission(user, "resident.read") ? residents.filter((resident) => resident.status === "active").length : "—"}</strong><small>Estado de atención</small></article>
              <article className="metric-panel metric-access"><span>Estado del panel</span><strong>{dashboardStatus || "Conectando"}</strong><small>API protegida por sesión</small></article>
            </div>
            <section className="access-section" aria-labelledby="access-title">
              <div className="section-heading"><div><p className="eyebrow">ACCESO PERSONAL</p><h2 id="access-title">Módulos habilitados</h2></div><span className="access-count">{modules.length} módulos</span></div>
              <div className="module-grid">
                {modules.map((module) => (
                  <button className="module-tile" key={module.id} type="button" onClick={() => setActiveView(module.id)}>
                    <span className={`tile-glyph glyph-${module.id}`} aria-hidden="true" />
                    <span><strong>{module.label}</strong><small>{module.id === "dashboard" || module.id === "residents" ? "Abrir módulo" : "Permiso asignado"}</small></span>
                    <span className="tile-arrow" aria-hidden="true">↗</span>
                  </button>
                ))}
              </div>
            </section>
            <section className="permission-section" aria-labelledby="permission-title">
              <div className="section-heading"><div><p className="eyebrow">CONTROL DE ACCESO</p><h2 id="permission-title">Permisos de tu perfil</h2></div></div>
              <div className="permission-list">
                {(user.role === "admin" ? ["Acceso administrativo"] : user.permissions ?? []).map((permission) => <span className="permission-item" key={permission}>{permissionLabel(permission)}</span>)}
              </div>
            </section>
          </section>
        ) : currentView === "rooms" && hasPermission(user, "room.read") ? (
          <section className="resident-content" aria-label="Gestión de habitaciones">
            {hasPermission(user, "room.write") && (
              <section className="card" aria-labelledby="room-form-title">
                <h2 id="room-form-title">{editingRoomId ? "Editar habitación" : "Nueva habitación"}</h2>
                <form onSubmit={handleRoomSubmit}>
                  <label htmlFor="room-code">Código de habitación</label>
                  <input
                    id="room-code"
                    value={roomCode}
                    onChange={(event) => setRoomCode(event.target.value)}
                    maxLength={32}
                    required
                    disabled={roomsSaving}
                  />
                  <label htmlFor="room-capacity">Capacidad</label>
                  <input
                    id="room-capacity"
                    type="number"
                    min="1"
                    step="1"
                    value={roomCapacity}
                    onChange={(event) => setRoomCapacity(event.target.value)}
                    required
                    disabled={roomsSaving}
                  />
                  <label htmlFor="room-status">Estado</label>
                  <select
                    id="room-status"
                    value={roomStatus}
                    onChange={(event) => setRoomStatus(event.target.value as RoomStatus)}
                    disabled={roomsSaving}
                  >
                    <option value="available">Disponible</option>
                    <option value="maintenance">En mantención</option>
                    <option value="closed">Cerrada</option>
                  </select>
                  {roomError && <p className="error" role="alert">{roomError}</p>}
                  {roomSuccess && <p className="success" role="status">{roomSuccess}</p>}
                  <div className="room-form-actions">
                    <button type="submit" disabled={roomsSaving}>
                      {roomsSaving ? "Guardando..." : editingRoomId ? "Guardar cambios" : "Crear habitación"}
                    </button>
                    {editingRoomId && <button className="secondary-action" type="button" onClick={cancelRoomEdit}>Cancelar</button>}
                  </div>
                </form>
              </section>
            )}
            <section className="card" aria-labelledby="rooms-list-title">
              <div className="section-heading">
                <div><p className="eyebrow">ALOJAMIENTO</p><h2 id="rooms-list-title">Habitaciones</h2></div>
                <button type="button" onClick={() => void loadRooms()} disabled={roomsLoading}>
                  {roomsLoading ? "Actualizando..." : "Actualizar"}
                </button>
              </div>
              {roomError && !hasPermission(user, "room.write") && <p className="error" role="alert">{roomError}</p>}
              {roomsLoading ? (
                <p role="status">Cargando habitaciones...</p>
              ) : rooms.length === 0 ? (
                <p>No hay habitaciones registradas.</p>
              ) : (
                <ul className="resident-list">
                  {rooms.map((room) => (
                    <li key={room.id}>
                      <div className="room-record">
                        <div className="room-record-heading">
                          <div className="room-summary">
                            <strong>{room.code}</strong>
                            <span>Ocupación: {room.occupancy} de {room.capacity}</span>
                          </div>
                          <div className="resident-actions">
                            <span className={`status room-status-${room.status}`}>{roomStatusLabels[room.status]}</span>
                            {hasPermission(user, "room.write") && <button type="button" onClick={() => startRoomEdit(room)}>Editar</button>}
                          </div>
                        </div>
                        {room.occupantIds?.map((residentId) => {
                          const resident = residents.find((item) => item.id === residentId);
                          return (
                            <div className="room-occupant" key={residentId}>
                              <span>{resident ? `${resident.firstName} ${resident.lastName}` : `Residente ${residentId.slice(-6)}`}</span>
                              {hasPermission(user, "room.write") && hasPermission(user, "resident.read") && (
                                <button
                                  type="button"
                                  disabled={roomsSaving}
                                  onClick={() => void updateRoomAssignment(room.id, residentId, "release")}
                                >
                                  Liberar
                                </button>
                              )}
                            </div>
                          );
                        })}
                        {hasPermission(user, "room.write") && hasPermission(user, "resident.read") && room.status === "available" && (
                          <div className="room-assign-controls">
                            <label className="visually-hidden" htmlFor={`room-resident-${room.id}`}>Residente para {room.code}</label>
                            <select
                              id={`room-resident-${room.id}`}
                              value={roomResidentSelection[room.id] ?? ""}
                              onChange={(event) => setRoomResidentSelection((current) => ({ ...current, [room.id]: event.target.value }))}
                              disabled={roomsSaving || room.occupancy >= room.capacity}
                            >
                              <option value="">Seleccionar residente activo</option>
                              {assignableResidents.map((resident) => (
                                <option key={resident.id} value={resident.id}>{resident.firstName} {resident.lastName}</option>
                              ))}
                            </select>
                            <button
                              type="button"
                              disabled={roomsSaving || room.occupancy >= room.capacity || !roomResidentSelection[room.id]}
                              onClick={() => void updateRoomAssignment(room.id, roomResidentSelection[room.id], "assign")}
                            >
                              Asignar
                            </button>
                          </div>
                        )}
                        <button
                          className="room-history-toggle"
                          type="button"
                          onClick={() => visibleRoomHistory === room.id ? setVisibleRoomHistory("") : void loadRoomHistory(room.id)}
                        >
                          {visibleRoomHistory === room.id ? "Ocultar historial" : "Ver historial"}
                        </button>
                        {visibleRoomHistory === room.id && (
                          <ol className="room-history-list">
                            {(roomHistory[room.id] ?? []).length === 0 ? (
                              <li>Sin movimientos registrados.</li>
                            ) : (roomHistory[room.id] ?? []).map((entry, index) => {
                              const resident = residents.find((item) => item.id === entry.residentId);
                              const residentName = resident ? `${resident.firstName} ${resident.lastName}` : `Residente ${entry.residentId.slice(-6)}`;
                              const changedAt = new Intl.DateTimeFormat("es-CL", { dateStyle: "short", timeStyle: "short" }).format(new Date(entry.changedAt));
                              return <li key={`${entry.changedAt}-${index}`}>{changedAt}: {entry.action === "assigned" ? "Asignado" : "Liberado"} {residentName} · {entry.actor}</li>;
                            })}
                          </ol>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </section>
        ) : currentView === "residents" && hasPermission(user, "resident.read") ? (
          <section className="resident-content" aria-label="Gestión de residentes">
            {hasPermission(user, "resident.create") && (
        <>

        <section
          className="card"
          aria-labelledby="new-resident-title"
        >
          <h2 id="new-resident-title">Nuevo residente</h2>

          <form onSubmit={handleSubmit} noValidate>
            <label htmlFor="firstName">
              Nombre
            </label>

            <input
              id="firstName"
              name="firstName"
              value={firstName}
              onChange={(event) => setFirstName(event.target.value)}
              required
              autoComplete="given-name"
              disabled={saving}
            />

            <label htmlFor="lastName">
              Apellido
            </label>

            <input
              id="lastName"
              name="lastName"
              value={lastName}
              onChange={(event) => setLastName(event.target.value)}
              required
              autoComplete="family-name"
              disabled={saving}
            />

            {error && (
              <p className="error" role="alert">
                {error}
              </p>
            )}

            {success && (
              <p className="success" role="status">
                {success}
              </p>
            )}

            <button type="submit" disabled={saving}>
              {saving ? "Guardando..." : "Crear residente"}
            </button>
          </form>
        </section>
        </>
      )}

        <section
          className="card"
          aria-labelledby="resident-list-title"
        >
          <div className="section-heading">
            <h2 id="resident-list-title">
              Residentes
            </h2>

            <button
              type="button"
              onClick={() => void loadResidents()}
              disabled={loading}
            >
              {loading ? "Actualizando..." : "Actualizar"}
            </button>
          </div>

          {loading ? (
            <p role="status">
              Cargando residentes...
            </p>
          ) : residents.length === 0 ? (
            <p>
              No hay residentes registrados.
            </p>
          ) : (
            <ul className="resident-list">
              {residents.map((resident) => (
                <li key={resident.id}>
                  <div>
                    <strong>
                      {resident.firstName} {resident.lastName}
                    </strong>
                  </div>

                  <div className="resident-actions">
                    <span className={`status status-${resident.status}`}>
                      {resident.status === "active"
                        ? "Activo"
                        : resident.status === "inactive"
                          ? "Inactivo"
                          : resident.status === "archived"
                            ? "Archivado"
                            : resident.status}
                    </span>
                    <button
                      type="button"
                      onClick={() => void selectResident(resident)}
                      aria-pressed={selectedResident?.id === resident.id}
                    >
                      Ver ficha
                    </button>
                    {resident.status !== "archived" && hasPermission(user, "resident.delete") && (
                      <button
                        type="button"
                        onClick={() => void archiveResident(resident)}
                        aria-label={`Archivar a ${resident.firstName} ${resident.lastName}`}
                      >
                        Archivar
                      </button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>

        {selectedResident && (
          <section className="card profile-card" aria-labelledby="profile-title">
            <div className="section-heading">
              <div>
                <p className="eyebrow">Ficha del residente</p>
                <h2 id="profile-title">
                  {selectedResident.firstName} {selectedResident.lastName}
                </h2>
              </div>
              <button
                type="button"
                onClick={() => void selectResident(selectedResident)}
                disabled={profileLoading || profileSaving}
              >
                {profileLoading ? "Cargando..." : "Recargar ficha"}
              </button>
            </div>

            {hasPermission(user, "resident.update") && <form
              className="resident-identity-form"
              onSubmit={handleResidentUpdate}
            >
              <fieldset disabled={residentSaving}>
                <legend>Identificación</legend>
                <div className="profile-fields">
                  <label htmlFor="resident-edit-first-name">Nombre</label>
                  <input
                    id="resident-edit-first-name"
                    autoComplete="given-name"
                    value={editFirstName}
                    onChange={(event) => setEditFirstName(event.target.value)}
                    required
                  />
                  <label htmlFor="resident-edit-last-name">Apellido</label>
                  <input
                    id="resident-edit-last-name"
                    autoComplete="family-name"
                    value={editLastName}
                    onChange={(event) => setEditLastName(event.target.value)}
                    required
                  />
                </div>
              </fieldset>
              {residentError && (
                <p className="error" role="alert">
                  {residentError}
                </p>
              )}
              {residentSuccess && (
                <p className="success" role="status">
                  {residentSuccess}
                </p>
              )}
              <button type="submit" disabled={residentSaving}>
                {residentSaving ? "Guardando..." : "Guardar identificación"}
              </button>
            </form>}

            {profileLoading ? (
              <p role="status">Cargando ficha...</p>
            ) : !profileLoaded ? (
              <p className="error" role="alert">
                {profileError || "No se pudo cargar la ficha."}
              </p>
            ) : (
              hasPermission(user, "resident.update") ? <form onSubmit={handleProfileSubmit}>
                <fieldset disabled={profileSaving}>
                  <legend>Datos personales</legend>
                  <div className="profile-fields">
                    <label htmlFor="dateOfBirth">Fecha de nacimiento</label>
                    <div className="date-input-wrap">
                      <input
                        id="dateOfBirth"
                        type="text"
                        inputMode="numeric"
                        autoComplete="bday"
                        placeholder="dd/mm/aaaa"
                        maxLength={10}
                        aria-describedby="date-of-birth-format"
                        value={profile.dateOfBirth}
                        onChange={(event) =>
                          updateProfile("dateOfBirth", formatDateInput(event.target.value))
                        }
                      />
                      <button
                        className="date-picker-button"
                        type="button"
                        onClick={() => datePickerRef.current?.showPicker()}
                        aria-label="Abrir calendario"
                        title="Abrir calendario"
                      >
                        <span className="calendar-icon" aria-hidden="true" />
                      </button>
                      <input
                        ref={datePickerRef}
                        className="native-date-picker"
                        type="date"
                        aria-label="Seleccionar fecha de nacimiento en el calendario"
                        lang="es-CL"
                        max={localDateToISO(new Date())}
                        value={parseSpanishDate(profile.dateOfBirth) ?? ""}
                        onChange={(event) =>
                          updateProfile(
                            "dateOfBirth",
                            formatDateForDisplay(event.target.value)
                          )
                        }
                        tabIndex={-1}
                      />
                      <span id="date-of-birth-format" className="date-format-hint">Escribe o elige día/mes/año</span>
                    </div>

                    <label htmlFor="profilePhone">Teléfono</label>
                    <input
                      id="profilePhone"
                      type="tel"
                      autoComplete="tel"
                      value={profile.phone}
                      onChange={(event) =>
                        updateProfile("phone", event.target.value)
                      }
                    />

                    <label htmlFor="profileEmail">Correo electrónico</label>
                    <input
                      id="profileEmail"
                      type="email"
                      autoComplete="email"
                      value={profile.email}
                      onChange={(event) =>
                        updateProfile("email", event.target.value)
                      }
                    />
                  </div>
                </fieldset>

                {(["primaryContact", "emergencyContact"] as const).map(
                  (contactType) => (
                    <fieldset key={contactType} disabled={profileSaving}>
                      <legend>
                        {contactType === "primaryContact"
                          ? "Contacto principal"
                          : "Contacto de emergencia"}
                      </legend>
                      <div className="profile-fields">
                        <label htmlFor={`${contactType}-name`}>Nombre</label>
                        <input
                          id={`${contactType}-name`}
                          autoComplete="name"
                          value={profile[contactType].name}
                          onChange={(event) =>
                            updateContact(contactType, "name", event.target.value)
                          }
                        />

                        <label htmlFor={`${contactType}-relationship`}>
                          Relación
                        </label>
                        <input
                          id={`${contactType}-relationship`}
                          value={profile[contactType].relationship}
                          onChange={(event) =>
                            updateContact(
                              contactType,
                              "relationship",
                              event.target.value
                            )
                          }
                        />

                        <label htmlFor={`${contactType}-phone`}>Teléfono</label>
                        <input
                          id={`${contactType}-phone`}
                          type="tel"
                          autoComplete="tel"
                          value={profile[contactType].phone}
                          onChange={(event) =>
                            updateContact(contactType, "phone", event.target.value)
                          }
                        />

                        <label htmlFor={`${contactType}-email`}>
                          Correo electrónico
                        </label>
                        <input
                          id={`${contactType}-email`}
                          type="email"
                          autoComplete="email"
                          value={profile[contactType].email}
                          onChange={(event) =>
                            updateContact(contactType, "email", event.target.value)
                          }
                        />
                      </div>
                    </fieldset>
                  )
                )}

                {profileError && (
                  <p className="error" role="alert">
                    {profileError}
                  </p>
                )}

                {profileSuccess && (
                  <p className="success" role="status">
                    {profileSuccess}
                  </p>
                )}

                <button type="submit" disabled={profileSaving}>
                  {profileSaving ? "Guardando..." : "Guardar ficha"}
                </button>
              </form> : <p className="read-only-note">Tu perfil permite consultar la ficha, pero no modificarla.</p>
            )}
          </section>
        )}
          </section>
        ) : currentView !== "none" ? (
          <section className="module-placeholder">
            <p className="eyebrow">PERMISO CONFIRMADO</p>
            <h2>{modules.find((module) => module.id === currentView)?.label}</h2>
            <p>Este módulo está habilitado para tu perfil. Su flujo de trabajo se incorporará en una próxima etapa.</p>
          </section>
        ) : (
          <section className="module-placeholder">
            <p className="eyebrow">SIN MÓDULOS ASIGNADOS</p>
            <h2>Contacta a administración</h2>
            <p>Tu cuenta todavía no tiene permisos para consultar módulos.</p>
          </section>
        )}
      </section>
    </main>
  );
}