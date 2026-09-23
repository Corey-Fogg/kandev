import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { coordinatorHref } from "@/lib/api/domains/orchestration-api";
import { selectCoordinatedPendingInputCount } from "@/lib/orchestration/coordinated-pending-input";
import { useRouter } from "@/lib/routing/client-router";
import { AppSidebarNavItem } from "./app-sidebar-nav-item";

type NavProps = { collapsed?: boolean; onNavigate?: () => void };

/** The active workspace's Coordinator entry, shown only while orchestration is enabled. */
export function OrchestrationNav({ collapsed = false, onNavigate }: NavProps) {
  const enabled = useAppStore((s) => s.features.orchestration);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  if (!enabled || !workspaceId) return null;
  return (
    <CoordinatorLink workspaceId={workspaceId} collapsed={collapsed} onNavigate={onNavigate} />
  );
}

function CoordinatorLink({
  workspaceId,
  collapsed,
  onNavigate,
}: NavProps & { workspaceId: string; collapsed: boolean }) {
  const { t } = useTranslation();
  const router = useRouter();
  const pendingInput = useAppStore(selectCoordinatedPendingInputCount);
  const href = coordinatorHref(workspaceId);
  return (
    <div data-testid="workspace-orchestration-nav">
      <AppSidebarNavItem
        icon={IconSitemap}
        label={t("orchestration:coordinator")}
        href={href}
        collapsed={collapsed}
        badge={pendingInput}
        onClick={
          onNavigate
            ? () => {
                router.push(href);
                onNavigate();
              }
            : undefined
        }
        testId="workspace-coordinator-link"
      />
    </div>
  );
}
