import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSearchParams, useRouter } from "@/lib/routing/client-router";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { coordinatorHref } from "@/lib/api/domains/orchestration-api";

const selections = new Map<string, string>();
let selectionOwner: string | undefined;

export function resolveCoordinatorSelection(
  assignments: { id: string }[],
  requested: string | null,
  remembered?: string,
): string {
  if (requested !== null) return assignments.some((item) => item.id === requested) ? requested : "";
  if (
    remembered !== undefined &&
    (remembered === "" || assignments.some((item) => item.id === remembered))
  )
    return remembered;
  return assignments.length === 1 ? assignments[0].id : "";
}

/**
 * Whether the page should write a selection it resolved without the URL
 * (remembered, or the only orchestrator) into the URL, so the left navigation
 * can mark the selected orchestrator active. With one orchestrator the entry
 * is active anyway.
 */
export function selectionNeedsUrl(
  assignmentCount: number,
  requested: string | null,
  selected: string,
) {
  return requested === null && selected !== "" && assignmentCount > 1;
}

export function useCoordinatorSelection(catalog: CoordinatorWorkspace) {
  const params = useSearchParams();
  const router = useRouter();
  const owner = useAppStore((s) => s.auth.user?.id);
  if (owner !== selectionOwner) {
    selections.clear();
    selectionOwner = owner;
  }
  const workspace = catalog.workspace.id;
  const requested = params.get("orchestratorId");
  const selected = resolveCoordinatorSelection(
    catalog.assignments,
    requested,
    selections.get(workspace),
  );
  const needsUrl = selectionNeedsUrl(catalog.assignments.length, requested, selected);
  useEffect(() => {
    if (selected || requested === "") selections.set(workspace, selected);
    if (needsUrl) router.replace(coordinatorHref(workspace, selected), { scroll: false });
  }, [workspace, requested, selected, needsUrl, router]);
  const choose = (id: string) => {
    const value = id === "none" ? "" : id;
    selections.set(workspace, value);
    router.replace(coordinatorHref(workspace, value), { scroll: false });
  };
  return { requested, selected, choose };
}
