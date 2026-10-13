import * as T from 'three';
import {RoomEnvironment} from 'three/addons/environments/RoomEnvironment.js';
import {buildBlazer} from './blazerModel';
import {cameraDistance, createLayerTimeline, poseLayers, type LayerPose} from './blazerStage';

type SceneOptions = {mode: 'hero' | 'layers'; progress: () => number};

/** A soft blob shadow: cheaper and softer than a shadow map alone. */
function contactShadow() {
 const size = 256, canvas = document.createElement('canvas');
 canvas.width = canvas.height = size;
 const context = canvas.getContext('2d')!;
 const gradient = context.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
 gradient.addColorStop(0, 'rgba(19,42,58,.5)');
 gradient.addColorStop(.42, 'rgba(19,42,58,.2)');
 gradient.addColorStop(1, 'rgba(19,42,58,0)');
 context.fillStyle = gradient;
 context.fillRect(0, 0, size, size);
 const texture = new T.CanvasTexture(canvas);
 texture.colorSpace = T.SRGBColorSpace;
 return {texture, dispose: () => texture.dispose()};
}

/*
 * Studio lighting, real shadows and a turntable for the procedural blazer.
 * Scrolling never gets intercepted: the construction story is scrubbed from the
 * page's own scroll position, and dragging only turns the model.
 */
