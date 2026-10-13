import * as T from 'three';

/*
 * A procedural tailored blazer: a woven wool fabric, a rounded torso loft,
 * set-in sleeves, a notched lapel, collar, pockets and trims. Everything is
 * generated from maths at runtime, so the landing page ships no binary assets.
 * This module builds geometry, materials and textures only — it never touches
 * WebGL, the DOM or window, which keeps it unit-testable in plain Node.
 *
 * Units: 1 unit ≈ 28cm. The wearer faces +Z, the wearer's left is +X.
 */

export type Vec2 = [number, number];
export type Vec3Point = [number, number, number];

const DEG = Math.PI / 180;
const clamp01 = (x: number) => (x < 0 ? 0 : x > 1 ? 1 : x);
const smooth = (x: number) => { const t = clamp01(x); return t * t * (3 - 2 * t); };
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

/** Deterministic PRNG so the fabric grain is identical on every page load. */
function noise(seed: number) {
 let a = seed >>> 0;
 return () => {
  a = (a + 0x6d2b79f5) >>> 0;
  let t = a;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
 };
}

/* ------------------------------------------------------------------ fabric */

export type FabricTextures = {
 map: T.DataTexture;
 normalMap: T.DataTexture;
 roughnessMap: T.DataTexture;
 dispose: () => void;
};

function makeTexture(size: number, pixels: Uint8Array, srgb: boolean, repeat: Vec2) {
 const texture = new T.DataTexture(pixels, size, size, T.RGBAFormat);
 texture.wrapS = texture.wrapT = T.RepeatWrapping;
 texture.repeat.set(repeat[0], repeat[1]);
 texture.magFilter = T.LinearFilter;
 texture.minFilter = T.LinearMipmapLinearFilter;
 texture.generateMipmaps = true;
 if (srgb) texture.colorSpace = T.SRGBColorSpace;
 texture.needsUpdate = true;
 return texture;
}

/**
 * A 2/2 twill wool weave. One height field drives the colour map, a
 * tangent-space normal map and a roughness map, so light rolls over real yarn
 * instead of a flat shaded surface.
 */
export function createFabricTextures(size = 256, repeat: Vec2 = [14, 16]): FabricTextures {
 const threads = 14, period = size / threads, cover = 0.84;
 const rand = noise(0x5eed1e);
 const grainRaw = new Float32Array(size * size);
 for (let i = 0; i < grainRaw.length; i++) grainRaw[i] = rand();
 // Blur the grain once: per-pixel noise would read as sensor dust, not wool.
 const grain = new Float32Array(size * size);
 const at = (x: number, y: number) => grainRaw[((y + size) % size) * size + ((x + size) % size)];
 for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
  let sum = 0;
  for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) sum += at(x + dx, y + dy);
  grain[y * size + x] = sum / 9;
 }
 const height = new Float32Array(size * size);
 const hump = (p: number) => (p < 0 || p > 1 ? 0 : 0.5 - 0.5 * Math.cos(2 * Math.PI * p));
 for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
  const fx = x / period, fy = y / period;
  const warpUp = ((Math.floor(fx) + Math.floor(fy)) % 4) < 2;
  const warp = fx % 1 < cover ? hump((fx % 1) / cover) : 0;
  const weft = fy % 1 < cover ? hump((fy % 1) / cover) : 0;
  const yarn = warpUp ? Math.max(warp * 0.92, weft * 0.3) : Math.max(weft * 0.92, warp * 0.3);
  height[y * size + x] = clamp01(yarn * 0.86 + grain[y * size + x] * 0.2);
 }
 const mapPx = new Uint8Array(size * size * 4);
 const normalPx = new Uint8Array(size * size * 4);
 const roughPx = new Uint8Array(size * size * 4);
 const strength = 2.4, wrap = (v: number) => (v + size) % size;
 for (let y = 0; y < size; y++) for (let x = 0; x < size; x++) {
  const i = y * size + x, o = i * 4, h = height[i];
  const dhx = height[y * size + wrap(x + 1)] - height[y * size + wrap(x - 1)];
  const dhy = height[wrap(y + 1) * size + x] - height[wrap(y - 1) * size + x];
  const nx = -dhx * strength, ny = -dhy * strength, nz = 1;
  const len = Math.hypot(nx, ny, nz) || 1;
  normalPx[o] = ((nx / len) * 0.5 + 0.5) * 255;
  normalPx[o + 1] = ((ny / len) * 0.5 + 0.5) * 255;
  normalPx[o + 2] = ((nz / len) * 0.5 + 0.5) * 255;
  normalPx[o + 3] = 255;
  // Yarn crests catch light; the valleys hold dye and a little lint.
  const tone = 0.84 + h * 0.16;
  mapPx[o] = tone * 252; mapPx[o + 1] = tone * 253; mapPx[o + 2] = tone * 255; mapPx[o + 3] = 255;
  const rough = clamp01(0.94 - h * 0.34 + (grain[i] - 0.5) * 0.08);
  roughPx[o] = roughPx[o + 1] = roughPx[o + 2] = rough * 255; roughPx[o + 3] = 255;
 }
 const map = makeTexture(size, mapPx, true, repeat);
 const normalMap = makeTexture(size, normalPx, false, repeat);
 const roughnessMap = makeTexture(size, roughPx, false, repeat);
 return {map, normalMap, roughnessMap, dispose: () => { map.dispose(); normalMap.dispose(); roughnessMap.dispose(); }};
}

