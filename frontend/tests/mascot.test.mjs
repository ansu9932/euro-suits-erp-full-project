import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {
  MASCOT_THEMES,
  MASCOT_CONFIG_SCHEMA,
  normalizeAIResponse,
  resolveMascotContext,
} from '../src/lib/mascot/garment-erp-mascot.js';

const modules = ['inventory', 'sewing', 'cutting', 'qc', 'dispatch', 'general'];
const statuses = ['idle', 'under_construction', 'live_api', 'event_trigger'];

test('configuration exposes every supported context and keeps the future endpoint bounded', () => {
  const schema = MASCOT_CONFIG_SCHEMA;
  assert.equal(schema.type, 'object');
  assert.deepEqual(schema.properties.dashboardModule.enum, modules);
  assert.deepEqual(schema.properties.botStatus.enum, statuses);
  assert.equal(schema.properties.aiApiEndpoint.type, 'string');
  assert.equal(schema.properties.aiApiEndpoint.maxLength, 2048);
  for (const color of ['primary', 'secondary', 'glow']) {
    assert.equal(schema.properties.colorTheme.properties[color].type, 'string');
  }
});

test('all dashboard themes have distinct accessories, idle behavior, and coming-soon copy', () => {
  assert.deepEqual(Object.keys(MASCOT_THEMES).sort(), [...modules].sort());
  const specifiedAccents = {
    inventory: '#10B981', sewing: '#F59E0B', cutting: '#6366F1',
    qc: '#EC4899', dispatch: '#3B82F6',
  };
  const visuals = new Set();
  const idles = new Set();
  for (const module of modules) {
    const theme = MASCOT_THEMES[module];
    for (const key of ['label', 'visualTheme', 'primary', 'secondary', 'glow', 'idle', 'tooltip']) {
      assert.equal(typeof theme[key], 'string', `${module}.${key}`);
      assert.ok(theme[key].trim(), `${module}.${key} cannot be empty`);
    }
    if (specifiedAccents[module]) assert.equal(theme.primary.toUpperCase(), specifiedAccents[module]);
    visuals.add(theme.visualTheme);
    idles.add(theme.idle);
  }
  assert.equal(visuals.size, modules.length);
  assert.equal(idles.size, modules.length);
});

const pageContexts = {
  'Materials & samples': 'inventory',
  'Inventory & stores': 'inventory',
  Purchases: 'inventory',
  Suppliers: 'inventory',
  'Sewing floor': 'sewing',
  Cutting: 'cutting',
  'Development & CAD': 'cutting',
  'Quality control': 'qc',
  'Warehouse & dispatch': 'dispatch',
  'Finishing & packing': 'dispatch',
};
for (const [page, expected] of Object.entries(pageContexts)) {
  test(`${page} determines the mascot even for a different department role`, () => {
    for (const [role, portal] of [['Super Admin', 'admin'], ['Quality', 'quality'], ['Stores', 'stores'], ['Production', 'sewing']]) {
      assert.equal(resolveMascotContext(page, role, portal), expected);
    }
  });
}

test('shared management and planning pages do not inherit an unrelated role theme', () => {
  const generalPages = ['Buyer orders', 'Enquiries & costing', 'Planning & T&A', 'Sampling',
    'User access', 'Companies', 'Reminders', 'Fields & ports', 'Teams & buyers',
    'Reports & audit', 'Approvals', 'Developer console'];
  for (const page of generalPages) {
    for (const [role, portal] of [['Stores', 'stores'], ['Quality', 'quality'], ['Production', 'sewing']]) {
      assert.equal(resolveMascotContext(page, role, portal), 'general', `${page} / ${role}`);
    }
  }
});

test('overview personalizes for every existing ERP department role', () => {
  const roleContexts = {
    'Super Admin': 'general', 'Finance Head': 'general', MD: 'general', CEO: 'general',
    Finance: 'general', Marketing: 'general', Planning: 'general', Sampling: 'general',
    Development: 'cutting', Sourcing: 'inventory', Stores: 'inventory', Cutting: 'cutting',
    Production: 'sewing', Finishing: 'dispatch', Quality: 'qc', Warehouse: 'dispatch',
  };
  for (const [role, expected] of Object.entries(roleContexts)) {
    assert.equal(resolveMascotContext('Overview', role), expected, role);
  }
});

test('portal fallback covers department URLs when a role is not supplied', () => {
  const portalContexts = {
    admin: 'general', md: 'general', finance: 'general', developer: 'general',
    management: 'general', accounts: 'general', merchandising: 'general', planning: 'general',
    sampling: 'general', cad: 'cutting', sourcing: 'inventory', stores: 'inventory',
    cutting: 'cutting', sewing: 'sewing', finishing: 'dispatch', quality: 'qc', warehouse: 'dispatch',
  };
  for (const [portal, expected] of Object.entries(portalContexts)) {
    assert.equal(resolveMascotContext('Overview', '', portal), expected, portal);
  }
  assert.equal(resolveMascotContext(), 'general');
  assert.equal(resolveMascotContext('unknown page', 'unknown role', 'unknown portal'), 'general');
});

