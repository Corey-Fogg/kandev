import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useWorkspaceOrchestrators } from "@/hooks/domains/orchestration/use-orchestrator-conversation";
import { coordinatorHref, type Orchestrator } from "@/lib/api/domains/orchestration-api";
import { selectCoordinatedPendingInputCount } from "@/lib/orchestration/coordinated-pending-input";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type NavProps = { collapsed?: boolean; onNavigate?: () => void };
type LinkProps = { collapsed: boolean; onNavigate?: () => void };

/**
 * The active workspace's orchestrator entries, shown only while orchestration
 * is enabled. Each listed orchestrator gets its own entry by name; a generic
 * Coordinator entry stands in while the list loads or when there is none.
 */
export function OrchestrationNav({ collapsed = false, onNavigate }: NavProps) {
  const enabled = useAppStore((s) => s.features.orchestration);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  if (!enabled || !workspaceId) return null;
  return (
    <OrchestratorLinks
      key={workspaceId}
      workspaceId={workspaceId}
      collapsed={collapsed}
      onNavigate={onNavigate}
    />
  );
}

function OrchestratorLinks({
  workspaceId,
  collapsed,
  onNavigate,
}: LinkProps & { workspaceId: string }) {
  const { data } = useWorkspaceOrchestrators(workspaceId);
  const orchestrators = data?.orchestrators ?? [];
  return (
    <div data-testid="workspace-orchestration-nav">
      {orchestrators.length === 0 ? (
        <CoordinatorLink workspaceId={workspaceId} collapsed={collapsed} onNavigate={onNavigate} />
      ) : (
        orchestrators.map((orchestrator) => (
          <OrchestratorNavItem
            key={orchestrator.id}
            workspaceId={workspaceId}
            orchestrator={orchestrator}
            only={orchestrators.length === 1}
            collapsed={collapsed}
            onNavigate={onNavigate}
          />
        ))
      )}
    </div>
  );
}

function useNavigate(href: string, onNavigate?: () => void) {
  const router = useRouter();
  if (!onNavigate) return undefined;
  return () => {
    router.push(href);
    onNavigate();
  };
}

function CoordinatorLink({
  workspaceId,
  collapsed,
  onNavigate,
}: LinkProps & { workspaceId: string }) {
  const { t } = useTranslation();
  const pendingInput = useAppStore((s) => selectCoordinatedPendingInputCount(s));
  const href = coordinatorHref(workspaceId);
  return (
    <AppSidebarNavItem
      icon={IconSitemap}
      label={t("orchestration:coordinator")}
      href={href}
      collapsed={collapsed}
      badge={pendingInput}
      badgeDescription={t("orchestration:navPendingInput", { count: pendingInput })}
      onClick={useNavigate(href, onNavigate)}
      testId="workspace-coordinator-link"
    />
  );
}

/** With several entries only the selected orchestrator's entry is active. */
function useEntryActive(workspaceId: string, orchestratorId: string, only: boolean) {
  const pathname = usePathname();
  const params = useSearchParams();
  if (pathname !== coordinatorHref(workspaceId)) return false;
  return only || params.get("orchestratorId") === orchestratorId;
}

function OrchestratorNavItem({
  workspaceId,
  orchestrator,
  only,
  collapsed,
  onNavigate,
}: LinkProps & { workspaceId: string; orchestrator: Orchestrator; only: boolean }) {
  const { t } = useTranslation();
  const active = useEntryActive(workspaceId, orchestrator.id, only);
  const pendingInput = useAppStore((s) => selectCoordinatedPendingInputCount(s, orchestrator.id));
  const href = coordinatorHref(workspaceId, orchestrator.id);
  const label =
    orchestrator.status === "paused"
      ? t("orchestration:orchestratorPausedLabel", { name: orchestrator.name })
      : orchestrator.name;
  return (
    <AppSidebarNavItem
      icon={IconSitemap}
      label={label}
      href={href}
      collapsed={collapsed}
      badge={pendingInput}
      badgeDescription={t("orchestration:navPendingInput", { count: pendingInput })}
      onClick={useNavigate(href, onNavigate)}
      isActive={active}
      testId={`workspace-coordinator-link-${orchestrator.id}`}
    />
  );
}