/* --------------------------------------------------------------- silhouette */

type Row = { t: number; y: number; w: number; d: number; n: number };

// Torso block from hem (t=0) to shoulder line (t=1): half width, half depth and
// the superellipse exponent that squares the chest and rounds the waist.
const TORSO: Row[] = [
 {t: 0.00, y: -1.44, w: 0.640, d: 0.420, n: 3.1},
 {t: 0.08, y: -1.24, w: 0.605, d: 0.400, n: 3.0},
 {t: 0.24, y: -0.80, w: 0.548, d: 0.366, n: 2.7},
 {t: 0.40, y: -0.34, w: 0.520, d: 0.346, n: 2.6},
 {t: 0.56, y: 0.10, w: 0.566, d: 0.376, n: 2.9},
 {t: 0.72, y: 0.55, w: 0.630, d: 0.396, n: 3.2},
 {t: 0.86, y: 0.95, w: 0.722, d: 0.394, n: 3.5},
 {t: 0.95, y: 1.14, w: 0.792, d: 0.374, n: 3.6},
 {t: 1.00, y: 1.20, w: 0.816, d: 0.350, n: 3.6}
];

const TORSO_TOP = 1.20;
const BREAK = 0.58; // height of the top button; below it the fronts are closed
const COLLAR_SPAN = 1.16; // radians of neck the collar band covers, from centre back

function torsoAt(t: number) {
 const v = clamp01(t);
 let i = 0;
 while (i < TORSO.length - 2 && TORSO[i + 1].t < v) i++;
 const a = TORSO[i], b = TORSO[i + 1];
 const k = smooth((v - a.t) / Math.max(1e-6, b.t - a.t));
 return {y: lerp(a.y, b.y, k), w: lerp(a.w, b.w, k), d: lerp(a.d, b.d, k), n: lerp(a.n, b.n, k)};
}

function superPoint(theta: number, w: number, d: number, n: number) {
 const s = Math.sin(theta), c = Math.cos(theta), e = 2 / n;
 return [
  Math.sign(s) * Math.pow(Math.abs(s), e) * w,
  Math.sign(c) * Math.pow(Math.abs(c), e) * d
 ] as Vec2;
}

