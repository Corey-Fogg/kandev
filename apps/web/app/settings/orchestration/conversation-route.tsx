import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import { useConversationChat } from "@/hooks/domains/orchestration/use-conversation-chat";
import type { Orchestrator } from "@/lib/api/domains/orchestration-api";
import type { TaskSession as APISession } from "@/lib/types/http";
import type { TaskSession } from "@/app/office/tasks/[id]/types";
import { OrchestratorConversationPane } from "./conversation-pane";

export function mapConversationSession(session: APISession): TaskSession {
  return {
    id: session.id,
    agentProfileId: session.agent_profile_id,
    agentName: "",
    agentRole: "agent",
    state: session.state as TaskSession["state"],
    isPrimary: Boolean(session.is_primary),
    startedAt: session.started_at,
    completedAt: session.completed_at ?? undefined,
    updatedAt: session.updated_at,
    errorMessage: session.error_message ?? undefined,
    metadata: session.metadata,
    commandCount: session.command_count,
  };
}

export function ConversationContent({
  taskId,
  embedded = false,
  visible = true,
  persona,
}: {
  taskId: string;
  embedded?: boolean;
  /** Polling stops while the hosting panel is hidden. */
  visible?: boolean;
  /** The conversation owner, when the host already holds the workspace catalog. */
  persona?: Orchestrator;
}) {
  const { t } = useTranslation();
  const chat = useConversationChat(taskId, visible);
  if (!chat.metadata || !chat.loaded) {
    if (!chat.error)
      return (
        <p role="status" className="p-4">
          {t("common:loading")}
        </p>
      );
    return (
      <div role="alert" className="p-4 space-y-3">
        <p>{t("orchestration:conversationUnavailable")}</p>
        <Button
          variant="outline"
          className="cursor-pointer max-md:min-h-11"
          onClick={() => void chat.refresh()}
        >
          {t("task:retry")}
        </Button>
      </div>
    );
  }
  const { task, sessions } = chat.metadata;
  return (
    <>
      {chat.error ? (
        <p role="alert" className="px-4 pt-2 text-sm">
          {t("orchestration:conversationUnavailable")}
        </p>
      ) : null}
      {chat.nextCursor && (
        <Button
          variant="ghost"
          className="cursor-pointer max-md:min-h-11 shrink-0"
          disabled={chat.loadingMore}
          onClick={() => void chat.loadMore()}
        >
          {t("orchestration:earlierMessages")}
        </Button>
      )}
      <OrchestratorConversationPane
        embedded={embedded}
        task={{ id: task.id, title: task.title, workspaceId: task.workspace_id }}
        orchestratorId={task.orchestrator_id ?? ""}
        persona={persona}
        comments={chat.comments}
        sessions={sessions.map(mapConversationSession)}
        onCommentsChanged={() => void chat.refresh()}
      />
    </>
  );
}

export function OrchestrationConversationRoute({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const enabled = useAppStore((s) => s.features.orchestration);
  return (
    <PageShell title={t("orchestration:orchestration")} scroll="none">
      <div className="flex flex-col h-full min-h-0">
        {enabled ? (
          <ConversationContent key={taskId} taskId={taskId} />
        ) : (
          <p>{t("common:unavailable")}</p>
        )}
      </div>
    </PageShell>
  );
}