test('the smallest valid AI response has safe presentation-only defaults', () => {
  const input = {message: 'Fabric assistant coming soon.'};
  const result = normalizeAIResponse(input);
  assert.deepEqual(result, {
    message: input.message, emotion: 'neutral', gesture: 'none', botStatus: 'event_trigger',
  });
  assert.deepEqual(input, {message: 'Fabric assistant coming soon.'});
  assert.notEqual(result, input);
});

test('valid future AI presentation states survive validation without adding transport data', () => {
  for (const emotion of ['neutral', 'happy', 'thinking', 'celebrate']) {
    for (const gesture of ['none', 'wave', 'nod', 'celebrate']) {
      for (const botStatus of statuses) {
        const payload = {message: 'A stitch in time!', emotion, gesture, botStatus};
        assert.deepEqual(normalizeAIResponse(payload), payload);
      }
    }
  }
});

test('HTML and script-looking messages remain plain text data', () => {
  const message = '<img src=x onerror="globalThis.spoolyExecuted=true"><script>fetch("/logout")</script>';
  delete globalThis.spoolyExecuted;
  assert.equal(normalizeAIResponse({message}).message, message);
  assert.equal(globalThis.spoolyExecuted, undefined);
});

test('untrusted AI payloads cannot carry executable instructions, endpoints, or extra fields', () => {
  const injectedFields = {
    command: 'delete all records', html: '<script>alert(1)</script>',
    aiApiEndpoint: 'https://example.invalid/private', url: 'javascript:alert(1)',
    permissions: ['admin'], dashboardModule: 'inventory', colorTheme: {primary: 'red'},
  };
  for (const [key, value] of Object.entries(injectedFields)) {
    assert.throws(() => normalizeAIResponse({message: 'Hello', [key]: value}), TypeError, key);
  }
  const prototypePayload = JSON.parse('{"message":"Hello","__proto__":{"polluted":true}}');
  assert.throws(() => normalizeAIResponse(prototypePayload), TypeError);
  assert.equal({}.polluted, undefined);
});

test('missing, non-object, and non-JSON-like payloads are rejected', () => {
  class ResponseLike { constructor() { this.message = 'Hello'; } }
  for (const payload of [undefined, null, true, 42, 'Hello', [], ['Hello'], new Date(), new ResponseLike(), () => ({message: 'Hello'})]) {
    assert.throws(() => normalizeAIResponse(payload), TypeError);
  }
  for (const message of [undefined, null, false, 123, {}, [], '', '   ', '\n\t']) {
    assert.throws(() => normalizeAIResponse({message}), TypeError);
  }
});

test('message limits accept the boundary and reject excessive content', () => {
  assert.equal(normalizeAIResponse({message: 'a'.repeat(280)}).message.length, 280);
  for (const length of [281, 10_000]) {
    assert.throws(() => normalizeAIResponse({message: 'a'.repeat(length)}), TypeError);
  }
});

test('invalid presentation enums and types are rejected without coercion', () => {
  for (const [key, values] of Object.entries({
    emotion: ['angry', '<script>', 1, null, {}, []],
    gesture: ['run-command', 'WAVE', true, null, {}, []],
    botStatus: ['online', 'LIVE_API', false, null, {}, []],
  })) {
    for (const value of values) {
      assert.throws(() => normalizeAIResponse({message: 'Hello', [key]: value}), TypeError, key);
    }
  }
});

test('module import and registration are safe during SSR and never contact the network', () => {
  const moduleURL = new URL('../src/lib/mascot/garment-erp-mascot.js', import.meta.url).href;
  const script = `
    import assert from 'node:assert/strict';
    const unexpected = () => { throw new Error('Unexpected network access'); };
    globalThis.fetch = unexpected;
    globalThis.WebSocket = unexpected;
    globalThis.EventSource = unexpected;
    assert.equal(typeof window, 'undefined');
    assert.equal(typeof document, 'undefined');
    const module = await import(${JSON.stringify(moduleURL)});
    assert.equal(module.registerMascot({gesture: unexpected}), undefined);
    assert.equal(typeof module.MascotEngine.setContext, 'function');
    assert.equal(typeof module.MascotEngine.dispatchAIResponse, 'function');
  `;
  const child = spawnSync(process.execPath, ['--input-type=module', '--eval', script], {
    encoding: 'utf8', timeout: 5_000,
  });
  assert.equal(child.error, undefined);
  assert.equal(child.status, 0, child.stderr || child.stdout);
});
