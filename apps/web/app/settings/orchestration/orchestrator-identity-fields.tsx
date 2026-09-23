import { useId } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { OrchestratorConfiguration } from "@/lib/api/domains/orchestration-api";

export const DISPLAY_NAME_MAX = 60;

/** A display name the backend accepts: empty (inherit) or at most 60 characters. */
export function validDisplayName(value: string) {
  return [...value.trim()].length <= DISPLAY_NAME_MAX;
}

type Props = {
  value: OrchestratorConfiguration;
  onChange: (patch: Partial<OrchestratorConfiguration>) => void;
  roleName: string;
};

export function OrchestratorIdentityFields({ value, onChange, roleName }: Props) {
  const { t } = useTranslation();
  const id = useId();
  const invalid = !validDisplayName(value.display_name);
  return (
    <fieldset className="space-y-2" data-testid="orchestrator-identity">
      <legend className="text-sm font-semibold">{t("orchestration:identitySection")}</legend>
      <Label htmlFor={id}>{t("orchestration:displayName")}</Label>
      <Input
        id={id}
        value={value.display_name}
        maxLength={DISPLAY_NAME_MAX}
        placeholder={roleName}
        aria-invalid={invalid || undefined}
        aria-describedby={`${id}-hint`}
        onChange={(event) => onChange({ display_name: event.target.value })}
        data-testid="orchestrator-display-name"
      />
      <p id={`${id}-hint`} className="text-xs text-muted-foreground">
        {t("orchestration:displayNameHint", { role: roleName })}
      </p>
      {invalid && (
        <p role="alert" className="text-xs text-destructive">
          {t("orchestration:displayNameTooLong")}
        </p>
      )}
    </fieldset>
  );
}

// Catalog keys, not copy: they resolve through `t()` at render.
const BEHAVIORS = [
  {
    key: "ask_before_create",
    label: "orchestration:askBeforeCreate",
    hint: "orchestration:askBeforeCreateHint",
  },
  {
    key: "auto_comment_source",
    label: "orchestration:autoCommentSource",
    hint: "orchestration:autoCommentSourceHint",
  },
  {
    key: "auto_move_source_done",
    label: "orchestration:autoMoveSourceDone",
    hint: "orchestration:autoMoveSourceDoneHint",
  },
] as const;

export function OrchestratorBehaviorFields({ value, onChange }: Omit<Props, "roleName">) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <fieldset className="space-y-4" data-testid="orchestrator-behavior">
      <legend className="text-sm font-semibold">{t("orchestration:behaviorSection")}</legend>
      {BEHAVIORS.map(({ key, label, hint }) => (
        <div key={key} className="flex items-start gap-3">
          <Switch
            id={`${id}-${key}`}
            checked={value[key]}
            onCheckedChange={(checked) => onChange({ [key]: checked })}
            aria-describedby={`${id}-${key}-hint`}
            className="mt-1"
            data-testid={`orchestrator-${key}`}
          />
          <div className="space-y-1">
            <Label htmlFor={`${id}-${key}`} className="cursor-pointer">
              {t(label)}
            </Label>
            <p id={`${id}-${key}-hint`} className="text-xs text-muted-foreground">
              {t(hint)}
            </p>
          </div>
        </div>
      ))}
    </fieldset>
  );
}
