/**
 * Spooly — a portable, dependency-free presentation engine. Import this module
 * and call registerMascot({ gesture }) on the client. ERP's gesture adapter uses
 * the shared Anime.js helper; no request, credential or business record enters
 * this component. The API hook accepts already-parsed, explicitly supplied data.
 *
 * @typedef {'inventory'|'sewing'|'cutting'|'qc'|'dispatch'|'general'} DashboardModule
 * @typedef {'idle'|'under_construction'|'live_api'|'event_trigger'} BotStatus
 * @typedef {{primary:string,secondary:string,glow:string}} ColorTheme
 * @typedef {{dashboardModule:DashboardModule,colorTheme:ColorTheme,botStatus:BotStatus,aiApiEndpoint:string}} MascotConfig
 * @typedef {{minimized:boolean,theme:'auto'|DashboardModule,quietMotion:boolean}} MascotPreferences
 */

const MODULES = Object.freeze(['inventory', 'sewing', 'cutting', 'qc', 'dispatch', 'general']);
const STATUSES = Object.freeze(['idle', 'under_construction', 'live_api', 'event_trigger']);
const EMOTIONS = Object.freeze(['neutral', 'happy', 'thinking', 'celebrate']);
const GESTURES = Object.freeze(['none', 'wave', 'nod', 'celebrate']);
const TAG = 'garment-erp-mascot';

export const MASCOT_THEMES = Object.freeze({
  inventory: Object.freeze({ label: 'Inventory & fabric', visualTheme: 'Cotton spool with measuring scarf', primary: '#10B981', secondary: '#99F6E4', glow: '#10B981', idle: 'count-spools', defaultEmotion: 'thinking', tooltip: "I'm calibrating fabric sensors... AI coming soon!" }),
  sewing: Object.freeze({ label: 'Sewing floor', visualTheme: 'Needle buddy with thimble hat', primary: '#F59E0B', secondary: '#FDE68A', glow: '#F59E0B', idle: 'stitch-bob', defaultEmotion: 'happy', tooltip: 'Stitch tracking AI module launching shortly!' }),
  cutting: Object.freeze({ label: 'Cutting & patterning', visualTheme: 'Shears & chalk', primary: '#6366F1', secondary: '#C7D2FE', glow: '#6366F1', idle: 'trace-curve', defaultEmotion: 'thinking', tooltip: 'Pattern optimizer connecting to cloud AI...' }),
  qc: Object.freeze({ label: 'Quality control', visualTheme: 'Golden button with magnifier', primary: '#EC4899', secondary: '#FBCFE8', glow: '#EC4899', idle: 'glowing-check', defaultEmotion: 'happy', tooltip: 'AI Defect Scan assistant powering up...' }),
  dispatch: Object.freeze({ label: 'Dispatch & shipping', visualTheme: 'Packing tag buddy', primary: '#3B82F6', secondary: '#BFDBFE', glow: '#3B82F6', idle: 'packing-list', defaultEmotion: 'neutral', tooltip: 'Logistics auto-agent arriving soon!' }),
  general: Object.freeze({ label: 'Admin & executive', visualTheme: 'Navy cotton spool with teal thread', primary: '#164E63', secondary: '#5EEAD4', glow: '#14B8A6', idle: 'thread-float', defaultEmotion: 'thinking', tooltip: 'A little thread of intelligence is on its way!' })
});

const COLOR = { type: 'string', pattern: '^(#[0-9A-Fa-f]{6}|hsl\\([0-9.,% /+-]+\\))$' };
export const MASCOT_CONFIG_SCHEMA = Object.freeze({
  $schema: 'https://json-schema.org/draft/2020-12/schema',
  title: 'MascotConfig', type: 'object', additionalProperties: false,
  required: ['dashboardModule', 'colorTheme', 'botStatus', 'aiApiEndpoint'],
  properties: {
    dashboardModule: { type: 'string', enum: [...MODULES], default: 'general' },
    colorTheme: { type: 'object', additionalProperties: false, required: ['primary', 'secondary', 'glow'], properties: { primary: { ...COLOR }, secondary: { ...COLOR }, glow: { ...COLOR } } },
    botStatus: { type: 'string', enum: [...STATUSES], default: 'under_construction' },
    aiApiEndpoint: { type: 'string', maxLength: 2048, default: '', description: 'Reserved for a future authenticated host integration. The mascot never fetches this value.' }
  }
});

/** Validate one display-only response. Unknown fields cannot smuggle commands or URLs. */
export function normalizeAIResponse(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload) || ![Object.prototype, null].includes(Object.getPrototypeOf(payload))) {
    throw new TypeError('Spooly expects a parsed JSON object.');
  }
  const allowed = ['message', 'emotion', 'gesture', 'botStatus'];
  if (Object.keys(payload).some(key => !allowed.includes(key))) throw new TypeError('Unsupported Spooly response field.');
  if (typeof payload.message !== 'string' || !payload.message.trim() || payload.message.length > 280) throw new TypeError('Spooly message must contain 1–280 characters.');
  const emotion = payload.emotion === undefined ? 'neutral' : payload.emotion;
  const gesture = payload.gesture === undefined ? 'none' : payload.gesture;
  const botStatus = payload.botStatus === undefined ? 'event_trigger' : payload.botStatus;
  if (!EMOTIONS.includes(emotion) || !GESTURES.includes(gesture) || !STATUSES.includes(botStatus)) throw new TypeError('Unsupported Spooly emotion, gesture or status.');
  return Object.freeze({ message: payload.message.trim(), emotion, gesture, botStatus });
}

