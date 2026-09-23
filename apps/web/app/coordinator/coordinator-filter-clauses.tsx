import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import {
  DIMENSION_METAS,
  getDimensionEnumOptions,
  getDimensionMeta,
  getOpLabel,
} from "@/components/task/sidebar-filter/filter-dimension-registry";
import type { MultiSelectOption } from "@/components/task/sidebar-filter/filter-multi-select";
import { TypedFilterClauseEditor } from "@/components/task/sidebar-filter/typed-filter-clause-editor";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { getExecutorLabel } from "@/lib/executor-icons";
import { repositorySlug } from "@/lib/repository-slug";
import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";
import { sidebarViewName } from "@/lib/state/slices/ui/sidebar-view-builtins";
import type {
  FilterClause,
  FilterDimension,
  FilterOp,
} from "@/lib/state/slices/ui/sidebar-view-types";
import type { Task } from "@/lib/types/http";
import { generateUUID } from "@/lib/utils";
import { CoordinatorSelect } from "./coordinator-select";

type Options = Partial<Record<FilterDimension, MultiSelectOption[]>>;

/** Choices come from the coordinator's own catalog, not the board snapshots. */
function useCatalogOptions(catalog: CoordinatorWorkspace, tasks: Task[]): Options {
  const { i18n } = useTranslation();
  return useMemo(() => {
    const workflows = catalog.workflows.filter(
      (flow) => flow.id !== catalog.workspace.office_workflow_id,
    );
    const names = new Map(workflows.map((flow) => [flow.id, flow.name]));
    const steps = catalog.steps
      .filter((step) => names.has(step.workflow_id))
      .sort(
        (a, b) =>
          (names.get(a.workflow_id) ?? "").localeCompare(names.get(b.workflow_id) ?? "") ||
          a.position - b.position,
      )
      .map((step) => ({
        value: step.id,
        label: step.name,
        color: step.color,
        group: names.get(step.workflow_id),
      }));
    const executors = [...new Set(tasks.map((task) => task.primary_executor_type).filter(Boolean))]
      .map(String)
      .sort()
      .map((type) => ({ value: type, label: getExecutorLabel(type) }));
    return {
      workflow: workflows.map((flow) => ({ value: flow.id, label: flow.name })),
      workflowStep: steps,
      repository: catalog.repositories.map((repo) => {
        const slug = repositorySlug(repo);
        return { value: slug, label: slug };
      }),
      executorType: executors,
    };
    // The language is a dependency so executor labels follow a locale switch.
  }, [catalog, tasks, i18n.language]);
}

function newClause(): FilterClause {
  const meta = DIMENSION_METAS[0];
  return {
    id: generateUUID(),
    dimension: meta.dimension,
    op: meta.defaultOp,
    value: meta.defaultValue,
  };
}

function SidebarViewCopy({
  workspaceId,
  onCopy,
}: {
  workspaceId: string;
  onCopy: (clauses: FilterClause[]) => void;
}) {
  const { t } = useTranslation();
  const views = useAppStore((s) => selectSidebarViews(s, workspaceId).views);
  if (views.length === 0) return null;
  return (
    <CoordinatorSelect
      label={t("orchestration:applySidebarView")}
      value="none"
      onChange={(id) => {
        const view = views.find((item) => item.id === id);
        if (view) onCopy(view.filters.map((clause) => ({ ...clause, id: generateUUID() })));
      }}
      options={[
        { id: "none", name: t("orchestration:applySidebarViewPlaceholder") },
        ...views.map((view) => ({ id: view.id, name: sidebarViewName(view, t) })),
      ]}
      testId="coordinator-copy-sidebar-view"
    />
  );
}

/** The sidebar's filter clauses, applied to the Coordinator view's tasks. */
export function CoordinatorFilterClauses({
  catalog,
  tasks,
  clauses,
  onChange,
  mobile,
}: {
  catalog: CoordinatorWorkspace;
  tasks: Task[];
  clauses: FilterClause[];
  onChange: (clauses: FilterClause[]) => void;
  mobile: boolean;
}) {
  const { t } = useTranslation();
  const options = useCatalogOptions(catalog, tasks);
  const optionsFor = (dimension: FilterDimension) =>
    getDimensionEnumOptions(getDimensionMeta(dimension)) ?? options[dimension] ?? [];
  const control = "cursor-pointer max-md:min-h-11";
  return (
    <div className="space-y-2" data-testid="coordinator-filter-clauses">
      {clauses.map((clause) => (
        <div key={clause.id} className="overflow-x-auto">
          <TypedFilterClauseEditor
            clause={clause}
            dimensions={DIMENSION_METAS}
            getMeta={getDimensionMeta}
            getDimensionLabel={(dimension) => t(getDimensionMeta(dimension).labelKey)}
            getOpLabel={(op, valueKind) => getOpLabel(op as FilterOp, valueKind)}
            optionsForDimension={optionsFor}
            onChange={(next) =>
              onChange(
                clauses.map((item) => (item.id === clause.id ? (next as FilterClause) : item)),
              )
            }
            onRemove={() => onChange(clauses.filter((item) => item.id !== clause.id))}
            mobile={mobile}
          />
        </div>
      ))}
      <div className="flex flex-wrap items-end gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          className={control}
          onClick={() => onChange([...clauses, newClause()])}
        >
          {t("orchestration:addFilter")}
        </Button>
        {clauses.length > 0 && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className={control}
            onClick={() => onChange([])}
          >
            {t("orchestration:clearFilters")}
          </Button>
        )}
        <div className="min-w-0 flex-1">
          <SidebarViewCopy workspaceId={catalog.workspace.id} onCopy={onChange} />
        </div>
      </div>
    </div>
  );
}
