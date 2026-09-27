import { describe, expect, it } from "vitest";
import { hasPermission, type AuthUser } from "./auth";

const admin: AuthUser = { username: "admin", role: "admin", permissions: [] };
const nurse: AuthUser = {
  username: "enfermeria",
  role: "nurse",
  permissions: ["resident.read", "resident.update", "medical.read", "medical.write", "medication.read", "medication.write"],
};

describe("auth permissions", () => {
  it("admin can access every protected module", () => {
    expect(hasPermission(admin, "resident.read")).toBe(true);
    expect(hasPermission(admin, "dashboard.read")).toBe(true);
    expect(hasPermission(admin, "user.write")).toBe(true);
  });

  it("nurse can access clinical modules but not user management", () => {
    expect(hasPermission(nurse, "medical.write")).toBe(true);
    expect(hasPermission(nurse, "user.read")).toBe(false);
    expect(hasPermission(nurse, "dashboard.read")).toBe(false);
  });
});
