import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { ConversationContent, OrchestrationConversationRoute } from "./conversation-route";

const chat = vi.hoisted(() => ({ use: vi.fn() }));
const feature = vi.hoisted(() => ({ orchestration: true }));
vi.mock("@/hooks/domains/orchestration/use-conversation-chat", () => ({
  useConversationChat: chat.use,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (s: unknown) => unknown) => select({ features: feature }),
}));
vi.mock("@/components/page-shell", () => ({
  PageShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("./conversation-pane", () => ({
  OrchestratorConversationPane: ({ comments }: { comments: unknown[] }) => (
    <p data-testid="pane">{comments.length}</p>
  ),
}));

const base = {
  comments: [],
  nextCursor: "",
  loadingMore: false,
  refresh: vi.fn(),
  loadMore: vi.fn(),
};
const metadata = {
  task: { id: "t", title: "Chat", workspace_id: "ws", orchestrator_id: "o" },
  sessions: [],
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  feature.orchestration = true;
});

it("shows a loading status until the conversation is read", () => {
  chat.use.mockReturnValue({ ...base, loaded: false });
  render(<ConversationContent taskId="t" />);
  expect(screen.getByRole("status").textContent).toBeTruthy();
});

it("offers a retry when the first read fails", () => {
  const refresh = vi.fn();
  chat.use.mockReturnValue({ ...base, loaded: false, error: new Error("down"), refresh });
  render(<ConversationContent taskId="t" />);
  expect(screen.getByRole("alert")).toBeTruthy();
  fireEvent.click(screen.getByRole("button"));
  expect(refresh).toHaveBeenCalled();
});

it("passes visibility to the chat reader and offers earlier messages", () => {
  const loadMore = vi.fn();
  chat.use.mockReturnValue({ ...base, loaded: true, metadata, nextCursor: "c1", loadMore });
  render(<ConversationContent taskId="t" visible={false} />);
  expect(chat.use).toHaveBeenCalledWith("t", false);
  expect(screen.getByTestId("pane")).toBeTruthy();
  fireEvent.click(screen.getByRole("button"));
  expect(loadMore).toHaveBeenCalled();
});

it("withholds the conversation while orchestration is off", () => {
  feature.orchestration = false;
  render(<OrchestrationConversationRoute taskId="t" />);
  expect(chat.use).not.toHaveBeenCalled();
});
