/** A bounded UTF-8 header preview; unsupported/ambiguous files remain inheritable. */
export function parseTableHeader(
  text: string,
  delimiter = ",",
  requireTerminator = false,
): string[] {
  const source = text.replace(/^\uFEFF/, "");
  const names: string[] = [];
  let value = "",
    quoted = false,
    closed = false;
  for (let i = 0; i < source.length; i++) {
    const char = source[i];
    if (quoted) {
      if (char === '"') {
        if (source[i + 1] === '"') {
          value += '"';
          i++;
        } else {
          quoted = false;
          closed = true;
        }
      } else value += char;
    } else if (char === delimiter || char === "\n" || char === "\r") {
      names.push(value);
      value = "";
      closed = false;
      if (char !== delimiter) return validHeaders(names);
    } else if (char === '"' && value === "" && !closed) quoted = true;
    else {
      if (closed || char === '"') throw new Error("Invalid table header");
      value += char;
    }
  }
  if (quoted || requireTerminator) throw new Error("Incomplete table header");
  return validHeaders([...names, value]);
}
function validHeaders(names: string[]): string[] {
  if (
    !names.length ||
    names.some((name) => !name.trim() || name.includes("\uFFFD")) ||
    new Set(names).size !== names.length
  )
    throw new Error("Ambiguous table headers");
  return names;
}
export async function extractTableHeaders(file: File): Promise<string[]> {
  if (!/\.csv$/i.test(file.name))
    throw new Error("Unsupported header preview");
  const text = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === "string") resolve(reader.result);
      else reject(new Error("Cannot decode table headers"));
    };
    reader.onerror = () => {
      reject(new Error("Cannot read table headers"));
    };
    reader.readAsText(file.slice(0, 262144), "UTF-8");
  });
  if (file.size > 262144 && !/[\r\n]/.test(text))
    throw new Error("Header too large");
  return parseTableHeader(
    text,
    ",",
    file.size > 262144,
  );
}
