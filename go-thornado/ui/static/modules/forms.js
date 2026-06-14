export function parseListInput(raw) {
  const text = String(raw || "").trim();
  if (!text) {
    return [];
  }
  if (text.startsWith("[")) {
    return JSON.parse(text);
  }
  return text.split(",").map((item) => item.trim()).filter(Boolean);
}
