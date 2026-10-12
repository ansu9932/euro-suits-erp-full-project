<script lang="ts">
 import {drawingMotion} from './drawingMotion';
 let {stage=0,paused=false}:{stage?:number;paused?:boolean}=$props();
 const captions=['FABRIC / COLOUR / WEAVE','STYLE / QUANTITY / DELIVERY','PATTERN / FIT / MEASUREMENTS','SAMPLE / BOM / APPROVAL','MARKERS / PANELS / BUNDLES','SEAMS / ASSEMBLY / LINE','PRESSING / TAGGING / QUALITY','CARTON / PACKING / DISPATCH'];
 const traces=[
  ['M150 90H376V346','M168 123H412V380H168Z','M194 275L384 150M194 288L384 163M194 301L384 176'],
  ['M157 126H397','M218 157L185 172 167 235 190 241 207 201 204 283H280L277 201 294 241 317 235 299 172 266 157','M158 340H227M292 340H378'],
  ['M99 88V390M91 88H107M91 390H107','M193 61H368M193 53V69M368 53V69','M194 122L205 201 194 371M366 122L355 201 366 371'],
  ['M248 86L192 111 211 201 201 380 277 391','M312 86L368 111 349 201 359 380 283 391','M385 96L395 106 413 86'],
  ['M174 134L187 213 176 383','M386 134L373 213 384 383','M140 136L90 300M420 136L470 300'],
  ['M194 122L205 201 194 371','M366 122L355 201 366 371','M176 145L126 310M384 145L434 310','M211 372L267 380M293 380L349 372'],
  ['M247 86L237 164 260 180 280 254','M313 86L323 164 300 180 280 254','M221 286L259 289M301 289L339 286','M417 93L426 102 441 83'],
  ['M146 220L280 277 414 220','M146 220V354L280 414 414 354V220','M280 277V414','M326 299V326M339 294V320M350 289V316M364 283V309M378 277V303']
 ];
