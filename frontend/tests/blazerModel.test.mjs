import {test} from 'node:test';
import assert from 'node:assert/strict';
import {Vector3} from 'three';
import {buildBlazer, createFabricTextures} from '../src/lib/landing/blazerModel.ts';
import {cameraDistance, createLayerTimeline, poseLayers} from '../src/lib/landing/blazerStage.ts';

const finite = (array) => {
 for (let i = 0; i < array.length; i++) if (!Number.isFinite(array[i])) return i;
 return -1;
};

function meshesOf(group) {
 const meshes = [];
 group.traverse((node) => { if (node.isMesh) meshes.push(node); });
 return meshes;
}

test('the blazer builds every tailored part with clean geometry', () => {
 const blazer = buildBlazer();
 for (const key of ['shellLeft', 'shellRight', 'canvasLeft', 'canvasRight', 'lining', 'sleeveLeft', 'sleeveRight', 'lapelLeft', 'lapelRight', 'trimLeft', 'trimRight']) {
  assert.ok(blazer.groups[key], `missing group ${key}`);
  assert.ok(meshesOf(blazer.groups[key]).length > 0, `group ${key} carries no mesh`);
 }
 let vertices = 0, triangles = 0;
 for (const geometry of blazer.geometries) {
  const position = geometry.getAttribute('position');
  const normal = geometry.getAttribute('normal');
  assert.ok(position, 'geometry without positions');
  assert.equal(finite(position.array), -1, 'position contains NaN or Infinity');
  assert.ok(normal, 'normals were not computed');
  assert.equal(finite(normal.array), -1, 'normal contains NaN or Infinity');
  for (let i = 0; i < normal.count; i++) {
   const length = Math.hypot(normal.getX(i), normal.getY(i), normal.getZ(i));
   assert.ok(Math.abs(length - 1) < 0.02, `normal ${i} is not unit length (${length})`);
  }
  vertices += position.count;
  triangles += (geometry.getIndex() ? geometry.getIndex().count : position.count) / 3;
 }
 assert.ok(vertices > 11000, `expected a detailed garment, got ${vertices} vertices`);
 assert.ok(triangles > 18000, `expected a detailed garment, got ${triangles} triangles`);
 blazer.dispose();
});

test('the silhouette is jacket sized, mirrored and faces the camera', () => {
 const blazer = buildBlazer();
 const box = blazer.bounds;
 const size = box.getSize(new Vector3());
 assert.ok(size.y > 2.5 && size.y < 3.1, `body height out of range: ${size.y}`);
 assert.ok(size.x > 2.0 && size.x < 2.4, `sleeve span out of range: ${size.x}`);
 assert.ok(size.z > 0.75 && size.z < 1.2, `body depth out of range: ${size.z}`);
 assert.ok(Math.abs(box.getCenter(new Vector3()).x) < 0.02, 'garment is not centred');
 // Both halves must be exact mirrors, otherwise the explode animation looks torn.
 for (const [left, right] of [['shellLeft', 'shellRight'], ['sleeveLeft', 'sleeveRight'], ['lapelLeft', 'lapelRight']]) {
  assert.equal(meshesOf(blazer.groups[left]).length, meshesOf(blazer.groups[right]).length, `${left}/${right} differ`);
 }
 // Chest normals point towards the viewer, not into the torso.
 const shell = meshesOf(blazer.groups.shellRight)[0];
 const position = shell.geometry.getAttribute('position'), normal = shell.geometry.getAttribute('normal');
 let frontZ = -Infinity, frontNormalZ = 0;
 for (let i = 0; i < position.count; i++) {
  const y = position.getY(i);
  if (y > -0.1 && y < 0.3 && position.getZ(i) > frontZ && position.getX(i) < 0.25) {
   frontZ = position.getZ(i);
   frontNormalZ = normal.getZ(i);
  }
 }
 assert.ok(frontZ > 0.3, `front of the chest is too flat: z=${frontZ}`);
 assert.ok(Math.abs(frontNormalZ) > 0.6, `chest normal points sideways: ${frontNormalZ}`);
 blazer.dispose();
});

test('the woven fabric carries colour, normal and roughness maps', () => {
 const textures = createFabricTextures(64, [3, 4]);
 for (const [name, texture] of Object.entries(textures)) {
  if (name === 'dispose') continue;
  assert.equal(texture.image.width, 64, `${name} width`);
  assert.equal(texture.image.height, 64, `${name} height`);
  assert.equal(texture.repeat.x, 3, `${name} repeat x`);
  assert.equal(texture.repeat.y, 4, `${name} repeat y`);
  assert.equal(texture.generateMipmaps, true, `${name} should mip-map`);
 }
 const pixels = textures.normalMap.image.data;
 let up = 0;
 for (let i = 0; i < pixels.length; i += 4) up += pixels[i + 2];
 const averageZ = up / (pixels.length / 4);
 assert.ok(averageZ > 128 && averageZ < 250, `normal map is not mostly facing up: ${averageZ}`);
 const rough = textures.roughnessMap.image.data;
 let min = 255, max = 0;
 for (let i = 0; i < rough.length; i += 4) { min = Math.min(min, rough[i]); max = Math.max(max, rough[i]); }
 assert.ok(max - min > 20, 'roughness map has no variation, fabric would look plastic');
 textures.dispose();
});

