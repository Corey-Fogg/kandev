import { ApiError } from "@/lib/api/client";

export type WindowPage<T> = { entries: T[]; next_cursor: string };

export type WindowSnapshot<T> = {
  entries: T[];
  nextCursor: string;
  loading: boolean;
  loaded: boolean;
  error: unknown;
};

const initialWindow = <T>(): WindowSnapshot<T> => ({
  entries: [],
  nextCursor: "",
  loading: false,
  loaded: false,
  error: null,
});

const sameEntries = <T>(a: T[], b: T[]) =>
  a.length === b.length && JSON.stringify(a) === JSON.stringify(b);

/**
 * A newest-first paged read that re-reads every loaded page on refresh, so a
 * polled window never drops rows the viewer already paged in. Only the latest
 * read may commit, and a read that returns identical rows keeps the previous
 * entries array so subscribers skip a re-render.
 */
export class PagedWindow<T extends { id: string }> {
  private snapshot = initialWindow<T>();
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private request?: AbortController;
  private pages = 1;
  constructor(private load: (after: string, signal: AbortSignal) => Promise<WindowPage<T>>) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  activate = () => {
    this.disposed = false;
  };
  dispose = () => {
    this.disposed = true;
    ++this.generation;
    this.request?.abort();
    this.snapshot = initialWindow<T>();
  };
  private update(patch: Partial<WindowSnapshot<T>>) {
    const next = { ...this.snapshot, ...patch };
    const changed = (Object.keys(patch) as (keyof WindowSnapshot<T>)[]).some(
      (key) => next[key] !== this.snapshot[key],
    );
    if (!changed) return;
    this.snapshot = next;
    this.listeners.forEach((listener) => listener());
  }
  private async readWindow(signal: AbortSignal) {
    const entries = new Map<string, T>();
    let after = "";
    for (let page = 0; page < this.pages; page++) {
      const result = await this.load(after, signal);
      for (const entry of result.entries) entries.set(entry.id, entry);
      after = result.next_cursor;
      if (!after) break;
    }
    return { entries: [...entries.values()], nextCursor: after };
  }
  /** Background refreshes leave `loading` alone so an unchanged poll never re-renders. */
  refresh = async (showLoading = !this.snapshot.loaded) => {
    if (this.disposed) return;
    this.request?.abort();
    const controller = new AbortController();
    this.request = controller;
    const generation = ++this.generation;
    if (showLoading) this.update({ loading: true });
    try {
      const result = await this.readWindow(controller.signal);
      if (this.disposed || generation !== this.generation) return;
      const entries = sameEntries(this.snapshot.entries, result.entries)
        ? this.snapshot.entries
        : result.entries;
      this.update({ entries, nextCursor: result.nextCursor, loaded: true, error: null });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const denied = error instanceof ApiError && [401, 403, 404, 409].includes(error.status);
      if (denied) {
        this.pages = 1;
        this.update(initialWindow<T>());
      }
      this.update({ error });
    } finally {
      if (!this.disposed && generation === this.generation) this.update({ loading: false });
    }
  };
  loadMore = async () => {
    if (this.snapshot.loading || !this.snapshot.nextCursor) return;
    ++this.pages;
    await this.refresh(true);
  };
}
