import { listOrchestrators } from "@/lib/api/domains/orchestration-api";

const FRESH_MS = 10_000;
type Entry = { at: number; request: ReturnType<typeof listOrchestrators> };
const entries = new Map<string, Entry>();

/**
 * Shares one orchestrator list read per workspace between the surfaces open at
 * the same time. A settled result is reused for a short window; a failed read
 * is never reused.
 */
export function readWorkspaceOrchestrators(workspaceId: string, now = Date.now()) {
  const cached = entries.get(workspaceId);
  if (cached && now - cached.at < FRESH_MS) return cached.request;
  const entry: Entry = { at: now, request: listOrchestrators(workspaceId) };
  entries.set(workspaceId, entry);
  entry.request.catch(() => {
    if (entries.get(workspaceId) === entry) entries.delete(workspaceId);
  });
  return entry.request;
}

/** Drops cached lists for one workspace, or for every workspace when omitted. */
export function invalidateWorkspaceOrchestrators(workspaceId?: string) {
  if (workspaceId) entries.delete(workspaceId);
  else entries.clear();
}
