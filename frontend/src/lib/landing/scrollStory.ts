/** Natural page scrolling drives the illustration; scrolling is never intercepted. */
export function observeStory(host: HTMLElement, update: (index: number, progress: number) => void) {
 const steps = Array.from(host.querySelectorAll<HTMLElement>('[data-story-step]'));
 let frame = 0;
 const read = () => {
  frame = 0;
  const anchor = window.innerHeight * (window.innerWidth <= 700 ? .85 : .53);
  const positions = steps.map(step => step.getBoundingClientRect().top + Math.min(step.offsetHeight * .35, 150));
  let index = 0;
  for (let i = 0; i < positions.length; i++) if (positions[i] <= anchor) index = i;
  const next = Math.min(index + 1, positions.length - 1);
  const between = next === index ? 0 : Math.max(0, Math.min(1, (anchor - positions[index]) / (positions[next] - positions[index])));
  update(index, index + between);
 };
 const schedule = () => { if (!frame) frame = requestAnimationFrame(read); };
 const resize = new ResizeObserver(schedule); resize.observe(host);
 window.addEventListener('scroll', schedule, {passive:true});
 window.addEventListener('resize', schedule); read();
 return () => { cancelAnimationFrame(frame); resize.disconnect(); window.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule); };
}

export function goToStoryStep(host: HTMLElement, index: number, paused: boolean) {
 const step = host.querySelectorAll<HTMLElement>('[data-story-step]')[index];
 if (!step) return;
 const anchor = window.innerHeight * (window.innerWidth <= 700 ? .85 : .53);
 const top = window.scrollY + step.getBoundingClientRect().top + Math.min(step.offsetHeight * .35, 150) - anchor + 2;
 window.scrollTo({top, behavior: paused || matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth'});
}
