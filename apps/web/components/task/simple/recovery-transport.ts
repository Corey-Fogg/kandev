import { createContext } from "react";

type RecoveryTransport = (
  taskId: string,
  sessionId: string,
  action: "resume" | "fresh_start",
) => Promise<unknown>;

// Conversation surfaces that own their own retry endpoint provide it here;
// without a provider the entry uses the task session recovery actions.
export const RecoveryTransportContext = createContext<RecoveryTransport | null>(null);
