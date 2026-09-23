import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { Input } from "@kandev/ui/input";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { COORDINATOR_GROUPS } from "@/lib/orchestration/coordinator-task-groups";
import {
  activeClauses,
  type CoordinatorFilterState,
} from "@/lib/orchestration/coordinator-task-filters";
import type { Task } from "@/lib/types/http";
import { CoordinatorSelect } from "./coordinator-select";
import { CoordinatorFilterClauses } from "./coordinator-filter-clauses";

export function TaskFilters({
  catalog,
  tasks,
  filters,
  setFilters,
  selected,
}: {
  catalog: CoordinatorWorkspace;
  tasks: Task[];
  filters: CoordinatorFilterState;
  setFilters: (next: CoordinatorFilterState) => void;
  selected: string;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const patch = (next: Partial<CoordinatorFilterState>) => setFilters({ ...filters, ...next });
  const active = activeClauses(filters.clauses).length;
  return (
    <div className="grid grid-cols-2 gap-3 p-4">
      <div className="col-span-2">
        <label className="text-xs text-muted-foreground" htmlFor="coordinator-search">
          {t("orchestration:searchTasks")}
        </label>
        <Input
          id="coordinator-search"
          value={filters.query}
          onChange={(e) => patch({ query: e.target.value })}
          className="mt-1 max-md:min-h-11"
        />
      </div>
      <details open={!isMobile} className="col-span-2">
        <summary className="md:hidden cursor-pointer min-h-11 flex items-center gap-2 text-sm">
          {t("task:filters")}
          {active > 0 && (
            <span className="text-xs text-muted-foreground">
              {t("orchestration:filtersActive", { count: active })}
            </span>
          )}
        </summary>
        <div className="grid grid-cols-2 gap-3">
          <div className="col-span-2">
            <CoordinatorFilterClauses
              catalog={catalog}
              tasks={tasks}
              clauses={filters.clauses}
              onChange={(clauses) => patch({ clauses })}
              mobile={isMobile}
            />
          </div>
          <CoordinatorSelect
            label={t("orchestration:taskScope")}
            value={filters.scope}
            onChange={(scope) => patch({ scope: scope === "coordinated" ? "coordinated" : "all" })}
            options={[
              { id: "all", name: t("orchestration:allTasks") },
              ...(selected || filters.scope === "coordinated"
                ? [{ id: "coordinated", name: t("orchestration:selectedTasks") }]
                : []),
            ]}
          />
          <CoordinatorSelect
            label={t("orchestration:groupFilter")}
            value={filters.group}
            onChange={(group) =>
              patch({ group: COORDINATOR_GROUPS.find((item) => item === group) ?? "all" })
            }
            options={[
              { id: "all", name: t("orchestration:allGroups") },
              ...COORDINATOR_GROUPS.map((id) => ({ id, name: t(`orchestration:group_${id}`) })),
            ]}
          />
        </div>
      </details>
    </div>
  );
}