test('disposing the blazer empties the scene graph', () => {
 const blazer = buildBlazer();
 assert.ok(blazer.geometries.length > 20);
 assert.ok(blazer.seamGeometries.length > 0, 'the cut-and-seam chapter has nothing to trace');
 blazer.dispose();
 assert.equal(blazer.root.children.length, 0);
});

test('the construction story opens the jacket, then closes it again', () => {
 const blazer = buildBlazer();
 const pose = {shell: 0, inner: 0, sleeve: 0, trim: 0, turn: -.2};
 const timeline = createLayerTimeline(pose);
 const at = (p) => { timeline.seek(Math.min(4, Math.max(0, p)) * 1000, true); poseLayers(blazer.groups, pose); };
 for (const key of Object.keys(blazer.groups)) {
  assert.equal(blazer.groups[key].position.x, 0, `${key} must start buttoned up`);
 }
 at(2);
 assert.ok(blazer.groups.shellRight.position.x > .4, 'the right front never swings open');
 assert.ok(blazer.groups.shellLeft.position.x < -.4, 'the left front never swings open');
 assert.ok(blazer.groups.lining.position.z < -.2, 'the lining never drops back');
 at(3.5);
 assert.ok(blazer.groups.sleeveRight.position.x > .5, 'the sleeves never detach for the cut chapter');
 assert.ok(blazer.groups.lapelRight.position.z > .5, 'the lapels never lift for the finish chapter');
 for (let p = 0; p <= 4; p += .25) {
  at(p);
  for (const key of Object.keys(blazer.groups)) {
   const group = blazer.groups[key];
   for (const axis of ['x', 'y', 'z']) assert.ok(Number.isFinite(group.position[axis]), `${key}.${axis} went non-finite at ${p}`);
  }
 }
 at(0);
 assert.ok(Math.abs(blazer.groups.shellRight.position.x) < 1e-6, 'the jacket does not button up again');
 assert.ok(Math.abs(pose.shell) < 1e-6, 'the shell channel does not rewind');
 blazer.dispose();
});

test('the camera frames the garment on wide and narrow viewports', () => {
 const desktop = cameraDistance(2.88, 32, 1.6, 'hero');
 assert.ok(desktop > 4 && desktop < 7, `desktop distance out of range: ${desktop}`);
 assert.ok(cameraDistance(2.88, 32, .6, 'hero') > desktop, 'narrow viewports must pull the camera back');
 assert.ok(cameraDistance(2.88, 32, 1.6, 'layers') > desktop, 'the exploded study needs more room');
 assert.equal(cameraDistance(2.88, 32, 1.6, 'hero', 2), desktop * 2, 'zoom must scale the distance');
});

test('tailored details sit where a jacket expects them', () => {
 const blazer = buildBlazer();
 const extent = (group) => {
  const box = {minX: Infinity, maxX: -Infinity, maxY: -Infinity};
  group.traverse(node => {
   if (!node.isMesh) return;
   const position = node.geometry.getAttribute('position');
   for (let i = 0; i < position.count; i++) {
    box.minX = Math.min(box.minX, position.getX(i));
    box.maxX = Math.max(box.maxX, position.getX(i));
    box.maxY = Math.max(box.maxY, position.getY(i));
   }
  });
  return box;
 };
 const sleeve = extent(blazer.groups.sleeveRight);
 assert.ok(sleeve.maxY < 1.34, `sleeve head floats above the shoulder line: ${sleeve.maxY}`);
 assert.ok(sleeve.maxX > 1.0, `sleeve does not hang clear of the body: ${sleeve.maxX}`);
 const lapel = extent(blazer.groups.lapelRight);
 assert.ok(lapel.minX > -0.06, `lapel crosses the centre front: ${lapel.minX}`);
 assert.ok(lapel.maxX < 0.7, `lapel wraps past the front edge: ${lapel.maxX}`);
 // Symmetry: the mirrored half must match exactly.
 const mirrored = extent(blazer.groups.lapelLeft);
 assert.ok(Math.abs(mirrored.maxX + lapel.minX) < 1e-6, 'lapels are not mirrored');
 blazer.dispose();
});
