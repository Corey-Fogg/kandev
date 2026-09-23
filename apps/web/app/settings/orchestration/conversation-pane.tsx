import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { TaskChat } from "@/components/task/simple/task-chat";
import { ChatIdentityContext } from "@/components/task/simple/chat-identity-context";
import { ImplicitTaskLinksContext } from "@/components/task/simple/task-link-context";
import { CommentTransportContext } from "@/components/task/simple/comment-transport";
import { RecoveryTransportContext } from "@/components/task/simple/recovery-transport";
import { ActiveSessionRefProvider } from "@/components/task/simple/components/active-session-ref-context";
import { TopbarWorkingIndicator } from "@/components/task/simple/components/topbar-working-indicator";
import { useWorkspaceOrchestrators } from "@/hooks/domains/orchestration/use-orchestrator-conversation";
import {
  createConversationSender,
  retryConversation,
} from "@/lib/api/domains/orchestration-conversation-api";
import {
  orchestratorsHref,
  orchestratorHref,
  coordinatorHref,
  type Orchestrator,
} from "@/lib/api/domains/orchestration-api";
import type { Task, TaskComment, TaskSession } from "@/app/office/tasks/[id]/types";

function usePersona(workspaceId: string, orchestratorId: string, provided?: Orchestrator) {
  const { data } = useWorkspaceOrchestrators(provided ? "" : workspaceId);
  return provided ?? data?.orchestrators.find((item) => item.id === orchestratorId);
}

function ConversationLinks({
  workspaceId,
  orchestratorId,
}: {
  workspaceId: string;
  orchestratorId: string;
}) {
  const { t } = useTranslation();
  return (
    <nav className="flex flex-wrap gap-4 text-sm">
      <Link
        className="underline max-md:min-h-11 inline-flex items-center"
        href={coordinatorHref(workspaceId, orchestratorId)}
      >
        {t("orchestration:coordinator")}
      </Link>
      <Link className="underline" href={orchestratorsHref(workspaceId)}>
        {t("orchestration:orchestration")}
      </Link>
      <Link className="underline" href={orchestratorHref(workspaceId, orchestratorId)}>
        {t("orchestration:configureOrchestrator")}
      </Link>
      <Link className="underline" href={`/?workspaceId=${workspaceId}`}>
        {t("orchestration:workspaceBoard")}
      </Link>
    </nav>
  );
}

export function OrchestratorConversationPane({
  task,
  comments,
  sessions,
  onCommentsChanged,
  orchestratorId,
  persona: providedPersona,
  embedded = false,
}: {
  task: Pick<Task, "id" | "title" | "workspaceId">;
  comments: TaskComment[];
  sessions: TaskSession[];
  onCommentsChanged: () => void;
  orchestratorId: string;
  persona?: Orchestrator;
  embedded?: boolean;
}) {
  const persona = usePersona(task.workspaceId, orchestratorId, providedPersona);
  const identity = useMemo(
    () => ({
      persona: persona ? { id: persona.id, name: persona.name, icon: persona.icon } : null,
    }),
    [persona],
  );
  const transport = useMemo(() => createConversationSender(task.id), [task.id]);
  const [scrollParent, setScrollParent] = useState<HTMLElement | null>(null);
  return (
    <ChatIdentityContext.Provider value={identity}>
      <ImplicitTaskLinksContext.Provider value={false}>
        <ActiveSessionRefProvider>
          <section
            ref={setScrollParent}
            className="flex-1 min-h-0 overflow-y-auto p-4 md:p-6"
            data-testid="orchestrator-conversation"
          >
            {!embedded && (
              <ConversationLinks workspaceId={task.workspaceId} orchestratorId={orchestratorId} />
            )}
            <h1 className="text-xl font-semibold my-4">{task.title}</h1>
            <TopbarWorkingIndicator taskId={task.id} comments={comments} />
            <RecoveryTransportContext.Provider value={retryConversation}>
              <CommentTransportContext.Provider value={transport}>
                <TaskChat
                  taskId={task.id}
                  comments={comments}
                  sessions={sessions}
                  timeline={[]}
                  scrollParent={scrollParent}
                  onCommentsChanged={onCommentsChanged}
                />
              </CommentTransportContext.Provider>
            </RecoveryTransportContext.Provider>
          </section>
        </ActiveSessionRefProvider>
      </ImplicitTaskLinksContext.Provider>
    </ChatIdentityContext.Provider>
  );
}
