export function nodeOrigin() {
  if (location.protocol === "file:") {
    return "http://127.0.0.1:1316";
  }
  return location.origin;
}

export async function requestJson(path, options = {}) {
  const response = await fetch(new URL(path, nodeOrigin()), {
    headers: options.body ? { "content-type": "application/json" } : undefined,
    ...options,
    body: options.body ? JSON.stringify(options.body) : undefined
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;
  if (!response.ok) {
    let message = payload && payload.error ? payload.error : response.statusText;
    if (payload && payload.raw_log) {
      message = payload.raw_log;
    }
    throw new Error(message);
  }
  return payload;
}
