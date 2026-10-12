<script lang="ts">
 import {onMount} from 'svelte';
 import {ArrowRight,ArrowDown,Check} from '@lucide/svelte';
 import GarmentDiagram from './GarmentDiagram.svelte';
 import {observeStory,goToStoryStep} from './scrollStory';
 type Stage={name:string;short:string;owner:string;heading:string;text:string;items:string[];next:string};
 let {stages,paused=false}:{stages:Stage[];paused?:boolean}=$props();
 let host:HTMLElement,active=$state(0),progress=$state(0);
 onMount(()=>observeStory(host,(index,value)=>{active=index;progress=value}));
</script>
<section id="workflow" class="es-flow-story es-section" bind:this={host} aria-labelledby="flow-heading">
 <div class="es-section-line"><span><i></i>03 / FOLLOW THE THREAD</span><span>FROM REQUIREMENT TO DISPATCH</span></div>
 <div class="es-flow-heading"><h2 id="flow-heading">Watch the garment.<br/><span>Follow the work.</span></h2><p>Scroll through the journey.<br/>Each detail moves with its department.</p></div>
 <div class="es-workflow-layout">
  <div class="es-workflow-sticky"><div class="es-workflow-top"><span>THE PRODUCTION JOURNEY</span><span>0{active+1} / 08</span></div><GarmentDiagram stage={active} {paused}/><div class="es-story-progress" aria-hidden="true"><span style={`transform:scaleX(${(progress+1)/8})`}></span></div><nav class="es-flow-stops" aria-label="Jump to production stage">{#each stages as stage,i}<button class:current={active===i} aria-current={active===i?'step':undefined} onclick={()=>goToStoryStep(host,i,paused)}><span>0{i+1}</span>{stage.short}</button>{/each}</nav><div class="es-workflow-hint"><ArrowDown size={14}/> SCROLL TO THE NEXT HANDOVER</div></div>
  <div class="es-workflow-chapters">{#each stages as stage,i}<article data-story-step class:current={active===i}><div class="es-workflow-number">0{i+1}<span>{stage.name}</span></div><div class="es-micro">{stage.owner}</div><h3>{stage.heading}</h3><p>{stage.text}</p><ul>{#each stage.items as item}<li><Check size={15}/>{item}</li>{/each}</ul><div class="es-handover"><ArrowRight size={18}/><span>{stage.next}</span></div></article>{/each}</div>
 </div>
</section>
