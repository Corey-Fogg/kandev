import type { ReactNode } from "react";
import { OrchestrationRolesPage } from "@/app/settings/orchestration/roles-page";
import { OrchestratorsPage } from "@/app/settings/orchestration/orchestrators-page";
import { OrchestratorEditor } from "@/app/settings/orchestration/orchestrator-editor";
import { WorkspaceSettingsShell } from "@/components/settings/workspaces/workspace-settings-shell";
import { safeDecodePathSegment } from "@/lib/routing/path";

const WORKSPACE_ORCHESTRATION = /^\/settings\/workspaces\/([^/]+)\/orchestration(?:\/([^/]+))?$/;

/** Global roles and the per-workspace orchestrator list and editor. */
export function renderOrchestrationSettingsRoute(pathname: string): ReactNode {
  if (pathname === "/settings/orchestration") return <OrchestrationRolesPage />;
  const match = pathname.match(WORKSPACE_ORCHESTRATION);
  const workspaceId = safeDecodePathSegment(match?.[1]);
  if (!match || !workspaceId) return null;
  const orchestratorId = match[2] ? safeDecodePathSegment(match[2]) : undefined;
  if (orchestratorId === null) return null;
  return (
    <WorkspaceSettingsShell workspaceId={workspaceId} activeTab="orchestration">
      {orchestratorId ? (
        <OrchestratorEditor workspaceId={workspaceId} id={orchestratorId} />
      ) : (
        <OrchestratorsPage workspaceId={workspaceId} />
      )}
    </WorkspaceSettingsShell>
  );
}
