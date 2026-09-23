import { fetchJson } from "../client";
import { generateUUID } from "@/lib/utils";
import type { CommentTransport } from "@/components/task/simple/comment-transport";
import type { RecoveryOutcome, RecoveryTarget } from "@/components/task/simple/recovery-transport";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
export type ConversationTask = {
  id: string;
  title: string;
  workspace_id: string;
  orchestrator_id: string;
};
type CommentDTO = {
  id: string;
  task_id: string;
  author_id: string;
  author_type: "user" | "agent";
  run_id?: string;
  run_status?: TaskComment["runStatus"];
  run_error?: string;
  body: string;
  created_at: string;
  source?: string;
  receipt_status?: string;
};
const path = (id: string) => `/api/v1/orchestration/tasks/${encodeURIComponent(id)}`;
export const getConversation = (id: string) => fetchJson<ConversationTask>(path(id));
function mapComment(row: CommentDTO): TaskComment {
  // Intake is durable before the scheduler creates a run. Keep that short
  // window visible in the transcript so a sent message never looks ignored.
  // Once a run exists, run_status remains authoritative (including failures).
  const runStatus =
    row.run_status ??
    (row.author_type === "user" && row.receipt_status === "expired" ? "failed" : undefined) ??
    (row.author_type === "user" &&
    (row.receipt_status === "accepted" || row.receipt_status === "queued")
      ? "queued"
      : undefined);
  return {
    id: row.id,
    taskId: row.task_id,
    authorId: row.author_id,
    authorType: row.author_type,
    runId: row.run_id,
    runStatus,
    runError: row.run_error,
    authorName: "",
    content: row.body,
    createdAt: row.created_at,
    source: row.source,
  };
}
export async function getConversationCommentPage(id: string, before = "", signal?: AbortSignal) {
  const query = new URLSearchParams({ before, limit: "50" });
  const result = await fetchJson<{ comments: CommentDTO[]; next_cursor: string }>(
    `${path(id)}/comments?${query}`,
    { init: { signal } },
  );
  return {
    comments: (result.comments ?? []).map(mapComment),
    next_cursor: result.next_cursor ?? "",
  };
}
export function createConversationSender(conversationId: string): CommentTransport {
  let pending: { body: string; id: string } | undefined;
  let inflight: Promise<unknown> | undefined;
  return async (id, body) => {
    if (id !== conversationId) throw new Error("Conversation identity changed");
    if (inflight) {
      if (pending?.body !== body.body) throw new Error("Another message is awaiting a receipt");
      return inflight;
    }
    if (!pending || pending.body !== body.body) pending = { body: body.body, id: generateUUID() };
    const attempt = pending;
    inflight = postConversationComment(id, { body: attempt.body, client_message_id: attempt.id });
    try {
      const result = await inflight;
      if (pending === attempt) pending = undefined;
      return result;
    } finally {
      inflight = undefined;
    }
  };
}
export const postConversationComment = (
  id: string,
  body: { body: string; client_message_id?: string },
) => fetchJson(`${path(id)}/comments`, { init: { method: "POST", body: JSON.stringify(body) } });

/**
 * Retries a failed conversation turn, named by its session or, for a turn
 * that never bound a session, by its run id.
 */
export async function retryConversation(
  id: string,
  target: RecoveryTarget,
  action: "resume" | "fresh_start",
): Promise<RecoveryOutcome> {
  const result = await fetchJson<{ status?: string }>(`${path(id)}/retry`, {
    init: {
      method: "POST",
      body: JSON.stringify({ session_id: target.sessionId, run_id: target.runId, action }),
    },
  });
  return result?.status === "already_queued" ? "already_queued" : "queued";
}
