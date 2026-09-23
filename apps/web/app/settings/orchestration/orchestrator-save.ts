import type {
  Orchestrator,
  OrchestratorConfiguration,
  OrchestratorPatch,
} from "@/lib/api/domains/orchestration-api";

/** Fields PATCH may change without a full replace, even while the orchestrator works. */
const PATCHABLE = [
  "display_name",
  "ask_before_create",
  "auto_comment_source",
  "auto_move_source_done",
] as const satisfies readonly (keyof OrchestratorPatch)[];

const REPLACED = ["role_id", "profile_id", "executor_preference", "context"] as const;

export function initialConfiguration(
  item: Orchestrator | undefined,
  defaultRoleId: string,
): OrchestratorConfiguration {
  const base = item ?? {
    role_id: defaultRoleId,
    profile_id: "",
    executor_preference: "",
    context: "",
  };
  return {
    role_id: base.role_id,
    profile_id: base.profile_id,
    executor_preference: base.executor_preference,
    context: base.context,
    // Older backends omit these; fall back to the documented defaults.
    display_name: item?.display_name ?? "",
    ask_before_create: item?.ask_before_create ?? false,
    auto_comment_source: item?.auto_comment_source ?? true,
    auto_move_source_done: item?.auto_move_source_done ?? false,
  };
}

/** Trims the display name the way the backend stores it. */
export function normalizedConfiguration(value: OrchestratorConfiguration) {
  return { ...value, display_name: value.display_name.trim() };
}

/**
 * The PATCH body when only identity or behavior fields changed, or null when a
 * replaced field changed too and the save needs a full PUT.
 */
export function patchFor(
  saved: OrchestratorConfiguration,
  value: OrchestratorConfiguration,
): OrchestratorPatch | null {
  if (REPLACED.some((key) => saved[key] !== value[key])) return null;
  const next = normalizedConfiguration(value);
  const body: OrchestratorPatch = {};
  for (const key of PATCHABLE) {
    if (saved[key] !== next[key]) Object.assign(body, { [key]: next[key] });
  }
  return body;
}
