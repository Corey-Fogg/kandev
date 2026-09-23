import { afterEach, describe, expect, it, vi } from "vitest";
import {
  createConversationSender,
  getConversationCommentPage,
  postConversationComment,
  retryConversation,
} from "./orchestration-conversation-api";
afterEach(() => vi.unstubAllGlobals());
it("uses independent endpoints and preserves comment run state", async () => {
  const fetcher = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        comments: [
          {
            id: "c",
            task_id: "t",
            author_id: "chief",
            author_type: "agent",
            body: "Finished",
            created_at: "2026-09-08",
            source: "session",
            run_id: "r",
            run_status: "finished",
          },
        ],
      }),
      { status: 200 },
    ),
  );
  vi.stubGlobal("fetch", fetcher);
  const { comments: rows } = await getConversationCommentPage("t");
  expect(rows[0]).toMatchObject({
    content: "Finished",
    authorId: "chief",
    runId: "r",
    runStatus: "finished",
  });
  expect(fetcher.mock.calls[0][0]).toContain("/api/v1/orchestration/tasks/t/comments");
  fetcher.mockImplementation(async () => new Response("{}", { status: 200 }));
  await postConversationComment("t", { body: "Hello" });
  await retryConversation("t", { sessionId: "session" }, "resume");
  expect(fetcher.mock.calls.map((call) => call[0])).toEqual(
    expect.arrayContaining([expect.stringContaining("/api/v1/orchestration/tasks/t/retry")]),
  );
  expect(fetcher.mock.calls.every((call) => !String(call[0]).includes("/office/"))).toBe(true);
});

it("shows an accepted user message as queued before a run exists", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          comments: [
            {
              id: "c",
              task_id: "t",
              author_id: "owner",
              author_type: "user",
              body: "Still waiting",
              created_at: "2026-09-18",
              receipt_status: "accepted",
            },
          ],
          next_cursor: "",
        }),
        { status: 200 },
      ),
    ),
  );

  const page = await getConversationCommentPage("t");

  expect(page.comments[0]).toMatchObject({ runStatus: "queued" });
});

describe("conversation sender", () => {
  const sentIds = (fetcher: ReturnType<typeof vi.fn>) =>
    fetcher.mock.calls.map((call) => JSON.parse(String(call[1]?.body)).client_message_id);

  it("reuses one client_message_id when the same message is retried", async () => {
    const fetcher = vi
      .fn()
      .mockRejectedValueOnce(new TypeError("network down"))
      .mockImplementation(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetcher);
    const send = createConversationSender("t");
    await expect(send("t", { body: "Hello", author_type: "user" })).rejects.toThrow();
    await send("t", { body: "Hello", author_type: "user" });
    await send("t", { body: "Next", author_type: "user" });
    const [first, retry, next] = sentIds(fetcher);
    expect(first).toBeTruthy();
    expect(retry).toBe(first);
    expect(next).not.toBe(first);
  });

  it("joins a duplicate submit and refuses a different message while one is in flight", async () => {
    let release!: (response: Response) => void;
    const fetcher = vi.fn(() => new Promise<Response>((resolve) => (release = resolve)));
    vi.stubGlobal("fetch", fetcher);
    const send = createConversationSender("t");
    const first = send("t", { body: "Hello", author_type: "user" });
    const duplicate = send("t", { body: "Hello", author_type: "user" });
    await expect(send("t", { body: "Other", author_type: "user" })).rejects.toThrow();
    await expect(send("other", { body: "Hello", author_type: "user" })).rejects.toThrow();
    release(new Response("{}", { status: 200 }));
    await Promise.all([first, duplicate]);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
});

it("retries an unbound turn by run id and reports an already queued retry", async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true, status: "queued" })))
    .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true, status: "already_queued" })));
  vi.stubGlobal("fetch", fetcher);
  await expect(retryConversation("t", { runId: "run-1" }, "resume")).resolves.toBe("queued");
  expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({ run_id: "run-1", action: "resume" });
  await expect(retryConversation("t", { runId: "run-1" }, "resume")).resolves.toBe(
    "already_queued",
  );
});
