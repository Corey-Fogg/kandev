import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import {
  getTaskProposal,
  listTaskProposals,
  type TaskProposal,
} from "@/lib/api/domains/orchestration-proposals-api";

const DECIDED_RANK: Record<TaskProposal["status"], number> = {
  pending: 0,
  approving: 1,
  approved: 2,
  dismissed: 2,
};

/**
 * Keeps the more decided of two copies of one proposal, so a list read that
 * started before a decision cannot put an approved card back to pending.
 */
export function mergeProposal(current: TaskProposal | undefined, next: TaskProposal) {
  if (!current) return next;
  return DECIDED_RANK[next.status] >= DECIDED_RANK[current.status] ? next : current;
}

/** `listedFor` is the proposal comment set the latest committed list read was requested for. */
type State = {
  scope: string;
  byId: Map<string, TaskProposal>;
  listedFor: string | null;
  /** Proposal comments whose proposal could not be read. */
  unavailable: ReadonlySet<string>;
};

/** Proposal comment ids in transcript order. */
function proposalCommentIds(comments: TaskComment[]) {
  return comments.filter((comment) => comment.source === "proposal").map((comment) => comment.id);
}

const AWAITING: readonly string[] = ["pending", "approving"];
const EMPTY = new Map<string, TaskProposal>();
const NONE: ReadonlySet<string> = new Set();

function newestCommentId(comments: TaskComment[]) {
  let newest: TaskComment | undefined;
  for (const comment of comments) {
    if (!newest || comment.createdAt >= newest.createdAt) newest = comment;
  }
  return newest?.id ?? "";
}

/**
 * The task proposals behind a coordinator conversation's proposal comments. It
 * re-reads when a proposal comment appears or the newest comment changes, reads
 * an older proposal missing from the list once, and forgets everything when the
 * workspace or orchestrator changes.
 */
export function useConversationProposals(
  workspaceId: string,
  orchestratorId: string,
  comments: TaskComment[],
) {
  const scope = workspaceId && orchestratorId ? `${workspaceId}\n${orchestratorId}` : "";
  const [state, setState] = useState<State>({
    scope,
    byId: EMPTY,
    listedFor: null,
    unavailable: NONE,
  });
  const generation = useRef(0);
  const fetched = useRef<{ scope: string; ids: Set<string> }>({ scope, ids: new Set() });
  const ids = useMemo(() => proposalCommentIds(comments), [comments]);
  const idsKey = [...ids].sort().join("\n");
  const idsKeyRef = useRef(idsKey);
  idsKeyRef.current = idsKey;
  const newest = useMemo(() => newestCommentId(comments), [comments]);
  const current = useMemo(
    () =>
      state.scope === scope ? state : { scope, byId: EMPTY, listedFor: null, unavailable: NONE },
    [state, scope],
  );

  const replace = useCallback(
    (proposal: TaskProposal) =>
      setState((prev) => {
        if (prev.scope !== scope) return prev;
        const byId = new Map(prev.byId);
        byId.set(proposal.id, mergeProposal(byId.get(proposal.id), proposal));
        return { ...prev, byId };
      }),
    [scope],
  );

  const read = useCallback(
    async (listedFor: string) => {
      if (!scope) return;
      const requested = ++generation.current;
      try {
        const { proposals } = await listTaskProposals(workspaceId, orchestratorId, "all");
        if (requested !== generation.current) return;
        setState((prev) => {
          const byId = new Map(prev.scope === scope ? prev.byId : undefined);
          for (const proposal of proposals ?? [])
            byId.set(proposal.id, mergeProposal(byId.get(proposal.id), proposal));
          const unavailable = prev.scope === scope ? prev.unavailable : NONE;
          return { scope, byId, listedFor, unavailable };
        });
      } catch {
        // A failed read keeps what is shown; the next comment change retries.
      }
    },
    [scope, workspaceId, orchestratorId],
  );
  const refresh = useCallback(() => read(idsKeyRef.current), [read]);

  useEffect(() => {
    void read(idsKey);
  }, [read, idsKey, newest]);

  useEffect(() => {
    if (fetched.current.scope !== scope) fetched.current = { scope, ids: new Set() };
    // Only ids the latest list read should have covered, so a new comment waits for its list.
    if (!scope || current.listedFor !== idsKey) return;
    for (const id of ids) {
      if (current.byId.has(id) || fetched.current.ids.has(id)) continue;
      fetched.current.ids.add(id);
      getTaskProposal(workspaceId, orchestratorId, id).then(replace, () =>
        setState((prev) =>
          prev.scope === scope
            ? { ...prev, unavailable: new Set([...prev.unavailable, id]) }
            : prev,
        ),
      );
    }
  }, [scope, ids, idsKey, current, workspaceId, orchestratorId, replace]);

  const pendingIds = ids.filter((id) => AWAITING.includes(current.byId.get(id)?.status ?? ""));
  return {
    byId: current.byId,
    unavailable: current.unavailable,
    pendingCount: pendingIds.length,
    firstPendingId: pendingIds[0] ?? "",
    refresh,
    replace,
  };
}
