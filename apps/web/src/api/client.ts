export interface HealthStatus {
  status: string;
}

const defaultAPIBaseURL = "http://127.0.0.1:8080";

export async function fetchHealth(fetcher: typeof fetch = fetch): Promise<HealthStatus> {
  const baseURL = import.meta.env.VITE_API_BASE_URL || defaultAPIBaseURL;
  const response = await fetcher(new URL("/healthz", baseURL));
  if (!response.ok) {
    throw new Error(`Foundation API health request failed with status ${response.status}`);
  }
  return response.json() as Promise<HealthStatus>;
}
