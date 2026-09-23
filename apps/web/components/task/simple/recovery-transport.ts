import { createContext } from "react";

/** The turn to retry: a failed session's latest turn, or one turn by run id. */
export type RecoveryTarget = { sessionId?: string; runId?: string };

/** What the retry did: queued a new turn, or found one already queued. */
export type RecoveryOutcome = "queued" | "already_queued";

type RecoveryTransport = (
  taskId: string,
  target: RecoveryTarget,
  action: "resume" | "fresh_start",
) => Promise<RecoveryOutcome>;

// Conversation surfaces that own their own retry endpoint provide it here;
// without a provider the entry uses the task session recovery actions.
export const RecoveryTransportContext = createContext<RecoveryTransport | null>(null);