/** Page wins over role; shared overview inherits the signed-in department. */
export function resolveMascotContext(page = '', role = '', portal = '') {
  const current = String(page).trim().toLowerCase();
  if (/quality|\bqc\b/.test(current)) return 'qc';
  if (/sewing/.test(current)) return 'sewing';
  if (/cutting|pattern|\bcad\b|development/.test(current)) return 'cutting';
  if (/dispatch|shipping|warehouse|packing|finishing/.test(current)) return 'dispatch';
  if (/inventory|fabric|material|purchases|suppliers|stores/.test(current)) return 'inventory';
  if (/user access|compan|reminders|fields|ports|teams|buyers|reports|audit|approvals|developer|buyer orders|enquir|costing|sampling|planning/.test(current)) return 'general';
  const identity = `${String(role)} ${String(portal)}`.toLowerCase();
  if (/quality|\bqc\b/.test(identity)) return 'qc';
  if (/sewing|production/.test(identity)) return 'sewing';
  if (/cutting|pattern|\bcad\b|development/.test(identity)) return 'cutting';
  if (/dispatch|shipping|warehouse|packing|finishing/.test(identity)) return 'dispatch';
  if (/inventory|fabric|sourcing|stores/.test(identity)) return 'inventory';
  return 'general';
}

const STYLE = `
  :host{--mascot-primary:#164E63;--mascot-secondary:#5EEAD4;--mascot-glow:#14B8A6;position:fixed;right:20px;bottom:20px;z-index:9999;width:180px;max-width:calc(100vw - 40px);color:#f3f7fa;font:13px/1.45 "DM Sans",system-ui,sans-serif;pointer-events:none;isolation:isolate;color-scheme:dark}
  *,*::before,*::after{box-sizing:border-box} [hidden]{display:none!important}
  button,select,input{font:inherit}button,select{color:inherit}button{touch-action:manipulation;-webkit-tap-highlight-color:transparent;cursor:pointer}
  button:focus-visible,select:focus-visible,input:focus-visible{outline:3px solid #5EEAD4;outline-offset:3px}
  button:disabled{cursor:default;opacity:.6} ::selection{background:#115e59;color:#fff}
  .stack{display:flex;flex-direction:column;align-items:flex-end;gap:9px}.panel{pointer-events:auto;width:180px;max-width:100%;background:#0A0A0C;box-shadow:0 8px 26px #0a142b29;border-radius:14px;padding:12px;text-align:left;overflow-wrap:anywhere}
  .panel-top{display:flex;align-items:center;justify-content:space-between;gap:6px;margin-bottom:8px}.panel-top strong{font-size:13px;font-weight:700}.close,.tool{display:grid;place-items:center;background:#183344;border:0;border-radius:8px;min-width:28px;height:28px;padding:5px;color:#e4f5f3}.close:hover,.tool:hover{background:#245066}.close svg,.tool svg{width:16px;height:16px}
  .speech{max-height:min(245px,45vh);overflow:auto;scrollbar-color:#47707b #0A0A0C;scrollbar-width:thin}.speech p{font-size:12px;line-height:1.55;min-height:38px;margin:0 0 11px;color:#e2edf0}.speech .status{display:block;margin-top:9px;color:#b5d0d3;font-size:10px}.preview{display:flex;justify-content:space-between;gap:6px;color:#b9d6d8;font-size:10px;margin-bottom:5px}.preview b{color:#f3f7fa;font-weight:600}.progress{display:flex;gap:3px;height:5px;border-radius:2px;overflow:hidden}.progress span{flex:1;border-radius:1px;background:var(--mascot-secondary);opacity:.45;animation:build-light 4.8s ease-in-out infinite}.progress span:nth-child(2){animation-delay:.6s}.progress span:nth-child(3){animation-delay:1.2s}.progress span:nth-child(4){animation-delay:1.8s}
  .settings label{display:block;margin:10px 0 5px;font-size:12px;color:#e2edf0}.settings select{width:100%;background:#183344;border:1px solid #456371;border-radius:6px;padding:7px 5px;min-height:36px;font-size:12px}.settings .check{display:flex;align-items:center;gap:7px;margin-top:12px}.settings input{width:16px;height:16px;margin:0;accent-color:#5eead4}.settings small{display:block;margin-top:9px;font-size:10px;color:#b5d0d3}
  .figure{position:relative;width:132px;pointer-events:none;display:flex;flex-direction:column;align-items:center}.character{display:block;position:relative;pointer-events:auto;border:0;background:transparent;padding:0;width:112px;height:132px;border-radius:28px}.character:hover .outline{stroke:var(--mascot-secondary)}.character svg{display:block;width:112px;height:132px;overflow:visible}.body{transform-origin:65px 75px;animation:breathe 4s ease-in-out infinite}.floating{animation:float 5s ease-in-out infinite}.eyes{transform-origin:65px 64px;animation:blink 6s ease-in-out infinite}.pupil{will-change:transform}.primary{fill:var(--mascot-primary);transition:fill .22s ease-out}.secondary{fill:var(--mascot-secondary);transition:fill .22s ease-out}.outline{stroke:#16344b;stroke-width:2;stroke-linejoin:round}.thread-line{stroke:var(--mascot-secondary);stroke-width:1.4;opacity:.4;fill:none}.limb{stroke:#16344b;stroke-width:5;fill:none;stroke-linecap:round}.accessory{display:none}.glow{fill:var(--mascot-glow);opacity:.07}.shadow{fill:#0a142b;opacity:.13}.badge{pointer-events:none;display:flex;align-items:center;gap:5px;background:#102d3e;border-radius:6px;padding:3px 7px;color:#e7f2f4;font-size:10px;line-height:1.4}.badge strong{font-weight:700}.badge span{font-size:9px;color:#b6d6d8}.toolbar{pointer-events:auto;display:flex;position:absolute;right:0;top:6px;flex-direction:column;gap:5px}.tool{width:28px;height:28px;background:#102d3e}.dock{pointer-events:auto;width:36px;height:36px;padding:0;display:grid;place-items:center;border:0;border-radius:50%;background:#102d3e;color:#5eead4;box-shadow:0 4px 12px #0a142b22}.dock:hover{background:#1d4959}.dock svg{width:25px;height:25px}
  /* Construction is an illustrative loop, never a measured completion value.
     The existing data-still/suspended/media guards stop this entire sequence. */
  .build-block{stroke:#16344b;stroke-width:1.5;stroke-linejoin:round}.build-block.first{animation:build-first 4.8s cubic-bezier(.16,1,.3,1) infinite}.build-block.second{animation:build-second 4.8s cubic-bezier(.16,1,.3,1) infinite}.build-block.third{animation:build-third 4.8s cubic-bezier(.16,1,.3,1) infinite}.build-hammer{transform-origin:91px 120px;animation:build-tap 4.8s ease-in-out infinite}.helmet-ridge{stroke:#B7791F;stroke-width:2;stroke-linecap:round}.sewing-thimble{transform:translate(-12px,116px) scale(.65)}
  :host([data-module="inventory"]) .inventory,:host([data-module="sewing"]) .sewing,:host([data-module="cutting"]) .cutting,:host([data-module="qc"]) .qc,:host([data-module="dispatch"]) .dispatch,:host([data-module="general"]) .general{display:inline}
  :host([data-module="qc"]) .spool-base,:host([data-module="dispatch"]) .spool-base{display:none}
  :host([data-module="inventory"]) .tiny-spool{animation:count 3s ease-in-out infinite}:host([data-module="inventory"]) .tiny-spool.second{animation-delay:.6s}:host([data-module="sewing"]) .floating{animation:stitch 2.6s ease-in-out infinite}:host([data-module="cutting"]) .chalk-path{stroke-dasharray:36;animation:trace 4s ease-in-out infinite}:host([data-module="qc"]) .checkmark{animation:check 3s ease-in-out infinite}:host([data-module="dispatch"]) .packing-tick{animation:check 3s ease-in-out infinite}
  .mouth-happy,.mouth-thinking{display:none}:host([data-emotion="happy"]) .mouth-neutral,:host([data-emotion="celebrate"]) .mouth-neutral,:host([data-emotion="thinking"]) .mouth-neutral{display:none}:host([data-emotion="happy"]) .mouth-happy,:host([data-emotion="celebrate"]) .mouth-happy,:host([data-emotion="thinking"]) .mouth-thinking{display:inline}:host([data-emotion="thinking"]) .brow{transform:translateY(-2px) rotate(-4deg);transform-origin:center}
  :host([data-still]) *, :host([suspended]) *{animation:none!important;transition:none!important}.gesture-wave{animation:wave .55s ease-out!important}.gesture-nod{animation:nod .45s ease-out!important}.gesture-celebrate{animation:celebrate .65s ease-out!important}
  :host([suspended]){display:none;pointer-events:none}:host([data-minimized]){width:36px}
  @keyframes breathe{0%,100%{transform:scale(1)}50%{transform:scale(1.018,1.025)}}@keyframes float{0%,100%{transform:translateY(0)}50%{transform:translateY(-2px)}}@keyframes blink{0%,44%,48%,100%{transform:scaleY(1)}46%{transform:scaleY(.08)}}@keyframes stitch{0%,100%{transform:translateY(0)}45%{transform:translateY(-2px)}55%{transform:translateY(1px)}}@keyframes count{0%,100%{transform:translateY(0);opacity:.6}50%{transform:translateY(-3px);opacity:1}}@keyframes trace{0%,100%{stroke-dashoffset:36;opacity:.4}60%{stroke-dashoffset:0;opacity:1}}@keyframes check{0%,100%{opacity:.6}50%{opacity:1}}@keyframes wave{0%,100%{transform:rotate(0)}35%{transform:rotate(-6deg)}70%{transform:rotate(5deg)}}@keyframes nod{0%,100%{transform:translateY(0)}45%{transform:translateY(3px)}}@keyframes celebrate{0%,100%{transform:translateY(0) rotate(0)}40%{transform:translateY(-5px) rotate(-4deg)}70%{transform:translateY(-2px) rotate(3deg)}}
  @keyframes build-first{0%,8%{opacity:.25;transform:translateY(-7px)}20%,88%{opacity:1;transform:translateY(0)}96%,100%{opacity:.25;transform:translateY(-7px)}}@keyframes build-second{0%,24%{opacity:.25;transform:translateY(-7px)}36%,88%{opacity:1;transform:translateY(0)}96%,100%{opacity:.25;transform:translateY(-7px)}}@keyframes build-third{0%,40%{opacity:.25;transform:translateY(-7px)}52%,88%{opacity:1;transform:translateY(0)}96%,100%{opacity:.25;transform:translateY(-7px)}}@keyframes build-tap{0%,55%,74%,90%,100%{transform:rotate(-22deg)}65%,83%{transform:rotate(9deg)}}@keyframes build-light{0%,100%{opacity:.3}35%,60%{opacity:1}}
  @media(prefers-reduced-motion:reduce){*,*::before,*::after{animation:none!important;transition:none!important}}@media(max-height:450px){.speech{max-height:120px}.settings{max-height:170px;overflow:auto}}@media print{:host{display:none}}
`;

