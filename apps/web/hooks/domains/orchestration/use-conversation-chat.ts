import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import {
  getConversation,
  getConversationCommentPage,
  type ConversationTask,
} from "@/lib/api/domains/orchestration-conversation-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { PagedWindow } from "@/lib/orchestration/conversation-comment-window";
import { useSessionLiveSyncSubscriptions } from "@/app/office/tasks/[id]/use-session-live-sync";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { TaskSession as APISession } from "@/lib/types/http";

export const ACTIVE_POLL_MS = 1000;
export const BUSY_POLL_MS = 3000;
export const IDLE_POLL_MS = 10000;

const ACTIVE_SESSION_STATES = new Set(["CREATED", "STARTING", "RUNNING"]);

type Metadata = { task: ConversationTask; sessions: APISession[] };

export function conversationPollInterval(
  comments: TaskComment[],
  sessions: APISession[],
  connected: boolean,
) {
  const awaitingReply = comments.some(
    (comment) =>
      comment.authorType === "user" &&
      (comment.runStatus === "queued" || comment.runStatus === "claimed"),
  );
  if (awaitingReply) return ACTIVE_POLL_MS;
  if (!connected || sessions.some((session) => ACTIVE_SESSION_STATES.has(session.state)))
    return BUSY_POLL_MS;
  return IDLE_POLL_MS;
}

const metadataSignature = (value: Metadata) =>
  JSON.stringify([
    value.task,
    value.sessions.map((session) => [session.id, session.state, session.updated_at]),
  ]);

function useConversationMetadata(taskId: string) {
  const store = useAppStoreApi();
  const [state, setState] = useState<{ data?: Metadata; error?: unknown }>({});
  const signature = useRef<string>(undefined);
  const pending = useRef(false);
  const load = useCallback(async () => {
    if (pending.current) return;
    pending.current = true;
    const activityEpochs = { ...store.getState().taskSessions.activityEpochBySession };
    try {
      const [task, result] = await Promise.all([getConversation(taskId), listTaskSessions(taskId)]);
      const data = { task, sessions: result.sessions ?? [] };
      const next = metadataSignature(data);
      if (next === signature.current) {
        setState((prev) => (prev.error ? { data: prev.data } : prev));
        return;
      }
      signature.current = next;
      store.getState().setTaskSessionsForTask(taskId, data.sessions, activityEpochs);
      setState({ data });
    } catch (error) {
      setState((prev) => ({ ...prev, error }));
    } finally {
      pending.current = false;
    }
  }, [store, taskId]);
  return { ...state, load };
}

/**
 * Reads one orchestration conversation: a paged comment window, the task and
 * its sessions. Polling runs only while `visible` and the document is shown,
 * fastest while a user message awaits its reply; a comment-created event
 * refreshes at once.
 */
export function useConversationChat(taskId: string, visible = true) {
  const connection = useAppStore((s) => s.connection.status);
  const view = useMemo(
    () =>
      new PagedWindow<TaskComment>(async (after, signal) => {
        const page = await getConversationCommentPage(taskId, after, signal);
        return { entries: page.comments, next_cursor: page.next_cursor };
      }),
    [taskId],
  );
  const snapshot = useSyncExternalStore(view.subscribe, view.getSnapshot, view.getSnapshot);
  const metadata = useConversationMetadata(taskId);
  const loadMetadata = metadata.load;
  const refresh = useCallback(async () => {
    await Promise.all([loadMetadata(), view.refresh()]);
  }, [loadMetadata, view]);
  useEffect(() => {
    view.activate();
    return view.dispose;
  }, [view]);
  const sessions = metadata.data?.sessions;
  const interval = conversationPollInterval(
    snapshot.entries,
    sessions ?? [],
    connection === "connected",
  );
  useEffect(() => {
    if (!visible) return;
    const poll = () => {
      if (!document.hidden) void refresh();
    };
    poll();
    const timer = window.setInterval(poll, interval);
    document.addEventListener("visibilitychange", poll);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", poll);
    };
  }, [visible, interval, refresh]);
  // A comment written outside this chat (an agent reply, an API client)
  // arrives as a live event, so it shows without waiting for the idle poll.
  useOfficeRefetch(`comments:${taskId}`, () => {
    if (visible && !document.hidden) void refresh();
  });
  const sessionIds = useMemo(() => sessions?.map((session) => session.id) ?? [], [sessions]);
  useSessionLiveSyncSubscriptions({ connectionStatus: connection, taskId, sessionIds });
  const comments = useMemo(
    () =>
      snapshot.entries
        .slice()
        .sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id)),
    [snapshot.entries],
  );
  return {
    comments,
    nextCursor: snapshot.nextCursor,
    loadingMore: snapshot.loading && snapshot.loaded,
    loaded: snapshot.loaded,
    metadata: metadata.data,
    error: snapshot.error || metadata.error,
    refresh,
    loadMore: view.loadMore,
  };
}
