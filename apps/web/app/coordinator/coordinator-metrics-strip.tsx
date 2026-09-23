import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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

/** Phones keep the tiles folded so the task list stays in view. */
function MetricsTiles({ metrics }: { metrics: CoordinatorMetrics }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const tiles = useTiles(metrics);
  return (
    <details open={!isMobile} className="space-y-2">
      <summary className="md:hidden cursor-pointer min-h-11 flex items-center text-sm">
        {t("orchestration:metricsHeading")}
      </summary>
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
    </details>
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

/** Outcome metrics for the selected orchestrator over the last 7 or 30 days. */
export function CoordinatorMetricsStrip({
  workspaceId,
  orchestratorId,
}: {
  workspaceId: string;
  orchestratorId: string;
}) {
  const { t } = useTranslation();
  const [days, setDays] = useState<7 | 30>(7);
  const { data, error, loading, refresh } = useCoordinatorMetrics(
    workspaceId,
    orchestratorId,
    days,
  );
  return (
    <section
      aria-labelledby="coordinator-metrics-heading"
      className="space-y-2 border-b px-4 py-3"
      data-testid="coordinator-metrics"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="coordinator-metrics-heading" className="hidden text-sm font-semibold md:block">
          {t("orchestration:metricsHeading")}
        </h2>
        <WindowToggle days={days} setDays={setDays} />
      </div>
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
    </section>
  );
}