const ICONS = {
  close: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m5 5 10 10M15 5 5 15"/></svg>',
  minimize: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M4 10h12"/></svg>',
  settings: '<svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M4 5h12M4 10h12M4 15h12"/><circle cx="7" cy="5" r="2" fill="#102d3e"/><circle cx="13" cy="10" r="2" fill="#102d3e"/><circle cx="8" cy="15" r="2" fill="#102d3e"/></svg>',
  dock: '<svg viewBox="0 0 30 30" fill="none" aria-hidden="true"><rect x="7" y="6" width="16" height="4" rx="2" fill="currentColor"/><rect x="7" y="20" width="16" height="4" rx="2" fill="currentColor"/><path d="M10 10h10v10H10z" fill="currentColor" opacity=".7"/><circle cx="13" cy="14" r="1.3" fill="#102d3e"/><circle cx="18" cy="14" r="1.3" fill="#102d3e"/><path d="M13 17q2.5 2 5 0" stroke="#102d3e" stroke-width="1.4" stroke-linecap="round"/></svg>'
};

// Inline, crisp geometry. No remote artwork, filters, scripts or SVG URLs.
const CHARACTER = `<svg viewBox="0 0 130 153" fill="none" aria-hidden="true">
 <ellipse class="shadow" cx="66" cy="116" rx="32" ry="4"/><ellipse class="glow" cx="66" cy="70" rx="47" ry="47"/>
 <g class="floating"><g class="gesture"><g class="body">
  <path class="limb" d="M42 81q-14-2-16 6M90 81q10-2 14-11M53 99l-3 13M79 99l3 13"/><path d="M43 113h12M78 113h12" stroke="#16344b" stroke-width="5" stroke-linecap="round"/>
  <g class="spool-base"><rect class="primary outline" x="36" y="39" width="60" height="59" rx="15"/><rect class="secondary outline" x="29" y="30" width="74" height="14" rx="7"/><rect class="secondary outline" x="29" y="94" width="74" height="14" rx="7"/><path class="thread-line" d="M39 48h54M39 53h54M39 86h54M39 91h54"/><ellipse cx="66" cy="36" rx="11" ry="2.5" fill="#16344b" opacity=".3"/></g>
  <g class="accessory qc"><circle cx="65" cy="69" r="34" fill="#F6CE68" class="outline"/><circle cx="65" cy="69" r="28" stroke="#B38723" stroke-width="2"/><circle cx="54" cy="83" r="2" fill="#B38723"/><circle cx="76" cy="83" r="2" fill="#B38723"/></g>
  <g class="accessory dispatch"><path d="m42 34 42 0 14 16-5 47-52 0-7-47Z" fill="#DBEAFE" class="outline"/><circle cx="64" cy="42" r="4" class="primary"/><path d="M64 39q-3-14 7-20" stroke="#16344b" stroke-width="2.5" stroke-linecap="round"/><path d="M41 83h50v11H41z" class="primary"/></g>
  <g class="construction-helmet"><path d="M45 30c0-12 8-19 21-19s21 7 21 19" fill="#FBBF24" class="outline"/><path d="M60 13v14m12-14v14" class="helmet-ridge"/><rect x="39" y="28" width="54" height="8" rx="4" fill="#FCD34D" class="outline"/><rect x="62" y="14" width="8" height="10" rx="2" fill="#16344B"/><path d="M64 19h4" stroke="#5EEAD4" stroke-width="1.5" stroke-linecap="round"/></g>
  <g class="eyes"><ellipse cx="53" cy="64" rx="9" ry="11" fill="#F8FAFC"/><ellipse cx="79" cy="64" rx="9" ry="11" fill="#F8FAFC"/><circle class="pupil" data-eye="left" cx="54" cy="65" r="4.5" fill="#102D3E"/><circle class="pupil" data-eye="right" cx="78" cy="65" r="4.5" fill="#102D3E"/><circle cx="55.5" cy="63.5" r="1.3" fill="#fff"/><circle cx="79.5" cy="63.5" r="1.3" fill="#fff"/></g>
  <path class="brow" d="m47 50 10-1M74 49l10 1" stroke="#16344b" stroke-width="2" stroke-linecap="round"/><path class="mouth-neutral" d="M61 79q5 3 10 0" stroke="#102d3e" stroke-width="2.5" stroke-linecap="round"/><path class="mouth-happy" d="M58 78q8 12 16 0Z" fill="#102d3e"/><path class="mouth-thinking" d="M64 81h7" stroke="#102d3e" stroke-width="2.5" stroke-linecap="round"/>
  <g class="accessory general"><path d="M90 88q17 12 20 0" stroke="#5EEAD4" stroke-width="2" stroke-linecap="round"/><path d="M39 93h13v5H39z" fill="#5EEAD4"/></g>
  <g class="accessory inventory"><path d="M36 84q28 9 60 0v8q-28 9-60 0Z" fill="#FFF3C9" class="outline"/><path d="m88 88-2 25 12-1-3-27" fill="#FFF3C9" class="outline"/><path d="M44 87v4m7-3v5m7-4v5m7-4v5m7-5v5m7-6v5m11 6h5m-5 7h5" stroke="#907C42" stroke-width="1.4"/><g class="tiny-spool"><rect x="12" y="44" width="11" height="14" rx="3" class="primary"/><path d="M11 44h13M11 58h13" stroke="#164e63" stroke-width="3" stroke-linecap="round"/></g><g class="tiny-spool second"><rect x="107" y="51" width="9" height="11" rx="2" class="secondary"/><path d="M106 51h11M106 62h11" stroke="#164e63" stroke-width="2.5" stroke-linecap="round"/></g></g>
  <g class="accessory sewing"><path d="m107 72 6-35" stroke="#E2E8F0" stroke-width="3" stroke-linecap="round"/><path d="m107 72 6-35" stroke="#16344b" stroke-width="1"/><ellipse cx="112" cy="42" rx="1.6" ry="4" transform="rotate(9 112 42)" stroke="#16344b"/><path d="M112 39q13-7 9 14" stroke="#F59E0B" stroke-width="2"/><path d="M17 95h13" stroke="#F59E0B" stroke-width="2" stroke-dasharray="3 3"/></g>
  <g class="accessory cutting"><path d="M16 98q12-21 25-10" class="chalk-path" stroke="#6366F1" stroke-width="2" stroke-linecap="round"/><path d="m19 81 7-7" stroke="#EDF0FF" stroke-width="5" stroke-linecap="round"/><g transform="translate(105 82) rotate(-12)"><circle cx="-3" cy="9" r="4.5" stroke="#6366F1" stroke-width="3"/><circle cx="8" cy="9" r="4.5" stroke="#6366F1" stroke-width="3"/><path d="m-1 5 9-19M6 5l-7-19" stroke="#16344b" stroke-width="2.5" stroke-linecap="round"/><circle cx="3" cy="0" r="2" fill="#C7D2FE"/></g></g>
  <g class="accessory qc"><path d="m95 82 10 15" stroke="#16344b" stroke-width="5" stroke-linecap="round"/><circle cx="91" cy="76" r="13" fill="#EC4899" fill-opacity=".2" stroke="#16344b" stroke-width="3"/><circle cx="91" cy="76" r="10" stroke="#FBCFE8" stroke-width="2"/><path class="checkmark" d="m84 76 5 4 8-10" stroke="#FFF" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"/></g>
  <g class="accessory dispatch"><rect x="96" y="69" width="21" height="30" rx="3" fill="#F8FAFC" class="outline"/><path d="M106 77h7m-7 7h7m-7 7h7" stroke="#7292B6" stroke-width="1.6"/><path class="packing-tick" d="m99 77 2 2 3-5m-5 11 2 2 3-5m-5 11 2 2 3-5" stroke="#3B82F6" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></g>
 </g></g></g>
 <g class="construction-site">
  <path d="M21 138v9m88-9v9M18 137h94" stroke="#16344B" stroke-width="4" stroke-linecap="round"/>
  <rect x="17" y="134" width="96" height="5" rx="2" fill="#FCD34D" stroke="#16344B" stroke-width="1.5"/>
  <path d="m26 135 3 3m7-3 3 3m55-3 3 3m7-3 3 3" stroke="#16344B" stroke-width="2"/>
  <g class="accessory sewing sewing-thimble"><path d="m51 30 3-18q11-7 22 0l4 18Z" fill="#E6EBEE" class="outline"/><path d="M60 15v3m8-3v3m-11 4v3m8-3v3m8-3v3" stroke="#748999" stroke-width="2" stroke-linecap="round"/></g>
  <rect class="build-block first primary" x="48" y="125" width="13" height="9" rx="2"/>
  <rect class="build-block second secondary" x="62" y="125" width="13" height="9" rx="2"/>
  <rect class="build-block third primary" x="55" y="115" width="13" height="9" rx="2"/>
  <g class="build-hammer"><path d="m91 120-14-13" stroke="#16344B" stroke-width="5" stroke-linecap="round"/><path d="m91 120-14-13" stroke="#C58F43" stroke-width="2.5" stroke-linecap="round"/><path d="m71 104 4-4 10 10-4 4Z" fill="#E2E8F0" stroke="#16344B" stroke-width="1.5" stroke-linejoin="round"/></g>
 </g></svg>`;

