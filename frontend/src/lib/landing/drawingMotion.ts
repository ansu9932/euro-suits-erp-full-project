import {createTimeline, stagger, svg} from 'animejs';

type DrawingOptions = {key: number; paused: boolean; labels?: string};

// One short sequence per chapter. Off-screen and reduced-motion content stays static.
export function drawingMotion(node: HTMLElement, initial: DrawingOptions) {
 let options = initial, visible = false;
 let timeline: ReturnType<typeof createTimeline> | undefined;
 let restore: (() => void)[] = [];
 const reduce = matchMedia('(prefers-reduced-motion: reduce)');
 const reset = () => { timeline?.revert(); timeline = undefined; restore.forEach(fn => fn()); restore = []; };
 const play = () => {
  reset();
  if (!visible || options.paused || reduce.matches || document.hidden) return;
  const paths = [...node.querySelectorAll<SVGGeometryElement>('[data-thread]')];
  for (const path of paths) {
   const attrs = ['style', 'pathLength', 'draw', 'stroke-dasharray', 'stroke-dashoffset'].map(name => [name, path.getAttribute(name)] as const);
   restore.push(() => attrs.forEach(([name, value]) => value === null ? path.removeAttribute(name) : path.setAttribute(name, value)));
  }
  timeline = createTimeline({defaults: {ease: 'out(3)'}});
  if (paths.length) timeline.add(svg.createDrawable(paths), {draw: ['0 0', '0 1'], duration: 1250, delay: stagger(95)}, 0);
  const labels = node.querySelectorAll(options.labels ?? '.es-detail-label');
  if (labels.length) timeline.add(labels, {opacity: [0, 1], translateY: [8, 0], duration: 650, delay: stagger(100)}, 250);
  const artwork = node.querySelector('.es-diagram-art.visible');
  if (artwork) timeline.add(artwork, {scale: [.94, 1], translateY: [12, 0], duration: 1000, ease: 'out(4)'}, 0);
 };
 const observer = new IntersectionObserver(entries => {
  const next = entries[0].isIntersecting;
  if (visible !== next) { visible = next; play(); }
 }, {threshold: .1});
 observer.observe(node);
 reduce.addEventListener('change', play);
 document.addEventListener('visibilitychange', play);
 return {
  update(next: DrawingOptions) { const changed = next.key !== options.key || next.paused !== options.paused; options = next; if (changed) play(); },
  destroy() { reset(); observer.disconnect(); reduce.removeEventListener('change', play); document.removeEventListener('visibilitychange', play); }
 };
}
