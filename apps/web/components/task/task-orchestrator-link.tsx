import { createContext, useContext, type ReactNode } from "react";
import { IconSitemap } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import Link from "@/components/routing/app-link";
import { orchestratorHref } from "@/lib/api/domains/orchestration-api";
import type { Task } from "@/lib/types/http";

const TaskOrchestrationContext = createContext<Task | null>(null);

function coordinatorId(task: Task | null): string | null {
  const id = task?.metadata?.orchestration_chief_id;
  return typeof id === "string" && id ? id : null;
}

/**
 * Frames a coordinator-managed task with a link back to its coordinator. Any
 * other task, or any task while orchestration is off, renders `children` as is.
 */
export function OrchestratedTaskFrame({
  task,
  advanced = false,
  children,
}: {
  task: Task | null;
  advanced?: boolean;
  children: ReactNode;
}) {
  const enabled = useFeature("orchestration");
  if (!enabled || !task || !coordinatorId(task)) return children;
  return (
    <TaskOrchestrationContext.Provider value={task}>
      <div className="flex flex-col h-full min-h-0">
        <div className={advanced ? "hidden md:block" : undefined}>
          <TaskOrchestratorLink task={task} />
        </div>
        <div className="flex-1 min-h-0">{children}</div>
      </div>
    </TaskOrchestrationContext.Provider>
  );
}

export function MobileTaskOrchestratorLink() {
  const task = useContext(TaskOrchestrationContext);
  if (!task) return null;
  return <TaskOrchestratorLink task={task} compact />;
}

function TaskOrchestratorLink({ task, compact = false }: { task: Task | null; compact?: boolean }) {
  const { t } = useTranslation();
  const enabled = useFeature("orchestration");
  const id = coordinatorId(task);
  return enabled && task && id ? (
    <Link
      aria-label={t("orchestration:coordinatedBy")}
      className={
        compact
          ? "flex min-h-11 min-w-11 items-center justify-center rounded-md hover:bg-accent"
          : "block px-4 py-2 text-sm underline border-b"
      }
      href={orchestratorHref(task.workspace_id, id)}
    >
      {compact ? <IconSitemap className="size-4" aria-hidden /> : t("orchestration:coordinatedBy")}
    </Link>
  ) : null;
}