</script>
<div class="es-garment-diagram" class:paused style={`--stage:${stage}`} use:drawingMotion={{key:stage,paused,labels:'.es-diagram-caption > *'}}>
 <svg viewBox="0 0 560 480" fill="none" aria-hidden="true">
  <defs><pattern id="es-weave-grid" width="5" height="5" patternUnits="userSpaceOnUse"><path d="M0 0H5M0 0V5" stroke="#80adae" stroke-width=".7"/></pattern><linearGradient id="es-jacket-fill" x1="160" y1="80" x2="380" y2="410" gradientUnits="userSpaceOnUse"><stop stop-color="#355872"/><stop offset="1" stop-color="#14273c"/></linearGradient></defs>
  <g class="es-drawing-grid" stroke="#d3e2e8" stroke-width=".7">{#each [100,160,220,280,340,400,460] as x}<path d={`M${x} 35V435`}/>{/each}{#each [80,140,200,260,320,380] as y}<path d={`M55 ${y}H505`}/>{/each}<circle cx="280" cy="235" r="190" stroke-dasharray="2 7"/></g>
  <g class="diagram-fabric es-diagram-art" class:visible={stage===0}>
   <path d="M133 92H372V350H133Z" fill="#dcece8" stroke="#087f7c"/><path d="M168 123H412V380H168Z" fill="#274459"/><path d="M168 123H412V380H168Z" fill="url(#es-weave-grid)"/>
   <path d="M183 316H397V365H183Z" fill="#f5f7fa"/><text x="194" y="336">ETF / ESF</text><text x="194" y="352" class="small">FABRIC REFERENCE</text><path d="M118 105V336M110 105H126M110 336H126" stroke="#087f7c"/>
  </g>
  <g class="diagram-planning es-diagram-art" class:visible={stage===1}>
   <rect x="137" y="74" width="280" height="332" rx="8" fill="#fff" stroke="#b8cdd8"/><path d="M157 126H397M157 314H397M157 358H397" stroke="#d5e3e8"/><text x="157" y="107">BUYER / STYLE BRIEF</text><path d="M218 157L185 172 167 235 190 241 207 201 204 283H280L277 201 294 241 317 235 299 172 266 157 242 188Z" stroke="#087f7c" fill="#e8f3f0"/><path d="M219 157L228 198 242 225 256 198 266 157" stroke="#087f7c"/><text x="158" y="339">QUANTITY</text><text x="291" y="339">DELIVERY</text><path d="M326 166H384M326 185H374M326 204H380" stroke="#a3bcc8" stroke-linecap="round"/>
  </g>
  <g class="diagram-garment es-diagram-art" class:visible={stage>=2&&stage<=6} class:cut={stage===4} class:sewing={stage===5} class:finished={stage===6} class:pattern={stage===2}>
   <g class="diagram-body-left"><path d="M248 86L192 111 211 201 201 380 277 391 280 254 269 158Z" fill={stage>=5?'url(#es-jacket-fill)':'#e6f0ed'} stroke="#42687c"/><path d="M247 86L237 164 260 180 280 254 225 186 235 164Z" fill={stage>=5?'#3b6074':'#c6ded7'} stroke="#42687c"/><path d="M221 286L259 289V300L221 297Z" fill="#7badab"/></g>
   <g class="diagram-body-right"><path d="M312 86L368 111 349 201 359 380 283 391 280 254 291 158Z" fill={stage>=5?'url(#es-jacket-fill)':'#e6f0ed'} stroke="#42687c"/><path d="M313 86L323 164 300 180 280 254 335 186 325 164Z" fill={stage>=5?'#3b6074':'#c6ded7'} stroke="#42687c"/><path d="M301 289L339 286V297L301 300Z" fill="#7badab"/><circle cx="290" cy="267" r="4" fill="#c1d9d6"/><circle cx="290" cy="304" r="4" fill="#c1d9d6"/></g>
   <path class="diagram-sleeve-left" d="M192 111L168 132 113 312 155 325 200 220 211 201Z" fill={stage>=5?'url(#es-jacket-fill)':'#c6ded7'} stroke="#42687c"/>
   <path class="diagram-sleeve-right" d="M368 111L392 132 447 312 405 325 360 220 349 201Z" fill={stage>=5?'url(#es-jacket-fill)':'#c6ded7'} stroke="#42687c"/>
   <g class="diagram-seams" stroke={stage===5?'#8de0d1':'#087f7c'} stroke-width="1.6" stroke-dasharray="5 6"><path d="M194 122L205 201 194 371M366 122L355 201 366 371M176 145L126 310M384 145L434 310M211 372L267 380M293 380L349 372"/></g>
   {#if stage===2}<g stroke="#087f7c" stroke-width="1"><path d="M99 88V390M91 88H107M91 390H107M193 61H368M193 53V69M368 53V69M280 85V396" stroke-dasharray="3 5"/><text x="219" y="47">SHOULDER / FIT</text></g>{/if}
   {#if stage===3}<g><circle cx="399" cy="96" r="29" fill="#087f7c"/><path d="M385 96L395 106 413 86" stroke="white" stroke-width="3"/><text x="338" y="51">MD APPROVAL</text></g>{/if}
   {#if stage===6}<g><path d="M372 251L396 238 420 260 396 281Z" fill="#087f7c"/><circle cx="394" cy="251" r="3" fill="white"/><path d="M371 253L353 247" stroke="#087f7c"/><circle cx="429" cy="93" r="25" fill="#d9eee7"/><path d="M417 93L426 102 441 83" stroke="#087f7c" stroke-width="3"/></g>{/if}
  </g>
  <g class="diagram-packing es-diagram-art" class:visible={stage===7}><path d="M146 220L280 164 414 220 280 277Z" fill="#b2d6cc" stroke="#5c8d89"/><path d="M146 220V354L280 414V277Z" fill="#e3efeb" stroke="#5c8d89"/><path d="M280 277L414 220V354L280 414Z" fill="#c7e1d8" stroke="#5c8d89"/><path d="M173 210L203 124 246 112 280 143 314 112 357 124 387 210 280 253Z" fill="url(#es-jacket-fill)" stroke="#547587"/><path d="M246 112L263 185 280 216 297 185 314 112" stroke="#9dc7c5"/><path d="M280 277L246 261V399L280 414Z" fill="#087f7c"/><path d="M315 294L391 261V312L315 345Z" fill="white"/><path d="M326 299V326M331 297V324M339 294V320M345 291V318M350 289V316M358 286V312M364 283V309M369 281V307M378 277V303" stroke="#14273c" stroke-width="2"/></g>
  <g class="es-animated-thread" stroke={stage===5?'#61c6b4':'#087f7c'} stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
   {#each traces[stage] as line}<path data-thread d={line}/>{/each}
  </g>
  <g class="es-drawing-registration" stroke="#6b989d" stroke-width=".8"><path d="M32 53V32H53M507 32H528V53M528 427V448H507M53 448H32V427"/><path d="M270 28H290M280 18V38M270 452H290M280 442V462"/></g>
 </svg>
 <div class="es-diagram-caption"><span>0{stage+1} / GARMENT JOURNEY</span><strong>{captions[stage]}</strong></div>
</div>
