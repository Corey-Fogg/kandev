import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Orchestrator } from "@/lib/api/domains/orchestration-api";
import {
  existingOrchestratorId,
  initialConfiguration,
  normalizedConfiguration,
  patchFor,
} from "./orchestrator-save";
import { validDisplayName } from "./orchestrator-identity-fields";

const saved = initialConfiguration(
  {
    role_id: "chief-of-staff",
    profile_id: "p",
    executor_preference: '{"executor_profile_id":"e"}',
    context: "ctx",
    display_name: "",
    ask_before_create: false,
    auto_comment_source: true,
    auto_move_source_done: false,
  } as Orchestrator,
  "",
);

describe("orchestrator save", () => {
  it("defaults behavior settings for a new or older orchestrator", () => {
    expect(initialConfiguration(undefined, "role")).toMatchObject({
      role_id: "role",
      display_name: "",
      ask_before_create: false,
      auto_comment_source: true,
      auto_move_source_done: false,
    });
  });

  it("patches only changed identity and behavior fields, trimming the name", () => {
    expect(patchFor(saved, { ...saved, display_name: "  Jeb ", ask_before_create: true })).toEqual({
      display_name: "Jeb",
      ask_before_create: true,
    });
    expect(patchFor(saved, saved)).toEqual({});
  });

  it("falls back to a full replace when a configuration field changed", () => {
    expect(patchFor(saved, { ...saved, display_name: "Jeb", context: "new" })).toBeNull();
    expect(normalizedConfiguration({ ...saved, display_name: " Jeb " }).display_name).toBe("Jeb");
  });

  it("recognizes the single-orchestrator conflict", () => {
    const conflict = new ApiError("conflict", 409, {
      error: "orchestrator_exists",
      orchestrator_id: "chief",
    });
    expect(existingOrchestratorId(conflict)).toBe("chief");
    expect(existingOrchestratorId(new ApiError("busy", 409, { error: "working" }))).toBeNull();
    expect(existingOrchestratorId(new Error("x"))).toBeNull();
  });

  it("limits display names to 60 characters after trimming", () => {
    expect(validDisplayName("")).toBe(true);
    expect(validDisplayName(`  ${"a".repeat(60)}  `)).toBe(true);
    expect(validDisplayName("a".repeat(61))).toBe(false);
  });
});
