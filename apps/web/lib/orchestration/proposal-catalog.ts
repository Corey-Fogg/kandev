import { createContext } from "react";

type Named = { id: string; name: string };

/** Names and choices for proposal cards; the Coordinator view provides its workspace catalog. */
export type ProposalCatalog = {
  workflows: Named[];
  steps: (Named & { workflow_id: string; position?: number })[];
  repositories: Named[];
  profiles: Named[];
};

export const ProposalCatalogContext = createContext<ProposalCatalog | null>(null);

const byId = (items: Named[], id?: string) =>
  id ? (items.find((item) => item.id === id)?.name ?? id) : "";

/** Display names for a spec's references; unknown ids fall back to the raw id. */
export function proposalNames(
  catalog: ProposalCatalog | null,
  spec: {
    workflow_id?: string;
    workflow_step_id?: string;
    repository_id?: string;
    assignee?: string;
  },
) {
  return {
    workflow: byId(catalog?.workflows ?? [], spec.workflow_id),
    step: byId(catalog?.steps ?? [], spec.workflow_step_id),
    repository: byId(catalog?.repositories ?? [], spec.repository_id),
    assignee: byId(catalog?.profiles ?? [], spec.assignee),
  };
}
