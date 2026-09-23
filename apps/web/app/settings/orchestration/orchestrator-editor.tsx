import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
  AlertDialogAction,
} from "@kandev/ui/alert-dialog";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { toast } from "@/lib/toast/sonner";
import {
  useOrchestrationData,
  notifyOrchestrationChanged,
} from "@/hooks/domains/orchestration/use-orchestration-data";
import {
  selectedExecutor,
  getOrchestrator,
  listOrchestratorRoles,
  listOrchestrationProfiles,
  saveOrchestrator,
  patchOrchestrator,
  deleteOrchestrator,
  orchestratorsHref,
  orchestratorHref,
  type Orchestrator,
  type OrchestratorRole,
  type OrchestrationProfile,
} from "@/lib/api/domains/orchestration-api";
import { OrchestrationGate } from "./orchestration-gate";
import { OpenOrchestratorConversation, OrchestratorTasks } from "./orchestrator-connections";
import { OrchestratorFields } from "./orchestrator-fields";
import { validDisplayName } from "./orchestrator-identity-fields";
import { initialConfiguration, normalizedConfiguration, patchFor } from "./orchestrator-save";
export function OrchestratorEditor({ workspaceId, id }: { workspaceId: string; id: string }) {
  return (
    <OrchestrationGate>
      <EditorLoader key={`${workspaceId}:${id}`} workspaceId={workspaceId} id={id} />
    </OrchestrationGate>
  );
}
function EditorLoader({ workspaceId, id }: { workspaceId: string; id: string }) {
  const { t } = useTranslation();
  const load = useCallback(async () => {
    const [roles, profiles, item] = await Promise.all([
      listOrchestratorRoles(),
      listOrchestrationProfiles(workspaceId),
      id === "new" ? Promise.resolve(undefined) : getOrchestrator(workspaceId, id),
    ]);
    return { roles: roles.roles, profiles: profiles.profiles, item };
  }, [workspaceId, id]);
  const { data, error } = useOrchestrationData(load);
  if (error) return <p role="alert">{error}</p>;
  if (!data) return <p>{t("common:loading")}</p>;
  return <EditorForm workspaceId={workspaceId} {...data} />;
}
function useEditorForm({
  workspaceId,
  item,
  roles,
  profiles,
}: {
  workspaceId: string;
  item?: Orchestrator;
  roles: OrchestratorRole[];
  profiles: OrchestrationProfile[];
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [value, setValue] = useState(() => initialConfiguration(item, roles[0]?.id ?? ""));
  const [saved, setSaved] = useState(value);
  const [name, setName] = useState(item?.name ?? "");
  const valid =
    profiles.some((p) => p.id === value.profile_id) &&
    roles.some((r) => r.id === value.role_id) &&
    !!selectedExecutor(value.executor_preference) &&
    validDisplayName(value.display_name);
  const save = async () => {
    const patch = item ? patchFor(saved, value) : null;
    const result = patch
      ? await patchOrchestrator(workspaceId, item!.id, patch)
      : await saveOrchestrator(workspaceId, item?.id, normalizedConfiguration(value));
    setSaved(value);
    setName(result.name);
    notifyOrchestrationChanged(workspaceId);
    if (!item) router.replace(orchestratorHref(workspaceId, result.id));
  };
  useSettingsSaveContributor({
    id: `orchestrator-${item?.id ?? "new"}`,
    revision: JSON.stringify(value),
    isDirty: !!item && JSON.stringify(saved) !== JSON.stringify(value),
    canSave: valid,
    save,
    discard: () => setValue(saved),
  });
  const create = async () => {
    setBusy(true);
    try {
      await save();
    } catch (e) {
      toast.error(String(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    try {
      await deleteOrchestrator(workspaceId, item!.id);
      notifyOrchestrationChanged(workspaceId);
      router.push(orchestratorsHref(workspaceId));
    } catch (e) {
      toast.error(String(e));
    }
  };
  return { t, value, setValue, busy, valid, create, remove, name };
}
function EditorForm({
  workspaceId,
  item,
  roles,
  profiles,
}: {
  workspaceId: string;
  item?: Orchestrator;
  roles: OrchestratorRole[];
  profiles: OrchestrationProfile[];
}) {
  const { t, value, setValue, busy, valid, create, remove, name } = useEditorForm({
    workspaceId,
    item,
    roles,
    profiles,
  });
  return (
    <section className="max-w-3xl space-y-5" data-testid="orchestrator-editor">
      <div className="flex flex-wrap gap-4">
        <Link className="underline" href={orchestratorsHref(workspaceId)}>
          {t("orchestration:orchestration")}
        </Link>
        <Link className="underline" href="/settings/orchestration">
          {t("orchestration:manageRoles")}
        </Link>
      </div>
      <h2 className="text-xl font-semibold">{item ? name : t("orchestration:addOrchestrator")}</h2>
      {item && <OpenOrchestratorConversation workspaceId={workspaceId} id={item.id} />}
      <OrchestratorFields value={value} onChange={setValue} roles={roles} profiles={profiles} />
      {!item && (
        <Button onClick={create} disabled={busy || !valid}>
          {t("orchestration:addOrchestrator")}
        </Button>
      )}
      {item && <OrchestratorTasks workspaceId={workspaceId} id={item.id} />}
      {item && (
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive">{t("orchestration:deleteOrchestrator")}</Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("orchestration:deleteOrchestrator")}</AlertDialogTitle>
              <AlertDialogDescription>
                {t("orchestration:deleteOrchestratorHint")}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("common:cancel")}</AlertDialogCancel>
              <AlertDialogAction onClick={remove}>{t("orchestration:delete")}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </section>
  );
}
