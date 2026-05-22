export type MaintenanceRouteConfig = {
  enabled?: boolean;
  message?: string;
  image_b64_json?: string;
};

export type MaintenanceRouteGroup = {
  key: string;
  label: string;
  paths: string[];
};

export const MAINTENANCE_ROUTE_GROUPS: MaintenanceRouteGroup[] = [
  { key: "openai-chat", label: "OpenAI Chat", paths: ["/v1/chat/completions"] },
  {
    key: "gpt-image",
    label: "GPT生图",
    paths: ["/v1/images/edits", "/v1/images/generations"],
  },
  { key: "claude", label: "Claude", paths: ["/v1/messages"] },
  {
    key: "codex",
    label: "Codex",
    paths: ["/v1/responses", "/v1/responses/compact"],
  },
];

export const MAINTENANCE_ROUTE_PATHS = MAINTENANCE_ROUTE_GROUPS.flatMap(
  (group) => group.paths,
);

export function extractBase64FromDataURL(dataURL: string) {
  const commaIndex = dataURL.indexOf(",");
  return commaIndex === -1
    ? dataURL.trim()
    : dataURL.slice(commaIndex + 1).trim();
}

function inferBase64ImageMimeType(b64: string) {
  const trimmed = b64.trim();
  if (trimmed.startsWith("/9j/")) return "image/jpeg";
  if (trimmed.startsWith("R0lG")) return "image/gif";
  if (trimmed.startsWith("UklGR")) return "image/webp";
  return "image/png";
}

export function getMaintenanceImagePreview(b64: string) {
  const trimmed = b64.trim();
  if (!trimmed) return "";
  if (trimmed.startsWith("data:image/")) return trimmed;
  return `data:${inferBase64ImageMimeType(trimmed)};base64,${trimmed}`;
}

export function parseMaintenanceRoutesJSON(
  value: string,
): Record<string, MaintenanceRouteConfig> {
  try {
    const parsed = JSON.parse(value || "{}");
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
      return {};
    const result: Record<string, MaintenanceRouteConfig> = {};
    for (const [path, raw] of Object.entries(parsed)) {
      if (!MAINTENANCE_ROUTE_PATHS.includes(path)) continue;
      if (!raw || typeof raw !== "object" || Array.isArray(raw)) continue;
      const item = raw as Record<string, unknown>;
      result[path] = {
        enabled: typeof item.enabled === "boolean" ? item.enabled : undefined,
        message: typeof item.message === "string" ? item.message : "",
        image_b64_json:
          typeof item.image_b64_json === "string" ? item.image_b64_json : "",
      };
    }
    return result;
  } catch {
    return {};
  }
}

export function stringifyMaintenanceRoutes(
  routes: Record<string, MaintenanceRouteConfig>,
) {
  const result: Record<string, MaintenanceRouteConfig> = {};
  for (const path of MAINTENANCE_ROUTE_PATHS) {
    const route = routes[path];
    if (!route) continue;
    const message = route.message?.trim() ?? "";
    const imageB64JSON = route.image_b64_json?.trim() ?? "";
    const enabled = route.enabled;
    if (enabled === false || message || imageB64JSON) {
      result[path] = {
        ...(enabled === false ? { enabled: false } : {}),
        ...(message ? { message } : {}),
        ...(imageB64JSON ? { image_b64_json: imageB64JSON } : {}),
      };
    }
  }
  return JSON.stringify(result, null, 2);
}

export function getMaintenanceGroupConfig(
  routes: Record<string, MaintenanceRouteConfig>,
  group: MaintenanceRouteGroup,
): MaintenanceRouteConfig {
  for (const path of group.paths) {
    const route = routes[path];
    if (route) return route;
  }
  return {};
}

export function updateMaintenanceGroup(
  routes: Record<string, MaintenanceRouteConfig>,
  group: MaintenanceRouteGroup,
  patch: MaintenanceRouteConfig,
) {
  const next = { ...routes };
  for (const path of group.paths) {
    next[path] = { ...(next[path] ?? {}), ...patch };
  }
  return next;
}