/** Shoulder line: high at the sleeve head, cut away for the neck front and back. */
function shoulderTop(theta: number) {
 const back = (1 - Math.cos(theta)) / 2;
 const neck = 1.03 - 0.09 * back;
 return neck + (1.23 - neck) * smooth(Math.abs(Math.sin(theta)));
}

/** Rim of the neck hole, a little lower at the centre back. */
function neckRim(theta: number) {
 const back = (1 - Math.cos(theta)) / 2;
 return new T.Vector3(0.205 * Math.sin(theta), 0.995 - 0.06 * back, 0.185 * Math.cos(theta) - 0.015);
}

/** Front edge, i.e. the lapel crease: overlapped below the break, open above. */
function edgeAngle(v: number) {
 if (v <= BREAK) return -3.5 * DEG;
 return lerp(-3.5 * DEG, 33 * DEG, smooth((v - BREAK) / (1 - BREAK)));
}

/** Surface point of one torso half. theta 0 is centre front, PI is centre back. */
function torsoPoint(side: number, v: number, theta: number) {
 const p = torsoAt(v);
 // The wearer's left panel lies on top where the fronts overlap.
 const overlap = side > 0 ? 0.011 * smooth(1 - Math.abs(theta) / (14 * DEG)) : 0;
 const [x, z] = superPoint(theta, p.w + overlap, p.d + overlap, p.n);
 const y = p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(v);
 return new T.Vector3(side * x, y, z);
}

/* --------------------------------------------------------------- geometry */

/** A grid surface from a point function, with UVs, indices and smooth normals. */
function buildPatch(
 point: (u: number, v: number) => T.Vector3,
 cols: number,
 rows: number,
 uvScale: Vec2 = [1, 1]
) {
 const positions: number[] = [], uvs: number[] = [], indices: number[] = [];
 for (let j = 0; j <= rows; j++) for (let i = 0; i <= cols; i++) {
  const u = i / cols, v = j / rows, p = point(u, v);
  positions.push(p.x, p.y, p.z);
  uvs.push(u * uvScale[0], v * uvScale[1]);
 }
 for (let j = 0; j < rows; j++) for (let i = 0; i < cols; i++) {
  const a = j * (cols + 1) + i, b = a + 1, c = a + cols + 1, d = c + 1;
  indices.push(a, b, c, b, d, c);
 }
 const geometry = new T.BufferGeometry();
 geometry.setAttribute('position', new T.Float32BufferAttribute(positions, 3));
 geometry.setAttribute('uv', new T.Float32BufferAttribute(uvs, 2));
 geometry.setIndex(indices);
 geometry.computeVertexNormals();
 return geometry;
}

