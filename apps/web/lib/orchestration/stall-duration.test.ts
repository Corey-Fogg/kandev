import { describe, expect, it } from "vitest";
import { formatStallDuration, parseGoDuration } from "./stall-duration";

describe("parseGoDuration", () => {
  it("reads Go duration strings", () => {
    expect(parseGoDuration("5m0s")).toBe(300);
    expect(parseGoDuration("2h0m1s")).toBe(7201);
    expect(parseGoDuration("1h2m3.5s")).toBe(3723.5);
    expect(parseGoDuration("500ms")).toBeCloseTo(0.5);
  });

  it("rejects anything else", () => {
    expect(parseGoDuration(undefined)).toBeNull();
    expect(parseGoDuration("")).toBeNull();
    expect(parseGoDuration("5 minutes")).toBeNull();
    expect(parseGoDuration("5m garbage")).toBeNull();
  });
});

describe("formatStallDuration", () => {
  it("formats hours and minutes for the locale", () => {
    expect(formatStallDuration(7201, "en")).toBe("2 hours");
    expect(formatStallDuration(3723, "en")).toBe("1 hour, 2 minutes");
    expect(formatStallDuration(300, "en")).toBe("5 minutes");
    expect(formatStallDuration(10, "en")).toBe("1 minute");
    expect(formatStallDuration(7260, "ja")).not.toContain("hour");
  });
});
