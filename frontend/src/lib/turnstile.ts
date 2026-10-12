type WidgetOptions = {sitekey: string; action: string; theme: string; callback: (token: string) => void; 'expired-callback': () => void; 'error-callback': () => void};
export type Turnstile = {render: (node: HTMLElement, options: WidgetOptions) => string; remove: (id: string) => void; reset: (id: string) => void};
declare global {interface Window { turnstile?: Turnstile }}
let loading: Promise<Turnstile> | undefined;
export function loadTurnstile(): Promise<Turnstile> {
 if (window.turnstile) return Promise.resolve(window.turnstile);
 if (loading) return loading;
 loading = new Promise<Turnstile>((resolve, reject) => {
  const script = document.createElement('script');
  script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
  script.async = true;
  const timeout = window.setTimeout(failed, 12000);
  function failed() { clearTimeout(timeout); script.remove(); loading = undefined; reject(new Error('Verification could not load. Check your connection and reload.')); }
  script.onload = () => { clearTimeout(timeout); window.turnstile ? resolve(window.turnstile) : failed(); };
  script.onerror = failed;
  document.head.appendChild(script);
 });
 return loading;
}