/** A thin curved solid — pocket flaps, welts and collar leaves. */
function buildSlab(
 point: (u: number, v: number) => T.Vector3,
 thickness: number,
 cols: number,
 rows: number,
 uvScale: Vec2 = [2, 2]
) {
 const eps = 1e-3, grid: T.Vector3[][] = [], normals: T.Vector3[][] = [];
 for (let j = 0; j <= rows; j++) {
  grid[j] = []; normals[j] = [];
  for (let i = 0; i <= cols; i++) {
   const u = i / cols, v = j / rows, p = point(u, v);
   const du = point(Math.min(1, u + eps), v).sub(point(Math.max(0, u - eps), v));
   const dv = point(u, Math.min(1, v + eps)).sub(point(u, Math.max(0, v - eps)));
   const n = new T.Vector3().crossVectors(du, dv);
   if (n.lengthSq() < 1e-12) n.set(0, 0, 1); else n.normalize();
   grid[j][i] = p; normals[j][i] = n;
  }
 }
 const positions: number[] = [], uvs: number[] = [], indices: number[] = [];
 const half = thickness / 2, stride = (cols + 1) * 2;
 for (let j = 0; j <= rows; j++) for (let i = 0; i <= cols; i++) {
  const u = i / cols, v = j / rows, p = grid[j][i], n = normals[j][i];
  positions.push(p.x + n.x * half, p.y + n.y * half, p.z + n.z * half);
  uvs.push(u * uvScale[0], v * uvScale[1]);
  positions.push(p.x - n.x * half, p.y - n.y * half, p.z - n.z * half);
  uvs.push(u * uvScale[0], v * uvScale[1]);
 }
 for (let j = 0; j < rows; j++) for (let i = 0; i < cols; i++) {
  const a = j * stride + i * 2, b = a + 2, c = a + stride, d = c + 2;
  indices.push(a, b, c, b, d, c);
  indices.push(a + 1, c + 1, b + 1, b + 1, c + 1, d + 1);
 }
 // Close the rim so a flap reads as folded cloth rather than a floating sheet.
 const loop: number[] = [];
 for (let i = 0; i <= cols; i++) loop.push(i);
 for (let j = 1; j <= rows; j++) loop.push(j * (cols + 1) + cols);
 for (let i = cols - 1; i >= 0; i--) loop.push(rows * (cols + 1) + i);
 for (let j = rows - 1; j >= 1; j--) loop.push(j * (cols + 1));
 for (let k = 0; k < loop.length; k++) {
  const a = loop[k] * 2, b = loop[(k + 1) % loop.length] * 2;
  indices.push(a, b, a + 1, b, b + 1, a + 1);
 }
 const geometry = new T.BufferGeometry();
 geometry.setAttribute('position', new T.Float32BufferAttribute(positions, 3));
 geometry.setAttribute('uv', new T.Float32BufferAttribute(uvs, 2));
 geometry.setIndex(indices);
 geometry.computeVertexNormals();
 return geometry;
}

/** Set-in sleeve: a tube along a curved arm line, optionally a sub-range of it. */
function buildSleeve(
 control: Vec3Point[],
 radius: (s: number) => Vec2,
 tubular: number,
 radial: number,
 uvScale: Vec2,
 range: Vec2 = [0, 1]
) {
 const curve = new T.CatmullRomCurve3(control.map(([x, y, z]) => new T.Vector3(x, y, z)));
 const positions: number[] = [], uvs: number[] = [], indices: number[] = [];
 const reference = new T.Vector3(0, 0, 1);
 for (let j = 0; j <= tubular; j++) {
  const s = lerp(range[0], range[1], j / tubular);
  const centre = curve.getPoint(s);
  const tangent = curve.getTangent(s).normalize();
  const side = new T.Vector3().crossVectors(tangent, reference);
  if (side.lengthSq() < 1e-8) side.set(1, 0, 0); else side.normalize();
  const other = new T.Vector3().crossVectors(tangent, side).normalize();
  const [rx, rz] = radius(s);
  for (let i = 0; i <= radial; i++) {
   const a = (i / radial) * Math.PI * 2;
   const p = centre.clone()
    .addScaledVector(side, Math.cos(a) * rx)
    .addScaledVector(other, Math.sin(a) * rz);
   positions.push(p.x, p.y, p.z);
   uvs.push((i / radial) * uvScale[0], s * uvScale[1]);
  }
 }
 for (let j = 0; j < tubular; j++) for (let i = 0; i < radial; i++) {
  const a = j * (radial + 1) + i, b = a + 1, c = a + radial + 1, d = c + 1;
  indices.push(a, c, b, b, c, d);
 }
 const geometry = new T.BufferGeometry();
 geometry.setAttribute('position', new T.Float32BufferAttribute(positions, 3));
 geometry.setAttribute('uv', new T.Float32BufferAttribute(uvs, 2));
 geometry.setIndex(indices);
 geometry.computeVertexNormals();
 return geometry;
}

/* -------------------------------------------------------------------- model */

export type BlazerGroups = Record<
 'shellLeft' | 'shellRight' | 'canvasLeft' | 'canvasRight' | 'lining' |
 'sleeveLeft' | 'sleeveRight' | 'lapelLeft' | 'lapelRight' | 'trimLeft' | 'trimRight',
 T.Group
