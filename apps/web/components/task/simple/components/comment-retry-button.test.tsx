import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";

const toast = vi.hoisted(() => ({ info: vi.fn(), success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast/sonner", () => ({ toast }));

import { RecoveryTransportContext } from "../recovery-transport";
import { CommentRetryButton } from "./comment-retry-button";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const RETRY_TESTID = "user-comment-retry";

describe("CommentRetryButton", () => {
  it("renders nothing without a conversation recovery endpoint", () => {
    render(<CommentRetryButton taskId="t" runId="r" />);
    expect(screen.queryByTestId(RETRY_TESTID)).toBeNull();
  });

  it("retries the failed turn by run id", async () => {
    const recover = vi.fn().mockResolvedValue("queued");
    render(
      <RecoveryTransportContext.Provider value={recover}>
        <CommentRetryButton taskId="t" runId="r" />
      </RecoveryTransportContext.Provider>,
    );
    fireEvent.click(screen.getByTestId(RETRY_TESTID));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(recover).toHaveBeenCalledWith("t", { runId: "r" }, "resume");
  });

  it("says so when a retry is already queued", async () => {
    const recover = vi.fn().mockResolvedValue("already_queued");
    render(
      <RecoveryTransportContext.Provider value={recover}>
        <CommentRetryButton taskId="t" runId="r" />
      </RecoveryTransportContext.Provider>,
    );
    fireEvent.click(screen.getByTestId(RETRY_TESTID));
    await waitFor(() => expect(toast.info).toHaveBeenCalledWith("A retry is already queued"));
    expect(toast.success).not.toHaveBeenCalled();
  });
});
