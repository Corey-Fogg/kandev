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

/** Safety cap on pages one read may walk, should the bound row vanish. */
const MAX_WINDOW_PAGES = 100;

const sameEntries = <T>(a: T[], b: T[]) =>
  a.length === b.length && JSON.stringify(a) === JSON.stringify(b);

/**
 * A newest-first paged read that re-reads down to the oldest row the viewer
 * paged in on every refresh, so a polled window never drops rows the viewer
 * already paged in, however many new rows arrive above them. Only the latest
 * read may commit, and a read that returns identical rows keeps the previous
 * entries array so subscribers skip a re-render.
 */
export class PagedWindow<T extends { id: string }> {
  private snapshot = initialWindow<T>();
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private request?: AbortController;
  /** Id of the oldest row the viewer paged in; empty while only the newest page is shown. */
  private bound = "";
  /** A requested older page that no read has committed yet. */
  private extend = false;
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
  private async readWindow(signal: AbortSignal, extend: boolean) {
    const entries = new Map<string, T>();
    let after = "";
    let reached = this.bound === "";
    let extra = extend ? 1 : 0;
    for (let page = 0; page < MAX_WINDOW_PAGES; page++) {
      const result = await this.load(after, signal);
      for (const entry of result.entries) {
        entries.set(entry.id, entry);
        if (entry.id === this.bound) reached = true;
      }
      after = result.next_cursor;
      if (!after) break;
      if (reached && extra-- <= 0) break;
    }
    return { entries: [...entries.values()], nextCursor: after };
  }
  private commit(result: { entries: T[]; nextCursor: string }, extend: boolean) {
    if (extend) {
      this.extend = false;
      this.bound = result.entries.at(-1)?.id ?? "";
    }
    const entries = sameEntries(this.snapshot.entries, result.entries)
      ? this.snapshot.entries
      : result.entries;
    this.update({ entries, nextCursor: result.nextCursor, loaded: true, error: null });
  }
  private fail(error: unknown) {
    const denied = error instanceof ApiError && [401, 403, 404, 409].includes(error.status);
    if (denied) {
      this.bound = "";
      this.extend = false;
      this.update(initialWindow<T>());
    }
    this.update({ error });
  }
  /** Background refreshes leave `loading` alone so an unchanged poll never re-renders. */
  refresh = async (showLoading = !this.snapshot.loaded) => {
    if (this.disposed) return;
    this.request?.abort();
    const controller = new AbortController();
    this.request = controller;
    const generation = ++this.generation;
    if (showLoading) this.update({ loading: true });
    const extend = this.extend;
    try {
      const result = await this.readWindow(controller.signal, extend);
      if (this.disposed || generation !== this.generation) return;
      this.commit(result, extend);
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      this.fail(error);
    } finally {
      if (!this.disposed && generation === this.generation) this.update({ loading: false });
    }
  };
  loadMore = async () => {
    if (this.snapshot.loading || !this.snapshot.nextCursor) return;
    this.extend = true;
    await this.refresh(true);
  };
}
