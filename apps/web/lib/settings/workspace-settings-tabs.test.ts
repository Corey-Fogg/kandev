import { describe, it, expect } from "vitest";
import {
  WORKSPACE_SETTINGS_TABS,
  workspaceTabVisible,
  workspaceSettingsHref,
} from "./workspace-settings-tabs";
describe("workspace orchestration navigation", () => {
  it("is independent from legacy Office", () => {
    expect(workspaceTabVisible("orchestration", {})).toBe(false);
    expect(workspaceTabVisible("orchestration", { orchestration: true })).toBe(true);
    expect(workspaceTabVisible("repositories", {})).toBe(true);
  });
  it("uses one workspace-scoped destination", () => {
    expect(WORKSPACE_SETTINGS_TABS.filter((t) => t.tab === "orchestration")).toHaveLength(1);
    expect(workspaceSettingsHref("one", "orchestration")).toBe(
      "/settings/workspaces/one/orchestration",
    );
  });
});
