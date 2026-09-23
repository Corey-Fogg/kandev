import { describe, expect, it } from "vitest";
import type { ProposalSpec } from "@/lib/api/domains/orchestration-proposals-api";
import { draftFromSpec, proposalEdits, validDraft } from "./proposal-edits";

const spec: ProposalSpec = {
  title: "Fix the parser",
  workflow_id: "wf",
  acceptance_criteria: ["Tests pass"],
};

describe("proposal edits", () => {
  it("sends nothing when nothing changed", () => {
    expect(proposalEdits(spec, draftFromSpec(spec))).toEqual({});
  });

  it("sends only changed fields, trimming the title and dropping blank criteria", () => {
    const draft = {
      ...draftFromSpec(spec),
      title: "  Fix the lexer ",
      repository_id: "repo",
      acceptance_criteria: [" Tests pass ", "", "Docs updated"],
    };
    expect(proposalEdits(spec, draft)).toEqual({
      title: "Fix the lexer",
      repository_id: "repo",
      acceptance_criteria: ["Tests pass", "Docs updated"],
    });
  });

  it("can clear a proposed field", () => {
    expect(proposalEdits(spec, { ...draftFromSpec(spec), workflow_id: "" })).toEqual({
      workflow_id: "",
    });
  });

  it("requires a 1 to 60 character title and at most 10 criteria of 300 characters", () => {
    const draft = draftFromSpec(spec);
    expect(validDraft(draft)).toBe(true);
    expect(validDraft({ ...draft, title: "   " })).toBe(false);
    expect(validDraft({ ...draft, title: "x".repeat(61) })).toBe(false);
    expect(validDraft({ ...draft, acceptance_criteria: Array(11).fill("c") })).toBe(false);
    expect(validDraft({ ...draft, acceptance_criteria: ["c".repeat(301)] })).toBe(false);
  });
});