const TEMPLATE = `<style>${STYLE}</style><div class="stack">
 <section class="panel speech" id="speech" aria-label="Spooly preview" hidden><div class="panel-top"><strong>Spooly · coming soon</strong><button class="close" data-action="close-speech" aria-label="Close Spooly message">${ICONS.close}</button></div><p id="message"></p><div class="preview"><b>Preview only</b><span>Coming soon</span></div><div class="progress" aria-hidden="true"><span></span><span></span><span></span><span></span></div><span class="status">Building your AI assistant</span><span class="status" id="status">AI is not connected.</span></section>
 <section class="panel settings" id="settings" aria-label="Spooly preferences" hidden><div class="panel-top"><strong>Your Spooly</strong><button class="close" data-action="close-settings" aria-label="Close Spooly preferences">${ICONS.close}</button></div><label for="theme">Appearance</label><select id="theme"><option value="auto">Match my workspace</option>${MODULES.map(module => `<option value="${module}">${MASCOT_THEMES[module].label}</option>`).join('')}</select><label class="check" for="quiet"><input id="quiet" type="checkbox"/>Quiet motion</label><small>Saved in this browser for your account.</small></section>
 <div class="figure" id="figure"><button class="character" id="character" data-action="speak" aria-label="Meet Spooly, your coming-soon assistant" aria-controls="speech" aria-expanded="false">${CHARACTER}</button><span class="badge"><strong>Spooly</strong><span>Coming soon</span></span><div class="toolbar"><button class="tool" id="preferences" data-action="settings" aria-label="Spooly preferences" aria-controls="settings" aria-expanded="false" title="Spooly preferences">${ICONS.settings}</button><button class="tool" data-action="minimize" aria-label="Minimize Spooly" title="Minimize Spooly">${ICONS.minimize}</button></div></div>
 <button class="dock" id="dock" data-action="restore" aria-label="Restore Spooly, coming-soon assistant" title="Restore Spooly" hidden>${ICONS.dock}</button>
</div>`;

