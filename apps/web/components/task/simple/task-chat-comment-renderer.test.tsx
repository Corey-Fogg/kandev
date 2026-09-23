import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { TaskComment } from "@/app/office/tasks/[id]/types";

vi.mock("./markdown-comment", () => ({
  MarkdownComment: ({ content }: { content: string }) => <p>{content}</p>,
}));

import { TaskChat } from "./task-chat";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";
import { CommentRendererContext } from "./comment-renderer-context";

afterEach(cleanup);

const comment = (id: string, createdAt: string, content: string, source?: string) =>
  ({
    id,
    taskId: "task-1",
    authorType: "agent",
    authorId: "agent-1",
    authorName: "Agent",
    content,
    createdAt,
    source,
  }) as TaskComment;
const comments = [
  comment("c-plain", "2026-05-01T10:00:00Z", "plain comment"),
  comment("c-card", "2026-05-01T11:00:00Z", "fallback body", "proposal"),
];
const wrap = (node: ReactNode) => (
  <StateProvider>
    <ActiveSessionRefProvider>{node}</ActiveSessionRefProvider>
  </StateProvider>
);

describe("TaskChat comment renderer context", () => {
  it("lets a provider replace one comment while others render normally", () => {
    render(
      wrap(
        <CommentRendererContext.Provider
          value={(entry) =>
            entry.source === "proposal" ? <p data-testid="custom-card">card</p> : null
          }
        >
          <TaskChat taskId="task-1" comments={comments} sessions={[]} readOnly />
        </CommentRendererContext.Provider>,
      ),
    );
    expect(screen.getByTestId("custom-card").parentElement?.id).toBe("comment-c-card");
    expect(screen.queryByText("fallback body")).toBeNull();
    expect(screen.getByText("plain comment")).toBeTruthy();
  });

  it("renders every comment as before without a provider", () => {
    render(wrap(<TaskChat taskId="task-1" comments={comments} sessions={[]} readOnly />));
    expect(screen.getByText("fallback body")).toBeTruthy();
    expect(screen.getByText("plain comment")).toBeTruthy();
  });
});
