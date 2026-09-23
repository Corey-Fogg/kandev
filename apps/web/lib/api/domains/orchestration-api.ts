import { fetchJson } from "../client";
export type OrchestratorRole = { id: string; name: string; icon?: string; instructions: string };
export type OrchestratorConfiguration = {
  role_id: string;
  profile_id: string;
  executor_preference: string;
  context: string;
  /** The instance name; empty inherits the role name. */
  display_name: string;
  ask_before_create: boolean;
  auto_comment_source: boolean;
  auto_move_source_done: boolean;
};
/** Fields PATCH may change, including while the orchestrator is working. */
export type OrchestratorPatch = Partial<
  Pick<
    OrchestratorConfiguration,
    "display_name" | "ask_before_create" | "auto_comment_source" | "auto_move_source_done"
  >
>;
export type Orchestrator = OrchestratorConfiguration & {
  /** The effective name: the display name when set, otherwise the role name. */
  name: string;
  role_name: string;
  icon?: string;
  instructions: string;
  id: string;
  workspace_id: string;
  status: string;
};
export type OrchestrationProfile = { id: string; name: string; agent_id: string };
const base = "/api/v1/orchestration";
const workspace = (id: string) => `${base}/workspaces/${encodeURIComponent(id)}`;
const instances = (id: string) => `${workspace(id)}/orchestrators`;
const json = (method: string, body?: unknown) => ({
  init: { method, ...(body === undefined ? {} : { body: JSON.stringify(body) }) },
});
export const listOrchestrators = (ws: string) =>
  fetchJson<{ orchestrators: Orchestrator[] }>(instances(ws));
export const getOrchestrator = (ws: string, id: string) =>
  fetchJson<Orchestrator>(`${instances(ws)}/${encodeURIComponent(id)}`);
export const saveOrchestrator = (
  ws: string,
  id: string | undefined,
  body: OrchestratorConfiguration,
) =>
  fetchJson<Orchestrator>(
    `${instances(ws)}${id ? `/${encodeURIComponent(id)}` : ""}`,
    json(id ? "PUT" : "POST", body),
  );
export const patchOrchestrator = (ws: string, id: string, body: OrchestratorPatch) =>
  fetchJson<Orchestrator>(`${instances(ws)}/${encodeURIComponent(id)}`, json("PATCH", body));
/** Outcome metrics for one orchestrator; nullable values are unknown in the window. */
export type CoordinatorMetrics = {
  days: number;
  since: string;
  delegated: number;
  completed: number;
  failed: number;
  truncated: boolean;
  success_rate: number | null;
  merged_prs: number | null;
  cycle_time_samples: number;
  cycle_time_median_hours: number | null;
  cycle_time_p90_hours: number | null;
  cost_usd: number;
  cost_per_merged_pr_usd: number | null;
  unpriced_event_count: number;
};
export const getOrchestratorMetrics = (
  ws: string,
  id: string,
  days: 7 | 30,
  init?: { signal?: AbortSignal },
) =>
  fetchJson<CoordinatorMetrics>(
    `${instances(ws)}/${encodeURIComponent(id)}/metrics?days=${days}`,
    init ? { init } : undefined,
  );
export const deleteOrchestrator = (ws: string, id: string) =>
  fetchJson(`${instances(ws)}/${encodeURIComponent(id)}`, json("DELETE"));
export const setOrchestratorStatus = (ws: string, id: string, status: string) =>
  fetchJson(`${instances(ws)}/${encodeURIComponent(id)}/status`, json("POST", { status }));
export const openOrchestratorConversation = (ws: string, id: string) =>
  fetchJson<{ task_id: string }>(
    `${instances(ws)}/${encodeURIComponent(id)}/conversation`,
    json("POST"),
  );
export const listOrchestrationProfiles = (ws: string) =>
  fetchJson<{ profiles: OrchestrationProfile[] }>(`${workspace(ws)}/profiles`);
export const listOrchestratorRoles = () =>
  fetchJson<{ roles: OrchestratorRole[] }>(`${base}/roles`);
export const saveOrchestratorRole = (role: Omit<OrchestratorRole, "id"> & { id?: string }) =>
  fetchJson<OrchestratorRole>(
    `${base}/roles${role.id ? `/${encodeURIComponent(role.id)}` : ""}`,
    json(role.id ? "PUT" : "POST", role),
  );
export const deleteOrchestratorRole = (id: string) =>
  fetchJson(`${base}/roles/${encodeURIComponent(id)}`, json("DELETE"));
export const orchestratorsHref = (ws: string) =>
  `/settings/workspaces/${encodeURIComponent(ws)}/orchestration`;
export const orchestratorHref = (ws: string, id: string) =>
  `${orchestratorsHref(ws)}/${encodeURIComponent(id)}`;
export const conversationHref = (task: string, ws?: string) =>
  `/workspace/conversations/${encodeURIComponent(task)}${ws ? `?workspaceId=${encodeURIComponent(ws)}` : ""}`;
export const orchestratorConversationHref = (ws: string, id: string, task: string) =>
  `${conversationHref(task, ws)}&orchestratorId=${encodeURIComponent(id)}`;

export const listOrchestratedTasks = (ws: string, id: string) =>
  fetchJson<{ tasks: { id: string; title: string; state: string }[] }>(
    `${instances(ws)}/${encodeURIComponent(id)}/tasks`,
  );
export function selectedExecutor(raw: string): string {
  try {
    return JSON.parse(raw || "{}").executor_profile_id || "";
  } catch {
    return "";
  }
}

export const coordinatorHref = (workspaceId: string, orchestratorId?: string) =>
  `/workspaces/${encodeURIComponent(workspaceId)}/coordinator${orchestratorId !== undefined ? `?orchestratorId=${encodeURIComponent(orchestratorId)}` : ""}`;
