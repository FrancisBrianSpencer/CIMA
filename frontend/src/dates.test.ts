import { describe, expect, it } from "vitest";
import {
  formatDateForDisplay,
  formatDateInput,
  localDateToISO,
  parseSpanishDate,
} from "./dates";

describe("formato de fecha chileno", () => {
  it("presenta fechas ISO como día/mes/año", () => {
    expect(formatDateForDisplay("2024-02-09")).toBe("09/02/2024");
  });

  it("convierte fechas chilenas válidas al formato de la API", () => {
    expect(parseSpanishDate("29/02/2024")).toBe("2024-02-29");
    expect(parseSpanishDate("")).toBe("");
  });

  it("rechaza fechas inexistentes y formatos ambiguos", () => {
    expect(parseSpanishDate("29/02/2023")).toBeNull();
    expect(parseSpanishDate("02/29/2024")).toBeNull();
  });

  it("agrega separadores mientras se escriben los dígitos", () => {
    expect(formatDateInput("2")).toBe("2");
    expect(formatDateInput("2507")).toBe("25/07");
    expect(formatDateInput("25071920")).toBe("25/07/1920");
    expect(formatDateInput("25/07/192099")).toBe("25/07/1920");
  });

  it("obtiene la fecha local en ISO sin aplicar desplazamiento horario", () => {
    expect(localDateToISO(new Date(2024, 0, 2, 23, 30))).toBe("2024-01-02");
  });
});