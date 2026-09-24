import { FormEvent, useEffect, useState } from "react";

type Resident = {
  id?: string;
  firstName: string;
  lastName: string;
  status: string;
};

const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080/api/v1";

export default function App() {
  const [residents, setResidents] = useState<Resident[]>([]);
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [error, setError] = useState("");

  async function loadResidents() {
    const response = await fetch(`${API_URL}/residents/`);
    if (!response.ok) throw new Error("No se pudieron cargar los residentes.");
    setResidents(await response.json());
  }

  useEffect(() => {
    loadResidents().catch((err: Error) => setError(err.message));
  }, []);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");

    const cleanFirstName = firstName.trim();
    const cleanLastName = lastName.trim();

    if (!cleanFirstName || !cleanLastName) {
      setError("Nombre y apellido son obligatorios.");
      return;
    }

    const response = await fetch(`${API_URL}/residents/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ firstName: cleanFirstName, lastName: cleanLastName }),
    });

    if (!response.ok) {
      setError("No se pudo crear el residente.");
      return;
    }

    setFirstName("");
    setLastName("");
    await loadResidents();
  }

  return (
    <main className="page">
      <section className="container" aria-labelledby="app-title">
        <header>
          <p className="eyebrow">CIMA</p>
          <h1 id="app-title">Control Interno y Monitoreo de Adultos Mayores</h1>
          <p>Base inicial del sistema de gestión de residentes.</p>
        </header>

        <section className="card" aria-labelledby="new-resident-title">
          <h2 id="new-resident-title">Nuevo residente</h2>
          <form onSubmit={handleSubmit} noValidate>
            <label htmlFor="firstName">Nombre</label>
            <input id="firstName" name="firstName" value={firstName}
              onChange={(event) => setFirstName(event.target.value)}
              required autoComplete="given-name" />

            <label htmlFor="lastName">Apellido</label>
            <input id="lastName" name="lastName" value={lastName}
              onChange={(event) => setLastName(event.target.value)}
              required autoComplete="family-name" />

            {error && <p className="error" role="alert">{error}</p>}
            <button type="submit">Crear residente</button>
          </form>
        </section>

        <section className="card" aria-labelledby="resident-list-title">
          <h2 id="resident-list-title">Residentes</h2>
          {residents.length === 0 ? (
            <p>No hay residentes registrados.</p>
          ) : (
            <ul className="resident-list">
              {residents.map((resident) => (
                <li key={resident.id}>
                  <strong>{resident.firstName} {resident.lastName}</strong>
                  <span>{resident.status}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      </section>
    </main>
  );
}