>;

export type BlazerModel = {
 root: T.Group;
 groups: BlazerGroups;
 seamGeometries: T.BufferGeometry[];
 traceMaterial: T.LineBasicMaterial;
 materials: T.Material[];
 geometries: T.BufferGeometry[];
 textures: FabricTextures;
 bounds: T.Box3;
 dispose: () => void;
};

export type BlazerOptions = {
 shell?: number;
 lapel?: number;
 lining?: number;
 canvas?: number;
 trim?: number;
 accent?: number;
 fabricRepeat?: Vec2;
};

export function buildBlazer(options: BlazerOptions = {}): BlazerModel {
 const textures = createFabricTextures(256, options.fabricRepeat ?? [14, 16]);
 const geometries: T.BufferGeometry[] = [], lineGeometries: T.BufferGeometry[] = [], seamGeometries: T.BufferGeometry[] = [];
 const materials: T.Material[] = [];
 const fabric = new T.MeshPhysicalMaterial({
  color: options.shell ?? 0x16304a,
  map: textures.map,
  normalMap: textures.normalMap,
  normalScale: new T.Vector2(0.6, 0.6),
  roughnessMap: textures.roughnessMap,
  roughness: 0.95,
  metalness: 0.02,
  sheen: 0.55,
  sheenRoughness: 0.72,
  sheenColor: new T.Color(0x8fc4d2),
  side: T.DoubleSide
 });
 const lapelMaterial = new T.MeshPhysicalMaterial({
  color: options.lapel ?? 0x1b3c58,
  map: textures.map,
  normalMap: textures.normalMap,
  normalScale: new T.Vector2(0.75, 0.75),
  roughnessMap: textures.roughnessMap,
  roughness: 0.86,
  metalness: 0.03,
  sheen: 0.7,
  sheenRoughness: 0.6,
  sheenColor: new T.Color(0xa6d6e0),
  side: T.DoubleSide
 });
 const feltMaterial = new T.MeshPhysicalMaterial({
  color: 0x101d29, roughness: 0.95, metalness: 0, sheen: 0.35,
  sheenColor: new T.Color(0x4d6d78), side: T.DoubleSide
 });
 const satin = textures.normalMap.clone();
 satin.repeat.set(26, 30);
 satin.needsUpdate = true;
 const liningMaterial = new T.MeshPhysicalMaterial({
  color: options.lining ?? 0x0f7f7c,
  normalMap: satin,
  normalScale: new T.Vector2(0.22, 0.22),
  roughness: 0.3,
  metalness: 0.12,
  clearcoat: 0.45,
  clearcoatRoughness: 0.35,
  sheen: 0.6,
  sheenColor: new T.Color(0x9fe8dd),
  side: T.DoubleSide
 });
 const canvasMaterial = new T.MeshStandardMaterial({
  color: options.canvas ?? 0xcfc4a6, roughness: 0.95, metalness: 0, side: T.DoubleSide
 });
 const trimMaterial = new T.MeshPhysicalMaterial({
  color: options.trim ?? 0x14273c, roughness: 0.55, metalness: 0.08,
  clearcoat: 0.4, clearcoatRoughness: 0.5, side: T.DoubleSide
 });
 const buttonMaterial = new T.MeshPhysicalMaterial({
  color: 0x2a3a45, roughness: 0.32, metalness: 0.18, clearcoat: 0.7, clearcoatRoughness: 0.25
 });
 const accentMaterial = new T.MeshPhysicalMaterial({
  color: options.accent ?? 0x74d3c2, roughness: 0.45, metalness: 0.05,
  sheen: 0.8, sheenColor: new T.Color(0xd7fff6), side: T.DoubleSide
 });
 const stitch = new T.LineDashedMaterial({color: 0x9fc9cf, dashSize: 0.022, gapSize: 0.026, transparent: true, opacity: 0.45});
 const trace = new T.LineBasicMaterial({color: 0x5bd9c1, transparent: true, opacity: 0});
 materials.push(fabric, lapelMaterial, feltMaterial, liningMaterial, canvasMaterial, trimMaterial, buttonMaterial, accentMaterial, stitch, trace);

 const root = new T.Group();
 const groups = {} as BlazerGroups;
 for (const key of ['shellLeft', 'shellRight', 'canvasLeft', 'canvasRight', 'lining', 'sleeveLeft', 'sleeveRight', 'lapelLeft', 'lapelRight', 'trimLeft', 'trimRight'] as const) {
  groups[key] = new T.Group();
  groups[key].name = key;
  root.add(groups[key]);
 }
 const add = (parent: T.Group, geometry: T.BufferGeometry, material: T.Material) => {
  geometries.push(geometry);
  const mesh = new T.Mesh(geometry, material);
  mesh.castShadow = true;
  mesh.receiveShadow = true;
  parent.add(mesh);
  return mesh;
 };
 const seam = (parent: T.Group, points: T.Vector3[], traced = false) => {
  const geometry = new T.BufferGeometry().setFromPoints(points);
  const line = new T.Line(geometry, stitch);
  line.computeLineDistances();
  lineGeometries.push(geometry);
  parent.add(line);
  if (traced) {
   const clone = geometry.clone();
   lineGeometries.push(clone);
   seamGeometries.push(clone);
   parent.add(new T.Line(clone, trace));
  }
 };

 /* One mirrored half per side, so the scroll story can open the jacket. */
 for (const side of [1, -1] as const) {
  const shell = side > 0 ? groups.shellRight : groups.shellLeft;
  const sleeve = side > 0 ? groups.sleeveRight : groups.sleeveLeft;
  const lapel = side > 0 ? groups.lapelRight : groups.lapelLeft;
  const trim = side > 0 ? groups.trimRight : groups.trimLeft;
  const canvas = side > 0 ? groups.canvasRight : groups.canvasLeft;

  add(shell, buildPatch((u, v) => torsoPoint(side, v, lerp(edgeAngle(v), Math.PI, u)), 46, 34, [15, 17]), fabric);
  // Shoulder yoke: the top surface between the neck hole and the shoulder line.
  add(shell, buildPatch(
   (u, v) => {
    const theta = lerp(edgeAngle(1), Math.PI, u);
    const rim = neckRim(theta);
    const edge = torsoPoint(side, 1, theta);
    const k = smooth(v);
    return new T.Vector3(
     side * lerp(rim.x, Math.abs(edge.x), k),
     lerp(rim.y, edge.y, k) + 0.05 * Math.sin(Math.PI * v),
     lerp(rim.z, edge.z, k)
    );
   },
   26, 8, [10, 3]
  ), fabric);

  /* Front lining, revealed when the shell swings open. */
  add(groups.lining, buildPatch(
   (u, v) => {
    const theta = lerp(6 * DEG, Math.PI, u);
    const p = torsoAt(v);
    const [x, z] = superPoint(theta, p.w - 0.035, p.d - 0.03, p.n);
    return new T.Vector3(side * x, p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(v) * 0.97, z);
   },
   30, 22, [12, 14]
  ), liningMaterial);

  /* Chest canvas: the structure that sits between shell and lining. */
  add(canvas, buildPatch(
   (u, v) => {
    const theta = lerp(8 * DEG, 62 * DEG, u);
    const t = lerp(0.34, 0.98, v);
    const p = torsoAt(t);
    const [x, z] = superPoint(theta, p.w - 0.02, p.d - 0.016, p.n);
    return new T.Vector3(side * x, p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(t) * 0.97, z);
   },
   14, 12, [4, 4]
  ), canvasMaterial);

  /* Sleeve: curved arm line, eased sleeve head, working cuff with buttons. */
  const arm: Vec3Point[] = [
   [side * 0.58, 0.94, -0.04], [side * 0.76, 1.00, 0.00], [side * 0.84, 0.70, 0.035],
   [side * 0.88, 0.30, 0.06], [side * 0.91, -0.16, 0.05], [side * 0.93, -0.63, 0.01],
   [side * 0.94, -1.05, -0.03]
  ];
  const armRadius = (s: number): Vec2 => {
   const r = lerp(0.268, 0.135, smooth(s)) * (1 + 0.07 * Math.exp(-Math.pow(s / 0.09, 2)));
   return [r, r * 0.9];
  };
  add(sleeve, buildSleeve(arm, armRadius, 40, 26, [11, 15]), fabric);
  add(sleeve, buildSleeve(
   arm.map(([x, y, z]) => [x * 1.04, y, z] as Vec3Point),
   (s) => [armRadius(s)[0] * 1.05, armRadius(s)[1] * 1.06],
   12, 22, [9, 1], [0.87, 1]
  ), trimMaterial);
  for (let i = 0; i < 4; i++) {
   const geometry = new T.SphereGeometry(0.023, 14, 10);
   geometries.push(geometry);
   const button = new T.Mesh(geometry, buttonMaterial);
   button.scale.z = 0.45;
   button.position.set(side * (1.055 + i * 0.004), -0.86 - i * 0.062, 0.075 + i * 0.004);
   button.castShadow = true;
   sleeve.add(button);
  }

  /* Notched lapel, folded from the crease in over the chest. */
  const lapelPoint = (u: number, v: number) => {
   const notch = 0.3 * Math.max(0, 1 - Math.abs(u - 0.18) / 0.15) * smooth((v - 0.7) / 0.3);
   const t = lerp(BREAK - 0.02, 1, v * (1 - notch));
   // The crease follows the front edge of the shell; the lapel edge stays inside
   // it, so both lapels meet at the break point without crossing each other.
   const crease = Math.max(0, edgeAngle(t));
   const width = (Math.min(crease, 21 * DEG * smooth(clamp01(v * 1.1))) + 0.5 * DEG) * u;
   const theta = crease - width;
   const p = torsoAt(t);
   const roll = 0.018 + 0.055 * Math.sin(Math.PI * clamp01(0.12 + u * 0.85)) * (0.3 + 0.7 * smooth(v));
   const [x, z] = superPoint(theta, p.w + roll, p.d + roll * 0.85, p.n);
   return new T.Vector3(side * x, p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(t), z);
  };
  add(lapel, buildPatch(lapelPoint, 16, 28, [5, 9]), lapelMaterial);
  // Under-collar leaf, then the collar band that wraps the back of the neck.
  add(lapel, buildPatch(
   (u, v) => {
    const theta = Math.PI - lerp(0.35, COLLAR_SPAN, u);
    const rim = neckRim(theta);
    const k = smooth(v);
    return new T.Vector3(side * rim.x * (1 + 0.4 * k), rim.y + 0.2 * k - 0.02, rim.z * (1 + 0.4 * k));
   },
   12, 5, [4, 2]
  ), feltMaterial);
  add(trim, buildPatch(
   (u, v) => {
    const theta = Math.PI - lerp(0, COLLAR_SPAN, u);
    const rim = neckRim(theta);
    const k = smooth(v);
    return new T.Vector3(side * rim.x * (1 + 0.52 * k), rim.y + 0.25 * k, rim.z * (1 + 0.52 * k) - 0.012 * k);
   },
   22, 6, [8, 2]
  ), lapelMaterial);
  add(trim, buildPatch(
   (u, v) => {
    const theta = Math.PI - lerp(0, COLLAR_SPAN, u);
    const rim = neckRim(theta);
    const k = smooth(v);
    return new T.Vector3(side * rim.x * (1 + 0.26 * k), rim.y + 0.11 * k - 0.025, rim.z * (1 + 0.26 * k));
   },
   22, 4, [8, 1]
  ), feltMaterial);

  /* Pockets: a lower flap and a breast welt, both following the body curve. */
  add(trim, buildSlab(
   (u, v) => {
    const theta = lerp(20 * DEG, 52 * DEG, u);
    const t = lerp(0.33, 0.26, v);
    const p = torsoAt(t);
    const lift = 0.024 + 0.007 * Math.sin(Math.PI * v);
    const [x, z] = superPoint(theta, p.w + lift, p.d + lift, p.n);
    return new T.Vector3(side * x, p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(t), z);
   },
   0.036, 16, 4, [3, 1]
  ), fabric);
  add(trim, buildSlab(
   (u, v) => {
    const theta = lerp(26 * DEG, 44 * DEG, u);
    const t = lerp(0.66, 0.63, v);
    const p = torsoAt(t);
    const [x, z] = superPoint(theta, p.w + 0.02, p.d + 0.017, p.n);
    return new T.Vector3(side * x, p.y + (shoulderTop(theta) - TORSO_TOP) * smooth(t), z);
   },
   0.026, 12, 2, [3, 1]
  ), trimMaterial);
  if (side > 0) {
   // Pocket square in the brand accent, folded in the breast welt.
   add(trim, buildPatch(
    (u, v) => {
     const theta = lerp(29 * DEG, 41 * DEG, u);
     const p = torsoAt(0.66);
     const lift = 0.04 + 0.075 * Math.sin(Math.PI * u) * (1 - v * 0.5);
     const [x, z] = superPoint(theta, p.w + lift, p.d + lift * 0.8, p.n);
     return new T.Vector3(x, p.y + 0.035 + v * 0.1, z);
    },
    10, 4, [2, 1]
   ), accentMaterial);
   // Front closure buttons on the wearer's left panel.
   for (const [y, z] of [[0.02, 0.395], [-0.3, 0.372]] as Vec2[]) {
    const geometry = new T.SphereGeometry(0.05, 20, 14);
    geometries.push(geometry);
    const button = new T.Mesh(geometry, buttonMaterial);
    button.scale.z = 0.42;
    button.position.set(0.06, y, z);
    button.castShadow = true;
    trim.add(button);
   }
  }

  /* Hand-finished AMF stitching along the lapel roll, front edge and armhole. */
  const lapelEdge: T.Vector3[] = [], frontEdge: T.Vector3[] = [], armhole: T.Vector3[] = [];
  for (let i = 0; i <= 40; i++) {
   const v = i / 40;
   lapelEdge.push(lapelPoint(0.985, v));
   frontEdge.push(torsoPoint(side, v, edgeAngle(v)));
  }
  for (let i = 0; i <= 24; i++) armhole.push(torsoPoint(side, 1, lerp(0.3, Math.PI - 0.3, i / 24)));
  seam(lapel, lapelEdge, true);
  seam(shell, frontEdge, true);
  seam(shell, armhole);
 }

 /* Centre-back seam of the lining. */
 const backSeamGeometry = new T.BufferGeometry().setFromPoints([
  new T.Vector3(0, 1.02, -0.32), new T.Vector3(0, -1.36, -0.39)
 ]);
 const backSeam = new T.Line(backSeamGeometry, stitch);
 backSeam.computeLineDistances();
 lineGeometries.push(backSeamGeometry);
 groups.lining.add(backSeam);

 const bounds = new T.Box3().setFromObject(root);
 const dispose = () => {
  geometries.forEach(geometry => geometry.dispose());
  lineGeometries.forEach(geometry => geometry.dispose());
  materials.forEach(material => material.dispose());
  textures.dispose();
  root.clear();
 };
 return {root, groups, seamGeometries, traceMaterial: trace, materials, geometries, textures, bounds, dispose};
}
