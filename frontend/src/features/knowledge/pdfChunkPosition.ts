// MultiRAG add_positions stores 1-based pages and top-left PDF point units.
// OCR output is divided by the render zoom before positions are persisted.
export function pdfHighlightRect(
  coordinates: number[],
  width: number,
  height: number,
  rotation: number,
  scale: number,
) {
  const [left, right, top, bottom] = coordinates;
  if (
    coordinates.length !== 4 ||
    ![...coordinates, width, height, scale].every(Number.isFinite) ||
    left < 0 ||
    top < 0 ||
    right <= left ||
    bottom <= top ||
    right > width ||
    bottom > height ||
    scale <= 0
  )
    return null;
  switch (rotation) {
    case 0:
      return {
        left: left * scale,
        top: top * scale,
        width: (right - left) * scale,
        height: (bottom - top) * scale,
      };
    case 90:
      return {
        left: (height - bottom) * scale,
        top: left * scale,
        width: (bottom - top) * scale,
        height: (right - left) * scale,
      };
    case 180:
      return {
        left: (width - right) * scale,
        top: (height - bottom) * scale,
        width: (right - left) * scale,
        height: (bottom - top) * scale,
      };
    case 270:
      return {
        left: top * scale,
        top: (width - right) * scale,
        width: (bottom - top) * scale,
        height: (right - left) * scale,
      };
    default:
      return null;
  }
}
