declare namespace Cloudflare {
  interface Env {
    DB?: D1Database;
    AGEN8_EVENTS_ENABLED?: string;
    AGEN8_EVENT_RELAY_URL?: string;
    AGEN8_EVENT_RELAY_TOKEN?: string;
    AGEN8_EVENT_ENCRYPTION_KEY?: string;
    AGEN8_EVENT_GRANT_ID?: string;
    AGEN8_EVENT_DISPATCH_KEY?: string;
    BUCKET?: R2Bucket;
  }
}
