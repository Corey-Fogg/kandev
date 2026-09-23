import { useCallback } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { CommentRenderer } from "@/components/task/simple/comment-renderer-context";
import { useConversationProposals } from "@/hooks/domains/orchestration/use-conversation-proposals";
import { useChatMotion } from "@/hooks/use-chat-motion";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import { ProposalCard } from "./proposal-card";

/** Renders proposal comments as decision cards and reports the ones awaiting a decision. */
export function useProposalRenderer(
  workspaceId: string,
  orchestratorId: string,
  comments: TaskComment[],
) {
  const proposals = useConversationProposals(workspaceId, orchestratorId, comments);
  const { byId, unavailable, replace } = proposals;
  const renderer = useCallback<CommentRenderer>(
    (comment) =>
      comment.source === "proposal" ? (
        <ProposalCard
          comment={comment}
          proposal={byId.get(comment.id)}
          unavailable={unavailable.has(comment.id)}
          workspaceId={workspaceId}
          orchestratorId={orchestratorId}
          onChange={replace}
        />
      ) : null,
    [byId, unavailable, replace, workspaceId, orchestratorId],
  );
  return {
    renderer,
    hasProposals: proposals.count > 0,
    pendingCount: proposals.pendingCount,
    firstPendingId: proposals.firstPendingId,
  };
}

/** Scrolls to a proposal card and moves focus to it. */
export function focusProposalCard(targetId: string, smooth: boolean) {
  const wrapper = document.getElementById(`comment-${targetId}`);
  const card = wrapper?.querySelector<HTMLElement>(`[data-proposal-id]`) ?? wrapper;
  card?.scrollIntoView({ behavior: smooth ? "smooth" : "auto", block: "center" });
  card?.focus({ preventScroll: true });
}

/** Jumps to the first proposal card that still needs a decision. */
export function ProposalsBanner({ count, targetId }: { count: number; targetId: string }) {
  const { t } = useTranslation();
  const motion = useChatMotion();
  if (count === 0) return null;
  return (
    <Button
      type="button"
      variant="outline"
      className="mb-3 w-full cursor-pointer justify-start max-md:min-h-11"
      onClick={() => focusProposalCard(targetId, motion)}
      data-testid="proposals-pending-banner"
    >
      {t("orchestration:proposalsPending", { count })}
    </Button>
  );
}
