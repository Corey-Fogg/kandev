import { createContext, useContext } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfiles } from "@/lib/state/slices/office/selectors";

export type ChatPersona = { id: string; name: string; icon?: string };

/**
 * A conversation that speaks as one persona provides it here, and every agent
 * entry then renders with that persona's name and icon. Without a provider,
 * agents resolve from the Office agent store so renames flow through.
 */
export const ChatIdentityContext = createContext<{ persona: ChatPersona | null } | null>(null);

export function useAgentIdentity(
  agentId: string | undefined,
  fallbackName?: string,
): { name: string; icon?: string } {
  const { t } = useTranslation();
  const conversation = useContext(ChatIdentityContext);
  const officeName = useAppStore((s) =>
    conversation ? undefined : selectOfficeAgentProfiles(s).find((a) => a.id === agentId)?.name,
  );
  if (conversation) {
    const persona = conversation.persona;
    return persona
      ? { name: persona.name, icon: persona.icon }
      : { name: fallbackName || t("task:agent") };
  }
  return { name: officeName ?? fallbackName ?? t("task:agent") };
}
