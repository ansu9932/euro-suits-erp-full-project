<script lang="ts">
 import {drawingMotion} from './drawingMotion';
 let {active=0,paused=false}:{active:number;paused:boolean}=$props();
 const details=[
  ['LAPEL ROLL','FABRIC GRAIN','TAILORED HEM'],
  ['CHEST CANVAS','SHAPE & SUPPORT','LAPEL STRUCTURE'],
  ['LINING PANEL','CENTRE SEAM','INNER FINISH'],
  ['SLEEVE HEAD','FRONT PANELS','SEAM ALLOWANCE'],
  ['NOTCH LAPEL','POCKET WELT','BUTTON DETAIL']
 ];
</script>
<div class="es-construction-details" use:drawingMotion={{key:active,paused}} aria-hidden="true">
 <svg viewBox="0 0 600 460" preserveAspectRatio="none" fill="none">
  <path class="es-detail-frame" d="M24 63V24H63M537 24H576V63M576 397V436H537M63 436H24V397"/>
  <g class="es-detail-guides">
   <path data-thread d="M38 104H119L224 158"/><circle cx="224" cy="158" r="3"/>
   <path data-thread d="M563 217H481L370 240"/><circle cx="370" cy="240" r="3"/>
   <path data-thread d="M38 370H148L229 326"/><circle cx="229" cy="326" r="3"/>
  </g>
  <path class="es-detail-axis" d="M300 28V53M300 407V432M286 40H314M286 420H314"/>
 </svg>
 <span class="es-detail-label detail-a">{details[active][0]}</span>
 <span class="es-detail-label detail-b">{details[active][1]}</span>
 <span class="es-detail-label detail-c">{details[active][2]}</span>
 <span class="es-detail-edition">TAILORING ATELIER / 0{active+1}</span>
</div>
