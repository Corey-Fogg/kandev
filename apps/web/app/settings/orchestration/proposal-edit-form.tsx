import { useId, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useProposalCatalog } from "@/hooks/domains/orchestration/use-proposal-catalog";
import type { ProposalEdits, ProposalSpec } from "@/lib/api/domains/orchestration-proposals-api";
import {
  draftFromSpec,
  proposalEdits,
  validDraft,
  PROPOSAL_CRITERIA_MAX,
  PROPOSAL_CRITERION_MAX,
  PROPOSAL_TITLE_MAX,
  type ProposalDraft,
} from "@/lib/orchestration/proposal-edits";

const DEFAULT = "__default__";
const CONTROL = "max-md:min-h-11";
type Option = { id: string; name: string };

function OptionField({
  label,
  value,
  options,
  onChange,
  testId,
}: {
  label: string;
  value: string;
  options: Option[];
  onChange: (value: string) => void;
  testId: string;
}) {
  const { t } = useTranslation();
  const id = useId();
  const known = !value || options.some((option) => option.id === value);
  return (
    <div className="min-w-0 space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Select
        value={value || DEFAULT}
        onValueChange={(next) => onChange(next === DEFAULT ? "" : next)}
      >
        <SelectTrigger id={id} className={`w-full cursor-pointer ${CONTROL}`} data-testid={testId}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={DEFAULT} className="cursor-pointer">
            {t("orchestration:proposalDefaultOption")}
          </SelectItem>
          {!known && (
            <SelectItem value={value} className="cursor-pointer">
              {value}
            </SelectItem>
          )}
          {options.map((option) => (
            <SelectItem key={option.id} value={option.id} className="cursor-pointer">
              {option.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function CriteriaEditor({
  criteria,
  onChange,
}: {
  criteria: string[];
  onChange: (criteria: string[]) => void;
}) {
  const { t } = useTranslation();
  const set = (index: number, value: string) =>
    onChange(criteria.map((item, i) => (i === index ? value : item)));
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("orchestration:proposalFieldCriteria")}</legend>
      {criteria.map((item, index) => (
        <div key={index} className="flex items-center gap-2">
          <Input
            value={item}
            maxLength={PROPOSAL_CRITERION_MAX}
            aria-label={t("orchestration:proposalCriterionLabel", { index: index + 1 })}
            onChange={(event) => set(index, event.target.value)}
            className={CONTROL}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={`shrink-0 cursor-pointer ${CONTROL} max-md:min-w-11`}
            aria-label={t("orchestration:proposalRemoveCriterion", { index: index + 1 })}
            onClick={() => onChange(criteria.filter((_, i) => i !== index))}
          >
            <IconX className="h-4 w-4" />
          </Button>
        </div>
      ))}
      {criteria.length < PROPOSAL_CRITERIA_MAX && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className={`cursor-pointer ${CONTROL}`}
          onClick={() => onChange([...criteria, ""])}
        >
          {t("orchestration:proposalAddCriterion")}
        </Button>
      )}
    </fieldset>
  );
}

function TitleField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const { t } = useTranslation();
  const id = useId();
  const length = [...value.trim()].length;
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between gap-2">
        <Label htmlFor={id}>{t("orchestration:proposalFieldTitle")}</Label>
        <span className="text-xs text-muted-foreground tabular-nums" aria-hidden="true">
          {t("orchestration:proposalTitleCounter", { length, max: PROPOSAL_TITLE_MAX })}
        </span>
      </div>
      <Input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        aria-invalid={length > PROPOSAL_TITLE_MAX || undefined}
        className={CONTROL}
        data-testid="proposal-edit-title"
      />
      {length > PROPOSAL_TITLE_MAX && (
        <p role="alert" className="text-xs text-destructive">
          {t("orchestration:proposalTitleTooLong")}
        </p>
      )}
    </div>
  );
}

function useDraftOptions(workspaceId: string, draft: ProposalDraft) {
  const { t } = useTranslation();
  const { catalog } = useProposalCatalog(workspaceId, true);
  return {
    workflows: catalog?.workflows ?? [],
    steps: (catalog?.steps ?? [])
      .filter((step) => step.workflow_id === draft.workflow_id)
      .sort((a, b) => (a.position ?? 0) - (b.position ?? 0)),
    repositories: catalog?.repositories ?? [],
    profiles: catalog?.profiles ?? [],
    modes: [
      { id: "execute", name: t("orchestration:proposalExecutionMode_execute") },
      { id: "design", name: t("orchestration:proposalExecutionMode_design") },
    ],
  };
}

/** Edits a proposal before approving it; only the changed fields are sent. */
export function ProposalEditForm({
  workspaceId,
  spec,
  busy,
  onSubmit,
  onCancel,
}: {
  workspaceId: string;
  spec: ProposalSpec;
  busy: boolean;
  onSubmit: (edits: ProposalEdits) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const descriptionId = useId();
  const [draft, setDraft] = useState(() => draftFromSpec(spec));
  const options = useDraftOptions(workspaceId, draft);
  const patch = (next: Partial<ProposalDraft>) => setDraft({ ...draft, ...next });
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (validDraft(draft) && !busy) onSubmit(proposalEdits(spec, draft));
  };
  return (
    <form className="space-y-3" onSubmit={submit} data-testid="proposal-edit-form">
      <TitleField value={draft.title} onChange={(title) => patch({ title })} />
      <div className="space-y-1">
        <Label htmlFor={descriptionId}>{t("orchestration:proposalFieldDescription")}</Label>
        <Textarea
          id={descriptionId}
          value={draft.description}
          onChange={(event) => patch({ description: event.target.value })}
        />
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <OptionField
          label={t("orchestration:proposalFieldWorkflow")}
          value={draft.workflow_id}
          options={options.workflows}
          onChange={(workflow_id) => patch({ workflow_id, workflow_step_id: "" })}
          testId="proposal-edit-workflow"
        />
        <OptionField
          label={t("orchestration:proposalFieldStep")}
          value={draft.workflow_step_id}
          options={options.steps}
          onChange={(workflow_step_id) => patch({ workflow_step_id })}
          testId="proposal-edit-step"
        />
        <OptionField
          label={t("orchestration:proposalFieldRepository")}
          value={draft.repository_id}
          options={options.repositories}
          onChange={(repository_id) => patch({ repository_id })}
          testId="proposal-edit-repository"
        />
        <OptionField
          label={t("orchestration:proposalFieldAssignee")}
          value={draft.assignee}
          options={options.profiles}
          onChange={(assignee) => patch({ assignee })}
          testId="proposal-edit-assignee"
        />
        <OptionField
          label={t("orchestration:proposalFieldExecutionMode")}
          value={draft.execution_mode}
          options={options.modes}
          onChange={(execution_mode) => patch({ execution_mode })}
          testId="proposal-edit-mode"
        />
      </div>
      <CriteriaEditor
        criteria={draft.acceptance_criteria}
        onChange={(acceptance_criteria) => patch({ acceptance_criteria })}
      />
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          disabled={busy || !validDraft(draft)}
          className={`cursor-pointer ${CONTROL}`}
        >
          {t("orchestration:proposalApproveEdited")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          className={`cursor-pointer ${CONTROL}`}
          onClick={onCancel}
        >
          {t("orchestration:proposalCancelEdit")}
        </Button>
      </div>
    </form>
  );
}
