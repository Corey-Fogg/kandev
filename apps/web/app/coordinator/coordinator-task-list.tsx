import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useCoordinatorTasks } from "@/hooks/domains/orchestration/use-coordinator-tasks";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { coordinatorTaskGroup } from "@/lib/orchestration/coordinator-task-groups";
import {
  activeClauses,
  applyCoordinatorFilters,
  catalogLookup,
  loadCoordinatorFilters,
  saveCoordinatorFilters,
  serverParams,
} from "@/lib/orchestration/coordinator-task-filters";
import { CoordinatorTaskGroups } from "./coordinator-task-groups";
import { TaskFilters } from "./coordinator-task-filters";
import { coordinatorTaskTotals } from "@/lib/orchestration/coordinator-task-totals";

/** Filters are remembered per workspace in this browser. */
function useCoordinatorFilterState(workspaceId: string) {
  const [filters, setFilters] = useState(() => loadCoordinatorFilters(workspaceId));
  useEffect(() => saveCoordinatorFilters(workspaceId, filters), [workspaceId, filters]);
  return [filters, setFilters] as const;
}

export function CoordinatorTaskList({
  catalog,
  selected,
}: {
  catalog: CoordinatorWorkspace;
  selected: string;
}) {
  const { t } = useTranslation();
  const [filters, setFilters] = useCoordinatorFilterState(catalog.workspace.id);
  const lookup = useMemo(() => catalogLookup(catalog.repositories), [catalog.repositories]);
  const view = useCoordinatorTasks(catalog.workspace.id, serverParams(filters, lookup));
  const acknowledged = useAppStore((s) => s.acknowledgedAgentErrors);
  const dismissed = useAppStore((s) => s.dismissedAgentErrors);
  const rows = useMemo(
    () =>
      applyCoordinatorFilters(view.tasks, filters, selected, lookup).map((task) => ({
        task,
        group: coordinatorTaskGroup(task, acknowledged, dismissed),
      })),
    [view.tasks, filters, selected, lookup, acknowledged, dismissed],
  );
  const clientFiltered = activeClauses(filters.clauses).length > 0;
  return (
    <section
      className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain pb-[env(safe-area-inset-bottom)]"
      aria-label={t("orchestration:tasksTab")}
    >
      <TaskFilters
        catalog={catalog}
        tasks={view.tasks}
        filters={filters}
        setFilters={setFilters}
        selected={selected}
      />
      <div className="border-y bg-muted/30 px-4 py-3 text-sm space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p>{t("orchestration:taskCoverage", { loaded: view.tasks.length, total: view.total })}</p>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void view.refresh()}
            disabled={view.loading}
            className="cursor-pointer max-md:min-h-11"
          >
            {t("task:refresh")}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t("orchestration:knownTaskTotals", coordinatorTaskTotals(rows.map(({ task }) => task)))}
        </p>
        {!view.complete && (
          <p className="text-xs text-muted-foreground">{t("orchestration:partialCoverage")}</p>
        )}
        {!view.complete && clientFiltered && (
          <p
            className="text-xs text-muted-foreground"
            data-testid="coordinator-filters-loaded-only"
          >
            {t("orchestration:filtersApplyToLoaded")}
          </p>
        )}
        {view.stale && (
          <p role="alert" className="text-amber-600">
            {t("orchestration:staleTasks")}
          </p>
        )}
        {view.loading && <p role="status">{t("common:loading")}</p>}
      </div>
      <div className="space-y-6 p-4">
        {!view.loading && !view.stale && rows.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {t(view.complete ? "orchestration:noTaskMatches" : "orchestration:noLoadedMatches")}
          </p>
        )}
        <CoordinatorTaskGroups rows={rows} catalog={catalog} groupFilter={filters.group} />
        {!view.complete && (
          <Button
            variant="outline"
            onClick={() => void view.loadMore()}
            disabled={view.loading || view.stale}
            className="cursor-pointer w-full max-md:min-h-11"
          >
            {t("orchestration:loadMoreTasks")}
          </Button>
        )}
      </div>
    </section>
  );
}
