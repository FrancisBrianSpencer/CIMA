// formatDateForDisplay convierte una fecha ISO de la API al formato chileno día/mes/año.
export function formatDateForDisplay(isoDate: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(isoDate);
  if (!match) return isoDate;

  const [, year, month, day] = match;
  const displayDate = `${day}/${month}/${year}`;
  return parseSpanishDate(displayDate) === isoDate ? displayDate : isoDate;
}

// formatDateInput agrega separadores a medida que se escriben los ocho dígitos de la fecha.
export function formatDateInput(value: string) {
  const digits = value.replace(/\D/g, "").slice(0, 8);
  if (digits.length <= 2) return digits;
  if (digits.length <= 4) return `${digits.slice(0, 2)}/${digits.slice(2)}`;
  return `${digits.slice(0, 2)}/${digits.slice(2, 4)}/${digits.slice(4)}`;
}

// localDateToISO obtiene la fecha local sin desplazarla por la zona horaria del navegador.
export function localDateToISO(date: Date) {
  const year = date.getFullYear().toString().padStart(4, "0");
  const month = (date.getMonth() + 1).toString().padStart(2, "0");
  const day = date.getDate().toString().padStart(2, "0");
  return `${year}-${month}-${day}`;
}

// parseSpanishDate valida una fecha día/mes/año y la adapta al formato ISO del backend.
export function parseSpanishDate(displayDate: string): string | null {
  const trimmedDate = displayDate.trim();
  if (trimmedDate === "") return "";

  const match = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(trimmedDate);
  if (!match) return null;

  const [, dayText, monthText, yearText] = match;
  const day = Number(dayText);
  const month = Number(monthText);
  const year = Number(yearText);
  if (year < 1 || month < 1 || month > 12) return null;

  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysPerMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (day < 1 || day > daysPerMonth[month - 1]) return null;

  return `${yearText}-${monthText}-${dayText}`;
}