function defaultPreferences() { return { minimized: false, theme: 'auto', quietMotion: false }; }

/** No globals are installed. Use these exports from any dashboard integration. */
export const MascotEngine = Object.freeze({
  setContext(moduleName) {
    if (!MODULES.includes(moduleName)) throw new TypeError('Unknown Spooly dashboard module.');
    const elements = typeof document === 'undefined' ? [] : [...document.querySelectorAll(TAG)];
    elements.forEach(element => element.setContext?.(moduleName));
    return elements.length;
  },
  async dispatchAIResponse(payload) {
    const response = normalizeAIResponse(payload);
    const elements = typeof document === 'undefined' ? [] : [...document.querySelectorAll(TAG)];
    await Promise.all(elements.map(element => element.dispatchAIResponse?.(response)));
    return response;
  }
});

/**
 * @param {{gesture?:(node:Element,name:string)=>(()=>void)|void}} options
 * @returns {CustomElementConstructor|undefined}
 */
export function registerMascot(options = {}) {
  if (typeof window === 'undefined' || !window.customElements) return undefined;
  if (window.customElements.get(TAG)) return window.customElements.get(TAG);

  class GarmentERPMascot extends HTMLElement {
    static get observedAttributes() { return ['dashboard-module', 'user-role', 'user-key', 'suspended']; }

    constructor() {
      super();
      this.attachShadow({ mode: 'open' });
      this.shadowRoot.innerHTML = TEMPLATE;
      this._prefs = defaultPreferences();
      this._module = 'general';
      this._themeInitialized = false;
      this._status = 'under_construction';
      this._emotion = 'neutral';
      this._message = '';
      this._pendingResponse = null;
      this._messageIndex = 0;
      this._connected = false;
      this._listening = false;
      this._idleUntil = 0;
      this._frame = 0;
      this._typing = 0;
      this._idleTimer = 0;
      this._gestureTimer = 0;
      this._gestureCleanup = null;
      this._cursor = { x: 0, y: 0, tx: 0, ty: 0 };
      this._bounds = null;
      this._elements = Object.fromEntries(['speech', 'message', 'settings', 'figure', 'character', 'preferences', 'dock', 'theme', 'quiet', 'status'].map(id => [id, this.shadowRoot.getElementById(id)]));
      this._pupils = [...this.shadowRoot.querySelectorAll('.pupil')];
      this._gestureNode = this.shadowRoot.querySelector('.gesture');
      this._onClick = event => this._click(event);
      this._onKey = event => { if (event.key === 'Escape' && (!this._elements.speech.hidden || !this._elements.settings.hidden)) { event.preventDefault(); event.stopPropagation(); this._closePanels(true); } };
      this._onChange = event => { if (event.target === this._elements.theme) this.setPreferences({ theme: event.target.value }); if (event.target === this._elements.quiet) this.setPreferences({ quietMotion: event.target.checked }); };
      this._onVisibility = () => this._syncActivity();
      this._onMedia = () => this._syncActivity();
      this._onMove = event => this._move(event);
      this._onLeave = () => this._targetEyes(0, 0);
      this._onResize = () => { this._bounds = null; this._targetEyes(0, 0); };
      this._onEnter = () => this._wake();
      this._onStorage = event => { if (event.key === this._storageKey()) { this._readPreferences(); this._applyTheme(); this._renderPreferences(); this._syncActivity(); } };
    }

    connectedCallback() {
      if (this._connected) return;
      this._connected = true;
      this._media = window.matchMedia('(prefers-reduced-motion: reduce)');
      this._media.addEventListener('change', this._onMedia);
      this.shadowRoot.addEventListener('click', this._onClick);
      this.shadowRoot.addEventListener('keydown', this._onKey);
      this.shadowRoot.addEventListener('change', this._onChange);
      this._elements.character.addEventListener('pointerenter', this._onEnter);
      document.addEventListener('visibilitychange', this._onVisibility);
      window.addEventListener('resize', this._onResize, { passive: true });
      window.addEventListener('storage', this._onStorage);
      this._readPreferences();
      this._applyTheme();
      this._renderPreferences();
      this._wake();
    }

    disconnectedCallback() {
      this._connected = false;
      this._media?.removeEventListener('change', this._onMedia);
      this.shadowRoot.removeEventListener('click', this._onClick);
      this.shadowRoot.removeEventListener('keydown', this._onKey);
      this.shadowRoot.removeEventListener('change', this._onChange);
      this._elements.character.removeEventListener('pointerenter', this._onEnter);
      document.removeEventListener('visibilitychange', this._onVisibility);
      window.removeEventListener('resize', this._onResize);
      window.removeEventListener('storage', this._onStorage);
      this._stopAll();
    }

    attributeChangedCallback(name, previous, value) {
      if (previous === value || !this._connected) return;
      if (name === 'suspended') { if (value !== null) this._closePanels(false); this._syncActivity(); return; }
      if (name === 'user-key') { this._readPreferences(); this._closePanels(false); this._message = ''; this._pendingResponse = null; this._messageIndex = 0; this._themeInitialized = false; this._elements.message.textContent = ''; this._elements.message.removeAttribute('aria-label'); this._status = 'under_construction'; this._emotion = 'neutral'; this._renderPreferences(); }
      this._applyTheme();
      this._wake();
    }

    /** Returns a detached, serializable configuration snapshot. */
    get config() {
      const theme = MASCOT_THEMES[this._module];
      return { dashboardModule: this._module, colorTheme: { primary: theme.primary, secondary: theme.secondary, glow: theme.glow }, botStatus: this._status, aiApiEndpoint: '' };
    }
    get preferences() { return { ...this._prefs }; }

    setContext(moduleName) {
      if (!MODULES.includes(moduleName)) throw new TypeError('Unknown Spooly dashboard module.');
      this.setAttribute('dashboard-module', moduleName);
      if (!this._connected) this._applyTheme();
      return this.config;
    }

    setPreferences(update) {
      if (!update || typeof update !== 'object' || Array.isArray(update) || Object.keys(update).some(key => !['theme', 'minimized', 'quietMotion'].includes(key))) throw new TypeError('Invalid Spooly preferences.');
      if ('theme' in update && !['auto', ...MODULES].includes(update.theme)) throw new TypeError('Unknown Spooly appearance.');
      if (['minimized', 'quietMotion'].some(key => key in update && typeof update[key] !== 'boolean')) throw new TypeError('Spooly motion and minimize preferences must be boolean.');
      const hadFocus = !!this.shadowRoot.activeElement;
      this._prefs = { ...this._prefs, ...update };
      if (this._prefs.minimized) this._closePanels(false);
      this._savePreferences();
      this._applyTheme();
      this._renderPreferences();
      this._wake();
      if (hadFocus && 'minimized' in update) (this._prefs.minimized ? this._elements.dock : this._elements.character).focus({ preventScroll: true });
      return this.preferences;
    }

    async dispatchAIResponse(payload) {
      const response = normalizeAIResponse(payload);
      this._status = response.botStatus;
      this._emotion = response.emotion;
      this._message = response.message;
      this._pendingResponse = this._elements.speech.hidden ? response : null;
      this.setAttribute('data-emotion', response.emotion);
      // Never opens a panel, restores the mascot, or announces background data.
      this._setMessage(response.message, false);
      this._elements.status.textContent = response.botStatus === 'live_api' ? 'Host-supplied AI preview.' : 'AI is not connected.';
      if (!this._elements.speech.hidden && !this._prefs.minimized && !this.hasAttribute('suspended')) { this._wake(); this._playGesture(response.gesture); }
      return response;
    }

    _storageKey() {
      const user = this.getAttribute('user-key');
      return user && /^[A-Za-z0-9_-]{1,80}$/.test(user) ? `euro-suits:spooly:v1:${user}` : null;
    }
    _readPreferences() {
      this._prefs = defaultPreferences();
      const key = this._storageKey();
      if (!key) return;
      try {
        const saved = JSON.parse(window.localStorage.getItem(key) || 'null');
        if (!saved || typeof saved !== 'object' || Array.isArray(saved)) return;
        if (typeof saved.minimized === 'boolean') this._prefs.minimized = saved.minimized;
        if (typeof saved.quietMotion === 'boolean') this._prefs.quietMotion = saved.quietMotion;
        if (['auto', ...MODULES].includes(saved.theme)) this._prefs.theme = saved.theme;
      } catch { /* Private browsing and malformed preferences safely use defaults. */ }
    }
    _savePreferences() {
      const key = this._storageKey();
      if (!key) return;
      try { window.localStorage.setItem(key, JSON.stringify(this._prefs)); } catch { /* UI remains usable with storage disabled. */ }
    }

    _applyTheme() {
      const requested = this.getAttribute('dashboard-module');
      const automatic = MODULES.includes(requested) ? requested : resolveMascotContext('', this.getAttribute('user-role') || '');
      const nextModule = this._prefs.theme === 'auto' ? automatic : this._prefs.theme;
      const moduleChanged = !this._themeInitialized || this._module !== nextModule;
      this._module = nextModule;
      this._themeInitialized = true;
      const theme = MASCOT_THEMES[this._module];
      if (moduleChanged && !this._pendingResponse) this._emotion = theme.defaultEmotion;
      this.style.setProperty('--mascot-primary', theme.primary);
      this.style.setProperty('--mascot-secondary', theme.secondary);
      this.style.setProperty('--mascot-glow', theme.glow);
      this.setAttribute('data-module', this._module);
      this.setAttribute('data-emotion', this._emotion);
      this._elements.character.title = `${theme.label} · ${theme.tooltip}`;
      this._bounds = null;
    }
    _renderPreferences() {
      this.toggleAttribute('data-minimized', this._prefs.minimized);
      this._elements.figure.hidden = this._prefs.minimized;
      this._elements.dock.hidden = !this._prefs.minimized;
      this._elements.theme.value = this._prefs.theme;
      this._elements.quiet.checked = this._prefs.quietMotion;
      if (this._prefs.minimized) this._closePanels(false);
    }

    _click(event) {
      const action = event.target.closest?.('[data-action]')?.dataset.action;
      if (!action) return;
      if (action === 'minimize') { this.setPreferences({ minimized: true }); return; }
      if (action === 'restore') { this.setPreferences({ minimized: false }); return; }
      if (action === 'close-speech' || action === 'close-settings') { this._closePanels(true); return; }
      if (action === 'settings') {
        const open = this._elements.settings.hidden;
        this._closePanels(false);
        this._elements.settings.hidden = !open;
        this._elements.preferences.setAttribute('aria-expanded', String(open));
        if (open) this._elements.theme.focus({ preventScroll: true });
        return;
      }
      if (action === 'speak') {
        this._closePanels(false);
        this._elements.speech.hidden = false;
        this._elements.character.setAttribute('aria-expanded', 'true');
        if (this._pendingResponse) {
          const response = this._pendingResponse;
          this._pendingResponse = null;
          this._wake();
          this._setMessage(response.message, true);
          this._playGesture(response.gesture);
          return;
        }
        const messages = [MASCOT_THEMES[this._module].tooltip, 'Hard hat on, loose threads off. My AI workshop is still under construction!', 'One block, one stitch, one bright idea. Building your future assistant!', 'The workbench is busy. This is a little building preview, not live AI progress.'];
        const index = this._messageIndex++ % messages.length;
        this._status = 'under_construction';
        this._emotion = index % 2 ? 'happy' : 'thinking';
        this.setAttribute('data-emotion', this._emotion);
        this._elements.status.textContent = 'AI is not connected.';
        this._wake();
        this._setMessage(messages[index], true);
        this._playGesture(['wave', 'nod', 'celebrate', 'nod'][index]);
      }
    }
    _closePanels(restoreFocus) {
      const settingsOpen = !this._elements.settings.hidden;
      this._elements.speech.hidden = true;
      this._elements.settings.hidden = true;
      this._elements.character.setAttribute('aria-expanded', 'false');
      this._elements.preferences.setAttribute('aria-expanded', 'false');
      this._finishTyping();
      if (restoreFocus) (this._prefs.minimized ? this._elements.dock : settingsOpen ? this._elements.preferences : this._elements.character).focus({ preventScroll: true });
    }
    _setMessage(message, animate) {
      this._finishTyping();
      this._message = message;
      // The entire sentence is immediately available to assistive technology;
      // only its visual text is typed. No aria-live background interruption.
      this._elements.message.setAttribute('aria-label', message);
      if (!animate || !this._canAnimate()) { this._elements.message.textContent = message; return; }
      const characters = [...message];
      let length = 0;
      this._elements.message.textContent = '';
      const type = () => {
        length = Math.min(characters.length, length + 2);
        this._elements.message.textContent = characters.slice(0, length).join('');
        if (length < characters.length && this._canAnimate()) this._typing = window.setTimeout(type, 22);
        else { this._typing = 0; this._elements.message.textContent = message; }
      };
      type();
    }
    _finishTyping() {
      window.clearTimeout(this._typing);
      this._typing = 0;
      if (this._message) this._elements.message.textContent = this._message;
    }

    _canAnimate() { return this._connected && !document.hidden && !this._media?.matches && !this._prefs.quietMotion && !this._prefs.minimized && !this.hasAttribute('suspended') && Date.now() < this._idleUntil; }
    _wake() {
      this._idleUntil = Date.now() + 12000;
      this._syncActivity();
    }
    _syncActivity() {
      window.clearTimeout(this._idleTimer);
      this._idleTimer = 0;
      const active = this._canAnimate();
      this.toggleAttribute('data-still', !active);
      if (active) {
        this._idleTimer = window.setTimeout(() => this._syncActivity(), Math.max(1, this._idleUntil - Date.now()));
        if (!this._listening) { document.addEventListener('mousemove', this._onMove, { passive: true }); document.addEventListener('mouseleave', this._onLeave); this._listening = true; }
      } else {
        this._stopEyes();
        this._finishTyping();
        this._stopGesture();
      }
    }
    _move(event) {
      if (!this._canAnimate()) return;
      if (event.clientX < window.innerWidth - 420 || event.clientY < window.innerHeight - 380) { this._targetEyes(0, 0); return; }
      this._bounds ||= this._elements.character.getBoundingClientRect();
      const dx = event.clientX - (this._bounds.left + this._bounds.width / 2);
      const dy = event.clientY - (this._bounds.top + this._bounds.height * .5);
      this._targetEyes(Math.max(-3, Math.min(3, dx / 65)), Math.max(-2.5, Math.min(2.5, dy / 65)));
    }
    _targetEyes(x, y) {
      this._cursor.tx = x; this._cursor.ty = y;
      if (this._frame || !this._canAnimate() || (Math.abs(this._cursor.x - x) < .02 && Math.abs(this._cursor.y - y) < .02)) return;
      const tick = () => {
        if (!this._canAnimate()) { this._frame = 0; return; }
        const cursor = this._cursor;
        cursor.x += (cursor.tx - cursor.x) * .2;
        cursor.y += (cursor.ty - cursor.y) * .2;
        this._pupils.forEach(pupil => pupil.setAttribute('transform', `translate(${cursor.x.toFixed(2)} ${cursor.y.toFixed(2)})`));
        if (Math.abs(cursor.tx - cursor.x) > .02 || Math.abs(cursor.ty - cursor.y) > .02) this._frame = window.requestAnimationFrame(tick);
        else this._frame = 0;
      };
      this._frame = window.requestAnimationFrame(tick);
    }
    _stopEyes() {
      if (this._listening) { document.removeEventListener('mousemove', this._onMove); document.removeEventListener('mouseleave', this._onLeave); this._listening = false; }
      window.cancelAnimationFrame(this._frame); this._frame = 0;
      this._cursor = { x: 0, y: 0, tx: 0, ty: 0 };
      this._pupils.forEach(pupil => pupil.removeAttribute('transform'));
    }
    _playGesture(name) {
      this._stopGesture();
      if (name === 'none' || !this._canAnimate()) return;
      if (options.gesture) {
        try { this._gestureCleanup = options.gesture(this._gestureNode, name) || null; } catch { this._gestureCleanup = null; }
      } else {
        this._gestureNode.style.transformOrigin = '65px 75px';
        this._gestureNode.classList.add(`gesture-${name}`);
      }
      this._gestureTimer = window.setTimeout(() => this._stopGesture(), 900);
    }
    _stopGesture() {
      window.clearTimeout(this._gestureTimer); this._gestureTimer = 0;
      this._gestureCleanup?.(); this._gestureCleanup = null;
      this._gestureNode.classList.remove('gesture-wave', 'gesture-nod', 'gesture-celebrate');
    }
    _stopAll() {
      window.clearTimeout(this._idleTimer); this._idleTimer = 0;
      this._finishTyping(); this._stopEyes(); this._stopGesture();
      this.toggleAttribute('data-still', true);
    }
  }

  window.customElements.define(TAG, GarmentERPMascot);
  return GarmentERPMascot;
}
