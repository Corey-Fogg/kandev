import { createContext } from "react";
import { createComment } from "@/lib/api/domains/office-api";

export type CommentTransport = (
  taskId: string,
  body: { body: string; author_type: string },
) => Promise<unknown>;

// Office task discussions post through the Office API; other conversation
// surfaces provide their own transport.
export const CommentTransportContext = createContext<CommentTransport>((taskId, body) =>
  createComment(taskId, body),
);
