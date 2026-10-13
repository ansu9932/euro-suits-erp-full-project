import {animate, stagger} from 'animejs';

/*
 * Landing page motion as Svelte actions. Every action is opt-in per element,
 * waits until the element is on screen, and stops completely when the visitor
 * asks for reduced motion or uses the page's own motion switch. The static DOM
 * always holds the full, readable content: motion only ever moves what is there.
 */

export type MotionOptions = {paused?: () => boolean; [key: string]: unknown};

const reduce = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
const still = (paused?: () => boolean) => reduce() || (paused ? paused() : false);

function onScreen(node: HTMLElement, enter: () => void, threshold = .2) {
 const observer = new IntersectionObserver(entries => {
  if (entries.some(entry => entry.isIntersecting)) { observer.disconnect(); enter(); }
 }, {threshold});
 observer.observe(node);
 return () => observer.disconnect();
}

/* ------------------------------------------------------------- word reveal */

/** Splits text into masked words and rises them into place, one after another. */
export function splitReveal(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial, undo: (() => void) | undefined, release: (() => void) = () => {};
 const words: HTMLElement[] = [];
 const walk = (parent: Node) => {
  for (const child of [...parent.childNodes]) {
   if (child.nodeType === Node.TEXT_NODE) {
    const text = child.textContent ?? '';
    if (!text.trim()) continue;
    const fragment = document.createDocumentFragment();
    for (const part of text.split(/(\s+)/)) {
     if (!part) continue;
     if (/^\s+$/.test(part)) { fragment.appendChild(document.createTextNode(part)); continue; }
     const outer = document.createElement('span');
     const inner = document.createElement('span');
     outer.className = 'es-word';
     inner.className = 'es-word-in';
     inner.textContent = part;
     outer.appendChild(inner);
     fragment.appendChild(outer);
     words.push(outer);
    }
    parent.replaceChild(fragment, child);
   } else if (child.nodeType === Node.ELEMENT_NODE) walk(child);
  }
 };
 const build = () => {
  if (words.length || still(options.paused)) return;
  walk(node);
  node.classList.add('es-split');
  undo = () => {
   for (const word of words) word.replaceWith(document.createTextNode(word.textContent ?? ''));
   words.length = 0;
   node.classList.remove('es-split');
   node.normalize();
   undo = undefined;
  };
  const reveal = animate(node.querySelectorAll('.es-word-in'), {
   translateY: ['108%', '0%'], opacity: [0, 1], rotate: [4, 0],
   duration: 900, delay: stagger(58), ease: 'out(4)'
  });
  release = () => reveal.revert();
 };
 const stop = () => { release(); undo?.(); release = () => {}; };
 const observer = onScreen(node, () => { if (!still(options.paused)) build(); }, .3);
 return {
  update(next: MotionOptions) {
   options = next;
   if (still(options.paused)) stop();
  },
  destroy() { stop(); observer(); }
 };
}

/* ---------------------------------------------------------------- magnetic */

/** Buttons lean toward the cursor and carry a highlight that follows it. */
export function magnetic(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial, frame = 0;
 const strength = typeof initial.strength === 'number' ? initial.strength : 9;
 const move = (event: PointerEvent) => {
  if (still(options.paused) || event.pointerType === 'touch') return;
  if (frame) return;
  frame = requestAnimationFrame(() => {
   frame = 0;
   const box = node.getBoundingClientRect();
   const x = (event.clientX - box.left) / box.width - .5;
   const y = (event.clientY - box.top) / box.height - .5;
   node.style.setProperty('--mx', `${(x * strength).toFixed(2)}px`);
   node.style.setProperty('--my', `${(y * strength * .7).toFixed(2)}px`);
   node.style.setProperty('--gx', `${((x + .5) * 100).toFixed(1)}%`);
   node.style.setProperty('--gy', `${((y + .5) * 100).toFixed(1)}%`);
  });
 };
 const leave = () => {
  node.style.setProperty('--mx', '0px');
  node.style.setProperty('--my', '0px');
 };
 node.addEventListener('pointermove', move);
 node.addEventListener('pointerleave', leave);
 return {
  update(next: MotionOptions) { options = next; if (still(options.paused)) leave(); },
  destroy() {
   if (frame) cancelAnimationFrame(frame);
   node.removeEventListener('pointermove', move);
   node.removeEventListener('pointerleave', leave);
  }
 };
}

/* -------------------------------------------------------------------- tilt */

/** Cards tilt in 3D toward the cursor. */
export function tilt(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial;
 const max = typeof initial.max === 'number' ? initial.max : 5;
 const move = (event: PointerEvent) => {
  if (still(options.paused) || event.pointerType === 'touch') return;
  const box = node.getBoundingClientRect();
  const x = (event.clientX - box.left) / box.width - .5;
  const y = (event.clientY - box.top) / box.height - .5;
  node.style.setProperty('--tx', `${(-y * max).toFixed(2)}deg`);
  node.style.setProperty('--ty', `${(x * max).toFixed(2)}deg`);
  node.style.setProperty('--gx', `${((x + .5) * 100).toFixed(1)}%`);
  node.style.setProperty('--gy', `${((y + .5) * 100).toFixed(1)}%`);
 };
 const leave = () => { node.style.setProperty('--tx', '0deg'); node.style.setProperty('--ty', '0deg'); };
 node.addEventListener('pointermove', move);
 node.addEventListener('pointerleave', leave);
 return {
  update(next: MotionOptions) { options = next; if (still(options.paused)) leave(); },
  destroy() { node.removeEventListener('pointermove', move); node.removeEventListener('pointerleave', leave); }
 };
}

