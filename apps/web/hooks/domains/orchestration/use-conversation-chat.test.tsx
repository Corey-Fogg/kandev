import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { TaskComment } from "@/app/office/tasks/[id]/types";
import type { TaskSession } from "@/lib/types/http";
import {
  ACTIVE_POLL_MS,
  BUSY_POLL_MS,
  IDLE_POLL_MS,
  conversationPollInterval,
  useConversationChat,
} from "./use-conversation-chat";

const api = vi.hoisted(() => ({
  conversation: vi.fn(),
  page: vi.fn(),
  sessions: vi.fn(),
}));
vi.mock("@/lib/api/domains/orchestration-conversation-api", () => ({
  getConversation: api.conversation,
  getConversationCommentPage: api.page,
}));
vi.mock("@/lib/api/domains/session-api", () => ({ listTaskSessions: api.sessions }));
vi.mock("@/app/office/tasks/[id]/use-session-live-sync", () => ({
  useSessionLiveSyncSubscriptions: () => {},
}));

const comment = (patch: Partial<TaskComment>): TaskComment => ({
  id: "c",
  taskId: "t",
  authorId: "u",
  authorType: "user",
  authorName: "",
  content: "Hello",
  createdAt: "2026-09-23T00:00:00Z",
  ...patch,
});
const session = (state: string) => ({ id: "s", state }) as TaskSession;
const wrapper = ({ children }: { children: ReactNode }) => (
  <StateProvider>{children}</StateProvider>
);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("conversation poll interval", () => {
  it("polls fastest while a user message awaits its reply", () => {
    expect(conversationPollInterval([comment({ runStatus: "queued" })], [], true)).toBe(
      ACTIVE_POLL_MS,
    );
    expect(conversationPollInterval([comment({ runStatus: "claimed" })], [], true)).toBe(
      ACTIVE_POLL_MS,
    );
  });

  it("backs off while connected and idle", () => {
    const finished = [comment({ runStatus: "finished" })];
    expect(conversationPollInterval(finished, [session("RUNNING")], true)).toBe(BUSY_POLL_MS);
    expect(conversationPollInterval(finished, [], false)).toBe(BUSY_POLL_MS);
    expect(conversationPollInterval(finished, [session("COMPLETED")], true)).toBe(IDLE_POLL_MS);
  });
});

describe("useConversationChat", () => {
  const serve = () => {
    api.conversation.mockResolvedValue({
      id: "t",
      title: "Chat",
      workspace_id: "ws",
      orchestrator_id: "o",
    });
    api.page.mockResolvedValue({ comments: [comment({})], next_cursor: "" });
    api.sessions.mockResolvedValue({ sessions: [] });
  };

  it("reads nothing while the hosting panel is hidden", async () => {
    serve();
    vi.useFakeTimers();
    renderHook(() => useConversationChat("t", false), { wrapper });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(IDLE_POLL_MS * 2);
    });
    expect(api.page).not.toHaveBeenCalled();
    expect(api.conversation).not.toHaveBeenCalled();
  });

  it("refreshes at once when a comment is created on the conversation", async () => {
    serve();
    const { result } = renderHook(
      () => ({ chat: useConversationChat("t"), store: useAppStoreApi() }),
      { wrapper },
    );
    await waitFor(() => expect(result.current.chat.loaded).toBe(true));
    const calls = api.page.mock.calls.length;
    act(() => {
      result.current.store.getState().setOfficeRefetchTrigger("comments:t");
    });
    await waitFor(() => expect(api.page.mock.calls.length).toBeGreaterThan(calls));
  });

  it("loads once shown and stops polling when hidden again", async () => {
    serve();
    const { result, rerender } = renderHook(
      ({ visible }: { visible: boolean }) => useConversationChat("t", visible),
      { wrapper, initialProps: { visible: true } },
    );
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.metadata?.task.id).toBe("t");
    expect(result.current.comments).toHaveLength(1);
    vi.useFakeTimers();
    rerender({ visible: false });
    const calls = api.page.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(IDLE_POLL_MS * 2);
    });
    expect(api.page).toHaveBeenCalledTimes(calls);
  });
});
