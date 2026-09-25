import { FormEvent, useCallback, useEffect, useState } from "react";

type ResidentStatus = "active" | "inactive";

type Resident = {
  id: string;
  firstName: string;
  lastName: string;
  status: ResidentStatus | string;
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

const emptyContact: Contact = {
  name: "",
  relationship: "",
  phone: "",
  email: "",
};

const emptyProfile: ResidentProfile = {
  dateOfBirth: "",
  phone: "",
  email: "",
  primaryContact: emptyContact,
  emergencyContact: emptyContact,
};

const API_URL =
  import.meta.env.VITE_API_URL ?? "http://localhost:8080/api/v1";

async function getApiError(response: Response, fallback: string) {
  try {
    const data = (await response.json()) as ApiError;
    return data.error ?? fallback;
  } catch {
    return fallback;
  }
}

export default function App() {
  const [residents, setResidents] = useState<Resident[]>([]);
  const [selectedResident, setSelectedResident] = useState<Resident | null>(null);
  const [profile, setProfile] = useState<ResidentProfile>(emptyProfile);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [profileLoading, setProfileLoading] = useState(false);
  const [profileSaving, setProfileSaving] = useState(false);
  const [profileLoaded, setProfileLoaded] = useState(false);

  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [profileError, setProfileError] = useState("");
  const [profileSuccess, setProfileSuccess] = useState("");

  const loadResidents = useCallback(async () => {
    setLoading(true);
    setError("");

    try {
      const response = await fetch(`${API_URL}/residents/`);

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
      setError(
        err instanceof Error
          ? err.message
          : "No se pudieron cargar los residentes."
      );
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadResidents();
  }, [loadResidents]);

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
      const response = await fetch(`${API_URL}/residents/`, {
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
      setError(
        err instanceof Error
          ? err.message
          : "No se pudo crear el residente."
      );
    } finally {
      setSaving(false);
    }
  }

  async function selectResident(resident: Resident) {
    setSelectedResident(resident);
    setProfile(emptyProfile);
    setProfileLoaded(false);
    setProfileLoading(true);
    setProfileError("");
    setProfileSuccess("");

    try {
      const response = await fetch(
        `${API_URL}/residents/${encodeURIComponent(resident.id)}/profile`
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo cargar la ficha.")
        );
      }

      const data = (await response.json()) as Partial<ResidentProfile>;
      setProfile({
        ...emptyProfile,
        ...data,
        primaryContact: { ...emptyContact, ...data.primaryContact },
        emergencyContact: { ...emptyContact, ...data.emergencyContact },
      });
      setProfileLoaded(true);
    } catch (err) {
      setProfileError(
        err instanceof Error ? err.message : "No se pudo cargar la ficha."
      );
    } finally {
      setProfileLoading(false);
    }
  }

  function updateProfile(field: "dateOfBirth" | "phone" | "email", value: string) {
    setProfile((current) => ({ ...current, [field]: value }));
  }

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

  async function handleProfileSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!selectedResident || !profileLoaded) return;

    setProfileError("");
    setProfileSuccess("");
    setProfileSaving(true);

    try {
      const response = await fetch(
        `${API_URL}/residents/${encodeURIComponent(selectedResident.id)}/profile`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(profile),
        }
      );

      if (!response.ok) {
        throw new Error(
          await getApiError(response, "No se pudo guardar la ficha.")
        );
      }

      const updatedProfile = (await response.json()) as Partial<ResidentProfile>;
      setProfile({
        ...emptyProfile,
        ...updatedProfile,
        primaryContact: { ...emptyContact, ...updatedProfile.primaryContact },
        emergencyContact: { ...emptyContact, ...updatedProfile.emergencyContact },
      });
      setProfileSuccess("Ficha actualizada correctamente.");
    } catch (err) {
      setProfileError(
        err instanceof Error ? err.message : "No se pudo guardar la ficha."
      );
    } finally {
      setProfileSaving(false);
    }
  }

  return (
    <main className="page">
      <section className="container" aria-labelledby="app-title">
        <header>
          <p className="eyebrow">CIMA</p>

          <h1 id="app-title">
            Control Interno y Monitoreo de Adultos Mayores
          </h1>

          <p>
            Gestión de residentes y seguimiento operacional.
          </p>
        </header>

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
                      {resident.status === "active" ? "Activo" : resident.status}
                    </span>
                    <button
                      type="button"
                      onClick={() => void selectResident(resident)}
                      aria-pressed={selectedResident?.id === resident.id}
                    >
                      Ver ficha
                    </button>
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

            {profileLoading ? (
              <p role="status">Cargando ficha...</p>
            ) : !profileLoaded ? (
              <p className="error" role="alert">
                {profileError || "No se pudo cargar la ficha."}
              </p>
            ) : (
              <form onSubmit={handleProfileSubmit}>
                <fieldset disabled={profileSaving}>
                  <legend>Datos personales</legend>
                  <div className="profile-fields">
                    <label htmlFor="dateOfBirth">Fecha de nacimiento</label>
                    <input
                      id="dateOfBirth"
                      type="date"
                      value={profile.dateOfBirth}
                      onChange={(event) =>
                        updateProfile("dateOfBirth", event.target.value)
                      }
                    />

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
              </form>
            )}
          </section>
        )}
      </section>
    </main>
  );
}