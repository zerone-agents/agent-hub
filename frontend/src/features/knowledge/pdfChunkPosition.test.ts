import { describe, expect, it } from "vitest";
import { pdfHighlightRect } from "./pdfChunkPosition";
describe("PDF chunk position transforms", () => {
  it("uses top-left PDF point coordinates and scales every region", () => {
    expect(pdfHighlightRect([10, 40, 20, 50], 100, 200, 0, 2)).toEqual({
      left: 20,
      top: 40,
      width: 60,
      height: 60,
    });
  });
  it("rotates the source rectangle clockwise around the actual page bounds", () => {
    expect(pdfHighlightRect([10, 40, 20, 50], 100, 200, 90, 1)).toEqual({
      left: 150,
      top: 10,
      width: 30,
      height: 30,
    });
    expect(pdfHighlightRect([10, 40, 20, 50], 100, 200, 180, 1)).toEqual({
      left: 60,
      top: 150,
      width: 30,
      height: 30,
    });
    expect(pdfHighlightRect([10, 40, 20, 50], 100, 200, 270, 1)).toEqual({
      left: 20,
      top: 60,
      width: 30,
      height: 30,
    });
  });
  it("rejects malformed, off-page, negative and empty boxes instead of highlighting guessed areas", () => {
    for (const coordinates of [
      [-1, 40, 20, 50],
      [10, 101, 20, 50],
      [10, 40, 20, 201],
      [40, 10, 20, 50],
      [10, 10, 20, 50],
      [10, 40, 20],
      [NaN, 40, 20, 50],
    ]) {
      expect(pdfHighlightRect(coordinates, 100, 200, 0, 1)).toBeNull();
    }
  });
});
