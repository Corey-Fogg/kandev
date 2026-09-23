import { createContext, type ReactNode } from "react";
import type { TaskComment } from "@/app/office/tasks/[id]/types";

/**
 * Replaces one comment's rendering in the shared chat. Returning null keeps the
 * default comment entry; without a provider every comment renders as before.
 */
export type CommentRenderer = (comment: TaskComment) => ReactNode | null;

export const CommentRendererContext = createContext<CommentRenderer | null>(null);