/* ---------------------------------------------------------------- spotlight */

/** A soft light that follows the pointer across a section. */
export function spotlight(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial, frame = 0;
 const move = (event: PointerEvent) => {
  if (still(options.paused) || event.pointerType === 'touch') return;
  if (frame) return;
  frame = requestAnimationFrame(() => {
   frame = 0;
   const box = node.getBoundingClientRect();
   node.style.setProperty('--sx', `${event.clientX - box.left}px`);
   node.style.setProperty('--sy', `${event.clientY - box.top}px`);
  });
 };
 window.addEventListener('pointermove', move, {passive: true});
 return {
  update(next: MotionOptions) { options = next; },
  destroy() { if (frame) cancelAnimationFrame(frame); window.removeEventListener('pointermove', move); }
 };
}

/* ---------------------------------------------------------------- parallax */

/** Drifts an element against the scroll, purely from the page's own position. */
export function parallax(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial, frame = 0;
 const speed = typeof initial.speed === 'number' ? initial.speed : .12;
 const read = () => {
  frame = 0;
  if (still(options.paused)) { node.style.setProperty('--drift', '0px'); return; }
  const box = node.getBoundingClientRect();
  const progress = (box.top + box.height / 2 - innerHeight / 2) / Math.max(1, innerHeight);
  node.style.setProperty('--drift', `${(-progress * speed * 100).toFixed(2)}px`);
 };
 const schedule = () => { if (!frame) frame = requestAnimationFrame(read); };
 window.addEventListener('scroll', schedule, {passive: true});
 window.addEventListener('resize', schedule);
 read();
 return {
  update(next: MotionOptions) { options = next; schedule(); },
  destroy() { if (frame) cancelAnimationFrame(frame); window.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule); }
 };
}

/* ---------------------------------------------------------------- count up */

/** Numbers count up once, then rest on their real value. */
export function countUp(node: HTMLElement, initial: MotionOptions & {to?: number; decimals?: number}) {
 let options = initial, frame = 0, done = false;
 const to = typeof initial.to === 'number' ? initial.to : Number(node.dataset.count ?? node.textContent ?? 0);
 const decimals = typeof initial.decimals === 'number' ? initial.decimals : 0;
 const suffix = node.dataset.suffix ?? '';
 const pad = Number(node.dataset.pad ?? 0);
 const format = (value: number) => `${value.toFixed(decimals)}${suffix}`.padStart(pad, '0');
 const settle = () => { node.textContent = format(to); };
 const run = () => {
  if (done) return;
  done = true;
  if (still(options.paused)) { settle(); return; }
  const started = performance.now(), duration = 1400;
  const step = (now: number) => {
   const t = Math.min(1, (now - started) / duration);
   const eased = 1 - Math.pow(1 - t, 3);
   node.textContent = format(to * eased);
   if (t < 1 && !still(options.paused)) frame = requestAnimationFrame(step); else settle();
  };
  frame = requestAnimationFrame(step);
 };
 settle();
 const observer = onScreen(node, run, .5);
 return {
  update(next: MotionOptions & {to?: number}) { options = next; if (still(options.paused)) { if (frame) cancelAnimationFrame(frame); settle(); } },
  destroy() { if (frame) cancelAnimationFrame(frame); observer(); }
 };
}

/* ------------------------------------------------------------ thread draw */

/** Draws the hero thread work, then lets a pulse keep travelling along it. */
export function threadDraw(node: HTMLElement, initial: MotionOptions = {}) {
 let options = initial;
 const animations: ReturnType<typeof animate>[] = [];
 const paths = [...node.querySelectorAll<SVGPathElement>('path[data-draw]')];
 const lengths = paths.map(path => {
  try { return path.getTotalLength(); } catch { return 0; }
 });
 const stop = () => {
  animations.splice(0).forEach(animation => animation.revert());
  paths.forEach(path => { path.style.strokeDasharray = ''; path.style.strokeDashoffset = ''; });
  node.classList.remove('is-live');
 };
 const play = () => {
  stop();
  if (still(options.paused)) return;
  paths.forEach((path, index) => {
   const length = lengths[index];
   if (!length) return;
   path.style.strokeDasharray = `${length}`;
   path.style.strokeDashoffset = `${length}`;
   animations.push(animate(path, {
    strokeDashoffset: [length, 0], duration: 2400, delay: 200 + index * 220, ease: 'inOut(2)',
    onComplete: () => { path.style.strokeDasharray = ''; path.style.strokeDashoffset = ''; }
   }));
  });
  node.classList.add('is-live');
 };
 const observer = onScreen(node, play, .1);
 return {
  update(next: MotionOptions) { options = next; if (still(options.paused)) stop(); else play(); },
  destroy() { stop(); observer(); }
 };
}

/* ---------------------------------------------------------- scroll meter */

/** The thin reading meter pinned to the top of the page. */
export function scrollMeter(node: HTMLElement) {
 let frame = 0;
 const read = () => {
  frame = 0;
  const span = document.documentElement.scrollHeight - innerHeight;
  const ratio = span > 0 ? Math.min(1, Math.max(0, scrollY / span)) : 0;
  node.style.transform = `scaleX(${ratio.toFixed(4)})`;
 };
 const schedule = () => { if (!frame) frame = requestAnimationFrame(read); };
 window.addEventListener('scroll', schedule, {passive: true});
 window.addEventListener('resize', schedule);
 read();
 return {destroy() { if (frame) cancelAnimationFrame(frame); window.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule); }};
}
