<script lang="ts">
 import {onMount} from 'svelte';
 // Loose fibres drifting through the hero: cheap, canvas based, and it stops the
 // moment the page is hidden, off screen, paused or set to reduced motion.
 let {paused=false}:{paused?:boolean}=$props();
 let canvas:HTMLCanvasElement;
 onMount(()=>{
  const context=canvas.getContext('2d');
  if(!context)return;
  const reduce=matchMedia('(prefers-reduced-motion: reduce)');
  type Mote={x:number;y:number;r:number;vy:number;vx:number;a:number;len:number;hue:number};
  let motes:Mote[]=[],width=0,height=0,frame=0,last=0,t=0,visible=true;
  const seed=()=>{
   motes=Array.from({length:width<620?22:40},(_,i)=>({
    x:Math.random()*width,y:Math.random()*height,
    r:.5+Math.random()*1.6,vy:-.06-Math.random()*.16,vx:(Math.random()-.5)*.08,
    a:.12+Math.random()*.3,len:Math.random()<.22?10+Math.random()*22:0,
    hue:i%5===0?168:200
   }));
  };
  const resize=()=>{
   const box=canvas.getBoundingClientRect();
   const ratio=Math.min(devicePixelRatio||1,2);
   width=Math.max(1,Math.round(box.width));height=Math.max(1,Math.round(box.height));
   canvas.width=width*ratio;canvas.height=height*ratio;
   context.setTransform(ratio,0,0,ratio,0,0);
   seed();
  };
  const draw=(now:number)=>{
   frame=requestAnimationFrame(draw);
   if(!visible||document.hidden||paused||reduce.matches||now-last<33)return;
   const dt=Math.min((now-last)/16.7,3);last=now;t+=dt*.01;
   context.clearRect(0,0,width,height);
   for(const m of motes){
    m.x+=m.vx*dt+Math.sin(t+m.y*.01)*.12*dt;
    m.y+=m.vy*dt;
    if(m.y<-30){m.y=height+20;m.x=Math.random()*width;}
    if(m.x<-20)m.x=width+10;else if(m.x>width+20)m.x=-10;
    context.strokeStyle=`hsla(${m.hue},38%,44%,${m.a})`;
    context.lineWidth=m.r*.7;
    context.beginPath();
    if(m.len){context.moveTo(m.x,m.y);context.lineTo(m.x-m.vx*m.len,m.y-m.vy*m.len*.6);}
    else{context.moveTo(m.x,m.y);context.lineTo(m.x+.01,m.y+.01);}
    context.stroke();
   }
  };
  const observer=new IntersectionObserver(entries=>{visible=entries[0].isIntersecting;},{rootMargin:'80px'});
  observer.observe(canvas);
  const ro=new ResizeObserver(resize);ro.observe(canvas);
  resize();frame=requestAnimationFrame(draw);
  return()=>{cancelAnimationFrame(frame);observer.disconnect();ro.disconnect();};
 });
</script>
<canvas class="es-dust" bind:this={canvas} aria-hidden="true"></canvas>
<style>.es-dust{position:absolute;inset:0;width:100%;height:100%;pointer-events:none;opacity:.7}</style>