export function createGarmentScene(
 host: HTMLElement,
 paused: () => boolean,
 onReady: () => void,
 onUnavailable: () => void,
 options: SceneOptions
) {
 const renderer = new T.WebGLRenderer({alpha: true, antialias: true, powerPreference: 'high-performance'});
 const pixelRatio = () => Math.min(window.devicePixelRatio || 1, host.clientWidth < 520 ? 1.4 : 1.85);
 renderer.setPixelRatio(pixelRatio());
 renderer.setClearColor(0xffffff, 0);
 renderer.toneMapping = T.ACESFilmicToneMapping;
 renderer.toneMappingExposure = 1.12;
 renderer.shadowMap.enabled = true;
 renderer.shadowMap.type = T.PCFSoftShadowMap;
 const canvas = renderer.domElement;
 canvas.style.touchAction = 'pan-y';
 host.appendChild(canvas);

 const scene = new T.Scene();
 const camera = new T.PerspectiveCamera(32, 1, .1, 60);
 const room = new RoomEnvironment(), pmrem = new T.PMREMGenerator(renderer);
 const env = pmrem.fromScene(room, .04);
 scene.environment = env.texture;
 scene.environmentIntensity = .6;
 room.dispose();
 pmrem.dispose();

 // Three-point rig: a shadow-casting key, a cool fill and a teal rim.
 scene.add(new T.HemisphereLight(0xdff2ff, 0x2b3f4a, .5));
 const key = new T.DirectionalLight(0xffffff, 2.5);
 key.position.set(-2.6, 4.2, 4.4);
 key.castShadow = true;
 key.shadow.mapSize.set(1024, 1024);
 key.shadow.camera.near = 1;
 key.shadow.camera.far = 16;
 key.shadow.camera.left = -3.2;
 key.shadow.camera.right = 3.2;
 key.shadow.camera.top = 3.4;
 key.shadow.camera.bottom = -3.4;
 key.shadow.bias = -.0006;
 key.shadow.normalBias = .02;
 key.shadow.radius = 3;
 scene.add(key);
 const fill = new T.DirectionalLight(0xc4e6ff, .95);
 fill.position.set(3.4, .5, 3.4);
 scene.add(fill);
 const rim = new T.DirectionalLight(0x79e2cd, 2.3);
 rim.position.set(2.6, 1.8, -3.6);
 scene.add(rim);
 const bounce = new T.DirectionalLight(0xffe8cc, .3);
 bounce.position.set(0, -3, 1.2);
 scene.add(bounce);

 const blazer = buildBlazer();
 const model = new T.Group();
 model.add(blazer.root);
 scene.add(model);
 const {groups: g} = blazer;
 // Centre the garment and park the ground shadow under the hem.
 const centre = blazer.bounds.getCenter(new T.Vector3());
 const height = blazer.bounds.getSize(new T.Vector3()).y;
 model.position.y = -centre.y;
 const groundY = blazer.bounds.min.y - centre.y - .07;

 const shadow = contactShadow();
 const blob = new T.Mesh(
  new T.PlaneGeometry(3.6, 2.6),
  new T.MeshBasicMaterial({map: shadow.texture, transparent: true, opacity: .55, depthWrite: false})
 );
 blob.rotation.x = -Math.PI / 2;
 blob.position.y = groundY;
 scene.add(blob);
 const catcher = new T.Mesh(
  new T.PlaneGeometry(9, 9),
  new T.ShadowMaterial({color: 0x14313f, opacity: .16})
 );
 catcher.rotation.x = -Math.PI / 2;
 catcher.position.y = groundY;
 catcher.receiveShadow = true;
 scene.add(catcher);

 /* Turntable: drag to rotate, wheel to zoom, inertia when released. */
 let yaw = .16, pitch = 0, yawTarget = .16, pitchTarget = 0, velocity = 0;
 let zoom = 1, zoomTarget = 1, dragging = false, lastX = 0, lastY = 0, touched = -9999;
 const down = (e: PointerEvent) => {
  dragging = true;
  lastX = e.clientX;
  lastY = e.clientY;
  velocity = 0;
  canvas.setPointerCapture(e.pointerId);
  canvas.classList.add('grabbing');
 };
 const move = (e: PointerEvent) => {
  if (!dragging) return;
  const dx = e.clientX - lastX, dy = e.clientY - lastY;
  lastX = e.clientX;
  lastY = e.clientY;
  yawTarget -= dx * .009;
  pitchTarget = T.MathUtils.clamp(pitchTarget - dy * .005, -.28, .32);
  velocity = T.MathUtils.clamp(-dx * .0009, -.05, .05);
  touched = performance.now();
 };
 const up = (e: PointerEvent) => {
  if (!dragging) return;
  dragging = false;
  if (canvas.hasPointerCapture(e.pointerId)) canvas.releasePointerCapture(e.pointerId);
  canvas.classList.remove('grabbing');
 };
 const wheel = (e: WheelEvent) => {
  zoomTarget = T.MathUtils.clamp(zoomTarget * (1 + e.deltaY * .0009), .82, 1.3);
  touched = performance.now();
  e.preventDefault();
 };
 const reset = () => { yawTarget = .16; pitchTarget = 0; zoomTarget = 1; velocity = 0; };
 canvas.addEventListener('pointerdown', down);
 canvas.addEventListener('pointermove', move);
 canvas.addEventListener('pointerup', up);
 canvas.addEventListener('pointercancel', up);
 canvas.addEventListener('wheel', wheel, {passive: false});
 canvas.addEventListener('dblclick', reset);

 let inView = true, frame = 0, last = 0, t = 0, pointerX = 0, pointerY = 0;
 let dirty = true, wasMoving = false, contextLost = false, displayProgress = 0, lastProgress = -1;
 const reduce = window.matchMedia('(prefers-reduced-motion: reduce)');
 const observer = new IntersectionObserver(entries => { inView = entries[0].isIntersecting; }, {rootMargin: '120px'});
 observer.observe(host);
 const resize = () => {
  const {width, height: boxHeight} = host.getBoundingClientRect();
  const h = Math.max(1, boxHeight);
  renderer.setPixelRatio(pixelRatio());
  renderer.setSize(width, h, false);
  camera.aspect = width / h;
  camera.updateProjectionMatrix();
  dirty = true;
 };
 const ro = new ResizeObserver(resize);
 ro.observe(host);
 resize();
 const pointer = (e: PointerEvent) => {
  if (e.pointerType === 'touch' || dragging) return;
  const b = host.getBoundingClientRect();
  pointerX = T.MathUtils.clamp((e.clientX - b.left) / Math.max(1, b.width) - .5, -.6, .6);
  pointerY = T.MathUtils.clamp((e.clientY - b.top) / Math.max(1, b.height) - .5, -.6, .6);
 };
 window.addEventListener('pointermove', pointer, {passive: true});
 const lost = (e: Event) => { e.preventDefault(); contextLost = true; canvas.style.opacity = '0'; onUnavailable(); };
 const restored = () => { contextLost = false; dirty = true; canvas.style.opacity = '1'; onReady(); };
 canvas.addEventListener('webglcontextlost', lost);
 canvas.addEventListener('webglcontextrestored', restored);

 // One reversible, scroll-scrubbed timeline drives every exploded layer.
 const choreography: LayerPose = {shell: 0, inner: 0, sleeve: 0, trim: 0, turn: -.2};
 const sequence = createLayerTimeline(choreography);

 function fit() {
  camera.position.z = cameraDistance(height, camera.fov, camera.aspect, options.mode, zoom);
 }
 function pose(p: number) {
  sequence.seek(T.MathUtils.clamp(p, 0, 4) * 1000, true);
  poseLayers(g, choreography);
  blob.scale.setScalar(1 + choreography.sleeve * .18);
  blob.material.opacity = .55 - choreography.inner * .18;
 }
 function render(now: number) {
  frame = requestAnimationFrame(render);
  if (!inView || document.hidden || contextLost || now - last < 20) return;
  const dt = Math.min((now - last) / 1000, .05);
  last = now;
  const moving = !paused() && !reduce.matches;
  const target = options.mode === 'layers' ? options.progress() : 0;
  displayProgress = moving ? T.MathUtils.lerp(displayProgress, target, 1 - Math.exp(-dt * 12)) : Math.floor(target + .001);
  if (Math.abs(displayProgress - target) < .002) displayProgress = target;
  zoom = T.MathUtils.lerp(zoom, zoomTarget, 1 - Math.exp(-dt * 8));
  if (!dragging) {
   // Released drags keep turning, then the turntable takes over again.
   yawTarget += velocity;
   velocity *= .94;
   if (moving && options.mode === 'hero' && now - touched > 3500) yawTarget += dt * .12;
  }
  yaw += (yawTarget - yaw) * (1 - Math.exp(-dt * 10));
  pitch += (pitchTarget - pitch) * (1 - Math.exp(-dt * 10));
  const changed = Math.abs(lastProgress - displayProgress) > .001 || Math.abs(zoom - zoomTarget) > .001;
  if (!moving && !dirty && !wasMoving && !changed && !dragging && Math.abs(velocity) < .0004) return;
  if (moving) t += dt;
  dirty = false;
  wasMoving = moving;
  lastProgress = displayProgress;
  pose(displayProgress);
  fit();
  const idle = moving && options.mode === 'hero';
  model.rotation.y = choreography.turn + yaw + (idle ? pointerX * .12 : 0);
  model.rotation.x = pitch + (idle ? pointerY * .05 + Math.sin(t * .23) * .012 : 0);
  model.rotation.z = idle ? Math.sin(t * .21) * .014 : 0;
  model.position.y = -centre.y + (idle ? Math.sin(t * .62) * .035 : 0);
  // The teal running stitch belongs to the cut-and-seam chapter only.
  const tracing = options.mode === 'layers' && Math.floor(target) === 3 && moving;
  blazer.traceMaterial.opacity = tracing ? .85 : 0;
  if (tracing) for (let i = 0; i < blazer.seamGeometries.length; i++) {
   const start = Math.floor(((t * .22 + i * .19) % 1) * 80);
   blazer.seamGeometries[i].setDrawRange(start, 14);
  }
  key.position.x = -2.6 + (moving ? Math.sin(t * .3) * .7 : 0);
  rim.intensity = 2.3 + (moving ? Math.sin(t * .5) * .35 : 0);
  renderer.render(scene, camera);
 }
 pose(0);
 fit();
 renderer.render(scene, camera);
 onReady();
 frame = requestAnimationFrame(render);

 return () => {
  sequence.revert();
  cancelAnimationFrame(frame);
  ro.disconnect();
  observer.disconnect();
  window.removeEventListener('pointermove', pointer);
  canvas.removeEventListener('pointerdown', down);
  canvas.removeEventListener('pointermove', move);
  canvas.removeEventListener('pointerup', up);
  canvas.removeEventListener('pointercancel', up);
  canvas.removeEventListener('wheel', wheel);
  canvas.removeEventListener('dblclick', reset);
  canvas.removeEventListener('webglcontextlost', lost);
  canvas.removeEventListener('webglcontextrestored', restored);
  blazer.dispose();
  blob.geometry.dispose();
  (blob.material as T.Material).dispose();
  shadow.dispose();
  catcher.geometry.dispose();
  (catcher.material as T.Material).dispose();
  env.dispose();
  renderer.dispose();
  canvas.remove();
 };
}
