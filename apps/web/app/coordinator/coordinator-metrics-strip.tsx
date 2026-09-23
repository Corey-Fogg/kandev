import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { useLocalStorageBoolean } from "@/hooks/use-local-storage-boolean";
import { useCoordinatorMetrics } from "@/hooks/domains/orchestration/use-coordinator-metrics";
import type { CoordinatorMetrics } from "@/lib/api/domains/orchestration-api";

type Tile = { id: string; label: string; value: string; detail?: string };
const WINDOWS = [7, 30] as const;

function useTiles(metrics: CoordinatorMetrics): Tile[] {
  const { t, i18n } = useTranslation();
  const none = t("orchestration:metricsNoValue");
  const count = new Intl.NumberFormat(i18n.language);
  const hours = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 });
  const percent = new Intl.NumberFormat(i18n.language, {
    style: "percent",
    maximumFractionDigits: 0,
  });
  const usd = new Intl.NumberFormat(i18n.language, { style: "currency", currency: "USD" });
  const formatHours = (value: number | null) =>
    value === null
      ? none
      : t("orchestration:metricsCycleTimeValue", { hours: hours.format(value) });
  const cost = usd.format(metrics.cost_usd);
  return [
    {
      id: "delegated",
      label: t("orchestration:metricsDelegated"),
      value: count.format(metrics.delegated),
    },
    {
      id: "completed",
      label: t("orchestration:metricsCompleted"),
      value: count.format(metrics.completed),
    },
    { id: "failed", label: t("orchestration:metricsFailed"), value: count.format(metrics.failed) },
    {
      id: "success",
      label: t("orchestration:metricsSuccessRate"),
      value: metrics.success_rate === null ? none : percent.format(metrics.success_rate),
    },
    {
      id: "merged",
      label: t("orchestration:metricsMergedPRs"),
      value: metrics.merged_prs === null ? none : count.format(metrics.merged_prs),
    },
    {
      id: "cycle",
      label: t("orchestration:metricsCycleTime"),
      value: formatHours(metrics.cycle_time_median_hours),
      detail:
        metrics.cycle_time_p90_hours === null
          ? undefined
          : t("orchestration:metricsCycleTimeP90", {
              hours: hours.format(metrics.cycle_time_p90_hours),
            }),
    },
    {
      id: "cost",
      label: t("orchestration:metricsCost"),
      value:
        metrics.unpriced_event_count > 0
          ? t("orchestration:metricsCostLowerBound", { amount: cost })
          : cost,
    },
  ];
}

function MetricsTiles({ metrics }: { metrics: CoordinatorMetrics }) {
  const { t } = useTranslation();
  const tiles = useTiles(metrics);
  return (
    <>
      <dl className="flex flex-wrap gap-2">
        {tiles.map((tile) => (
          <div
            key={tile.id}
            className="min-w-[7rem] flex-1 rounded-md border bg-card px-3 py-2"
            data-testid={`coordinator-metric-${tile.id}`}
          >
            <dt className="text-xs text-muted-foreground">{tile.label}</dt>
            <dd className="font-semibold tabular-nums">{tile.value}</dd>
            {tile.detail && <dd className="text-xs text-muted-foreground">{tile.detail}</dd>}
          </div>
        ))}
      </dl>
      {metrics.truncated && (
        <p className="text-xs text-muted-foreground">{t("orchestration:metricsTruncated")}</p>
      )}
    </>
  );
}

function WindowToggle({ days, setDays }: { days: 7 | 30; setDays: (days: 7 | 30) => void }) {
  const { t } = useTranslation();
  return (
    <div role="group" aria-label={t("orchestration:metricsWindow")} className="flex gap-1">
      {WINDOWS.map((value) => (
        <Button
          key={value}
          type="button"
          size="sm"
          variant={days === value ? "secondary" : "ghost"}
          aria-pressed={days === value}
          className="cursor-pointer max-md:min-h-11"
          onClick={() => setDays(value)}
        >
          {t(value === 7 ? "orchestration:metricsDays_7" : "orchestration:metricsDays_30")}
        </Button>
      ))}
    </div>
  );
}

const EXPANDED_KEY = "kandev.coordinator.metrics.expanded";
const EXPANDED_EVENT = "kandev:coordinator-metrics-expanded";

function MetricsSummary({ metrics }: { metrics: CoordinatorMetrics }) {
  const { t, i18n } = useTranslation();
  const count = new Intl.NumberFormat(i18n.language);
  return (
    <span className="truncate text-xs font-normal text-muted-foreground tabular-nums">
      {t("orchestration:metricsSummary", {
        completed: count.format(metrics.completed),
        failed: count.format(metrics.failed),
      })}
    </span>
  );
}

function useExpanded() {
  const { value, setValue } = useLocalStorageBoolean(EXPANDED_KEY, EXPANDED_EVENT, false);
  const toggle = () => {
    try {
      setValue(!value);
    } catch {
      // Storage unavailable: the strip keeps its current state.
    }
  };
  return { expanded: value, toggle };
}

/** Outcome metrics for the selected orchestrator over the last 7 or 30 days; collapsed by default. */
export function CoordinatorMetricsStrip({
  workspaceId,
  orchestratorId,
}: {
  workspaceId: string;
  orchestratorId: string;
}) {
  const { t } = useTranslation();
  const { expanded, toggle } = useExpanded();
  const [days, setDays] = useState<7 | 30>(7);
  const { data, error, loading, refresh } = useCoordinatorMetrics(
    workspaceId,
    orchestratorId,
    days,
  );
  const Chevron = expanded ? IconChevronDown : IconChevronRight;
  return (
    <section
      aria-labelledby="coordinator-metrics-heading"
      className="space-y-2 border-b px-4 py-1.5"
      data-testid="coordinator-metrics"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="coordinator-metrics-heading" className="min-w-0 flex-1 text-sm font-semibold">
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls="coordinator-metrics-body"
            className="flex w-full min-w-0 cursor-pointer items-center gap-1.5 text-left max-md:min-h-11"
            onClick={toggle}
            data-testid="coordinator-metrics-toggle"
          >
            <Chevron className="h-4 w-4 shrink-0" aria-hidden />
            <span>{t("orchestration:metricsHeading")}</span>
            {!expanded && data && !error && <MetricsSummary metrics={data} />}
          </button>
        </h2>
        {expanded && <WindowToggle days={days} setDays={setDays} />}
      </div>
      {expanded && (
        <div id="coordinator-metrics-body" className="space-y-2 pb-1.5">
          {error ? (
            <div role="alert" className="flex flex-wrap items-center gap-2 text-sm">
              <span>{t("orchestration:metricsUnavailable")}</span>
              <Button
                size="sm"
                variant="outline"
                className="cursor-pointer max-md:min-h-11"
                onClick={refresh}
              >
                {t("task:retry")}
              </Button>
            </div>
          ) : null}
          {loading && (
            <p role="status" className="text-xs text-muted-foreground">
              {t("common:loading")}
            </p>
          )}
          {data && !error && <MetricsTiles metrics={data} />}
        </div>
      )}
    </section>
  );
}
