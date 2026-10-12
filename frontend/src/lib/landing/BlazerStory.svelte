<script lang="ts">
 import {onMount} from 'svelte';
 import {ArrowDown,ArrowUpRight,Layers} from '@lucide/svelte';
 import GarmentScene from './GarmentScene.svelte';
 import ConstructionDetails from './ConstructionDetails.svelte';
 import {observeStory,goToStoryStep} from './scrollStory';
 let {paused=false}:{paused?:boolean}=$props();
 let host:HTMLElement;
 let active=$state(0),progress=$state(0);
 const layers=[
  {name:'The outer fabric',tag:'01 / FABRIC & IDENTITY',detail:'Every blazer begins with a fabric decision.',text:'Colour, weave, composition and weight create the first impression. A shared ETF / ESF reference keeps that decision connected to the sample and the order.',fields:['ETF / ESF reference','Composition & weave','Colour & weight'],part:'OUTER SHELL',note:'The visible character of the garment.'},
  {name:'The structure within',tag:'02 / CANVAS & INTERLINING',detail:'What you cannot see shapes what you can.',text:'Canvas and interlining support the chest and lapel. Record the required components in the sample BOM so development and production work from the same specification.',fields:['Sample BOM','Component specification','Approved construction'],part:'CANVAS / INTERLINING',note:'Support, shape and a controlled silhouette.'},
  {name:'The inner lining',tag:'03 / LINING & COMFORT',detail:'A considered finish, inside and out.',text:'The lining complements the outer fabric and covers internal construction. Keep the lining reference, colour and material requirements with the sample details.',fields:['Lining reference','Colour matching','Material requirements'],part:'INNER LINING',note:'The inside deserves the same attention.'},
  {name:'The cut & the seams',tag:'04 / PANELS & ASSEMBLY',detail:'Separate pieces. A precise fit.',text:'Sleeves, fronts and the back panel come together through patterns, cutting and sewing. Approved specifications give each team a consistent starting point.',fields:['CAD & patterns','Cut panels & bundles','Assigned sewing line'],part:'SLEEVES / BODY PANELS',note:'Accurate pieces become a complete garment.'},
  {name:'The finishing details',tag:'05 / LAPELS, POCKETS & BUTTONS',detail:'Small details complete the whole.',text:'Lapels, pockets, buttons and final pressing give the blazer its finished appearance. Quality checks connect these details to packing readiness.',fields:['Trims & button details','Finishing & pressing','Final quality checks'],part:'LAPELS / TRIMS / FINISH',note:'From individual details to a finished blazer.'}
 ];
 onMount(()=>observeStory(host,(index,value)=>{active=index;progress=value}));
</script>
<section id="blazer" class="es-blazer-story" bind:this={host} aria-labelledby="blazer-heading">
 <div class="es-story-intro"><div class="es-micro">01 / INSIDE THE GARMENT</div><h2 id="blazer-heading">A blazer.<br/><span>A world of detail.</span></h2><p>Scroll to open the layers.<br/>See how every detail connects to the work.</p></div>
 <div class="es-layer-layout">
  <div class="es-layer-sticky">
   <div class="es-layer-top"><span>EURO SUITS / CONSTRUCTION STUDY</span><Layers size={18}/></div>
   <div class="es-layer-model"><GarmentScene {paused} mode="layers" {progress}/><div class="es-layer-grid" aria-hidden="true"></div><ConstructionDetails {active} {paused}/></div><div class="es-story-progress" aria-hidden="true"><span style={`transform:scaleX(${(progress+1)/5})`}></span></div>
   <div class="es-layer-caption"><span class="es-layer-count">0{active+1}<small> / 05</small></span><div><strong>{layers[active].part}</strong><span>{layers[active].note}</span></div></div>
   <nav class="es-layer-dots" aria-label="Blazer layers">{#each layers as layer,i}<button aria-label={`Show ${layer.name.toLowerCase()}`} aria-current={active===i?'step':undefined} class:current={active===i} onclick={()=>goToStoryStep(host,i,paused)}><span></span></button>{/each}<span>SCROLL TO REVEAL <ArrowDown size={12}/></span></nav>
  </div>
  <div class="es-layer-chapters">{#each layers as layer,i}<article data-story-step class:current={active===i}><div class="es-micro">{layer.tag}</div><h3>{layer.name}<span>.</span></h3><h4>{layer.detail}</h4><p>{layer.text}</p><ul>{#each layer.fields as field}<li><ArrowUpRight size={14}/>{field}</li>{/each}</ul></article>{/each}</div>
 </div>
 <div class="es-layer-end"><span>A connected garment needs a connected workflow.</span><a href="#workflow">Follow the production journey <ArrowDown size={18}/></a></div>
</section>
