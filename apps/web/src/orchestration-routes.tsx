import { CoordinatorPage } from "@/app/coordinator/coordinator-page";
import { OrchestrationConversationRoute } from "@/app/settings/orchestration/conversation-route";
import { matchSingle } from "@/lib/routing/path";

export type OrchestrationSpaRoute =
  | { kind: "orchestrationConversation"; taskId: string }
  | { kind: "coordinator"; workspaceId: string };

export function resolveOrchestrationRoute(normalized: string): OrchestrationSpaRoute | null {
  const workspaceId = matchSingle(normalized, /^\/workspaces\/([^/]+)\/coordinator$/);
  if (workspaceId) return { kind: "coordinator", workspaceId };
  const taskId = matchSingle(normalized, /^\/workspace\/conversations\/([^/]+)$/);
  if (taskId) return { kind: "orchestrationConversation", taskId };
  return null;
}

export function isOrchestrationRoute(route: { kind: string }): route is OrchestrationSpaRoute {
  return route.kind === "coordinator" || route.kind === "orchestrationConversation";
}

export function OrchestrationRoute({ route }: { route: OrchestrationSpaRoute }) {
  if (route.kind === "coordinator") return <CoordinatorPage workspaceId={route.workspaceId} />;
  return <OrchestrationConversationRoute taskId={route.taskId} />;
}
