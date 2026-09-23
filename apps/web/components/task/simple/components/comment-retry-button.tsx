"use client";

import { useContext, useState } from "react";
import { IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { toast } from "@/lib/toast/sonner";
import { RecoveryTransportContext } from "../recovery-transport";

/**
 * Retries a failed user-message turn by its run id. A turn that failed before
 * it bound a session has no session error entry, so this is its only retry.
 * Renders nothing outside a surface that provides its own recovery endpoint.
 */
export function CommentRetryButton({ taskId, runId }: { taskId: string; runId: string }) {
  const { t } = useTranslation();
  const recover = useContext(RecoveryTransportContext);
  const [busy, setBusy] = useState(false);
  if (!recover) return null;
  const retry = async () => {
    setBusy(true);
    try {
      const outcome = await recover(taskId, { runId }, "resume");
      if (outcome === "already_queued") toast.info(t("orchestration:retryAlreadyQueued"));
      else toast.success(t("orchestration:retryQueued"));
    } catch (cause) {
      toast.error(String(cause));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      className="h-6 px-2 text-xs cursor-pointer"
      disabled={busy}
      onClick={() => void retry()}
      data-testid="user-comment-retry"
    >
      <IconRefresh className="h-3 w-3" aria-hidden />
      {t("orchestration:retryMessage")}
    </Button>
  );
}
