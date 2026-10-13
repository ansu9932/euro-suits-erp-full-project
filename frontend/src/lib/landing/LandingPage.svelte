<script lang="ts">
 import {onMount} from 'svelte';
 import {animate,stagger} from 'animejs';
 import {ArrowUpRight,ArrowRight,ArrowDown,Pause,Play,Menu,X,Layers,Check,Scissors,Package,Shirt,Workflow,LockKeyhole} from '@lucide/svelte';
 import '@fontsource/manrope/400.css';import '@fontsource/manrope/500.css';import '@fontsource/manrope/600.css';import '@fontsource/manrope/700.css';import '@fontsource/manrope/800.css';
 import GarmentScene from './GarmentScene.svelte';
 import DustField from './DustField.svelte';
 import BlazerStory from './BlazerStory.svelte';
 import WorkflowStory from './WorkflowStory.svelte';
 import AIAnnouncement from './AIAnnouncement.svelte';
 import {splitReveal,magnetic,tilt,spotlight,parallax,countUp,threadDraw,scrollMeter} from './pageMotion';
 import './landing.css';
 import './story.css';
 import './motion.css';
 import './pageMotion.css';
 let root:HTMLElement,paused=$state(false),menu=$state(false);
 const stages=[
  {name:'Company & sourcing',short:'Source',owner:'FINANCE · SOURCING',heading:'Start with the right foundation.',text:'Register companies, buyers and suppliers. Build a fabric library with ETF / ESF references, specifications and swatch photographs.',items:['Company review & approval','Supplier and material masters','Fabric references & sample photos'],next:'A shared, approved foundation for the teams that follow.',icon:Layers},
  {name:'Orders & planning',short:'Plan',owner:'MERCHANDISING · PLANNING',heading:'Turn the order into a plan.',text:'Bring buyer requirements, order quantities and delivery dates together. Plan capacity and follow time-and-action milestones.',items:['Buyer orders & style information','Capacity and line allocation','Time & action milestones'],next:'Clear requirements move into development.',icon:Workflow},
  {name:'Development & CAD',short:'Develop',owner:'PRODUCT DEVELOPMENT · CAD',heading:'Give every style a clear direction.',text:'Connect style details, patterns and technical documents so the next team works from the same reference.',items:['Style and pattern development','Specifications & technical files','Development handover'],next:'The design is ready for sampling.',icon:Layers},
  {name:'Sample & approval',short:'Sample',owner:'MERCHANDISING · MD · SAMPLING',heading:'Approve the details before production.',text:'Create a sample indent with the required sizes, colours and components. MD approval sends it to Sampling, with one indent number per piece.',items:['Sample requests & optional BOM','MD approval before sampling','Piece-wise indents & handover'],next:'Approved samples guide the production team.',icon:Shirt},
  {name:'Cutting',short:'Cut',owner:'CUTTING TEAM',heading:'Precision begins at the cutting table.',text:'Organise fabric laying, markers, checking and bundles. Keep cutting progress connected to the order it belongs to.',items:['Table and marker allocation','Fabric checks & bundling','Order-wise cutting progress'],next:'Prepared bundles move to assigned sewing lines.',icon:Scissors},
  {name:'Sewing',short:'Sew',owner:'PRODUCTION MANAGERS · LINE TEAMS',heading:'The right work. The right line.',text:'Track production on assigned sewing lines. Managers and supervisors work within the lines, teams and orders they are authorised to manage.',items:['Line-based work allocation','Daily production entries','Pending quantities & targets'],next:'Completed garments move into finishing.',icon:Shirt},
  {name:'Finishing & quality',short:'Finish',owner:'FINISHING · QUALITY',heading:'Bring every piece to its final standard.',text:'Follow product-specific finishing for suits, trousers and shirts, including checking, pressing, tagging and packing preparation.',items:['Product-specific finishing','Quality checks & pressing','Tagging and packing readiness'],next:'Checked garments are ready to pack.',icon:Check},
  {name:'Packing & dispatch',short:'Deliver',owner:'PACKING · WAREHOUSE · ACCOUNTS',heading:'Close the loop, from carton to dispatch.',text:'Connect packed goods, warehouse movement and the invoicing queue. Keep the final handover visible against the original order.',items:['Packing and warehouse records','Invoicing & logistics handover','Order dispatch tracking'],next:'A traceable journey, from requirement to dispatch.',icon:Package}
 ];
 const departments=[
  {id:'md',name:'Leadership',text:'Approvals and operational oversight',roles:['MD','Management'],links:['md','management']},
  {id:'finance',name:'Finance & administration',text:'Company records, users and accounts',roles:['Finance','Accounts','Admin'],links:['finance','accounts','admin']},
  {id:'merchandising',name:'Commercial & planning',text:'Buyer requirements and production plans',roles:['Merchandising','Planning'],links:['merchandising','planning']},
  {id:'sourcing',name:'Materials & development',text:'Fabric, inventory, patterns and samples',roles:['Sourcing','Stores','CAD','Sampling'],links:['sourcing','stores','cad','sampling']},
  {id:'sewing',name:'Production',text:'From cut panels to finished garments',roles:['Cutting','Sewing','Finishing'],links:['cutting','sewing','finishing']},
  {id:'quality',name:'Quality & dispatch',text:'Final checks and warehouse handover',roles:['Quality','Warehouse'],links:['quality','warehouse']}
 ];
 const metrics=[{value:8,label:'CONNECTED STAGES'},{value:6,label:'DEPARTMENT WORKSPACES'},{value:5,label:'GARMENT LAYERS EXPLORED'},{value:3,label:'PRODUCT LINES COVERED'}];
 const ticker=['SUITS','TROUSERS','SHIRTS','FABRIC LIBRARY','SAMPLE APPROVALS','LINE PLANNING','QUALITY CHECKS','PACKING & DISPATCH'];
 const orbital=[{label:'MERCHANDISING',sub:'Buyer requirements',x:60,y:7},{label:'PLANNING',sub:'Capacity & milestones',x:86,y:27},{label:'PRODUCTION',sub:'Lines & progress',x:88,y:69},{label:'QUALITY',sub:'Checks & finishing',x:52,y:90},{label:'SOURCING',sub:'Fabric & suppliers',x:13,y:77},{label:'SAMPLING',sub:'Indents & approval',x:9,y:28}];
 onMount(()=>{
  const reduce=matchMedia('(prefers-reduced-motion: reduce)');paused=reduce.matches;
  const animations:ReturnType<typeof animate>[]=[];
  if(!reduce.matches)animations.push(animate(root.querySelectorAll('.hero-reveal'),{opacity:[0,1],translateY:[20,0],delay:stagger(90),duration:750,ease:'out(3)'}));
  const observer=new IntersectionObserver(entries=>{for(const e of entries)if(e.isIntersecting){if(!reduce.matches&&!paused)animations.push(animate(e.target,{opacity:[.25,1],translateY:[24,0],duration:700,ease:'out(3)'}));observer.unobserve(e.target)}},{threshold:.12});
  root.querySelectorAll('[data-reveal]').forEach(el=>observer.observe(el));
  const changed=()=>{paused=reduce.matches;if(reduce.matches)animations.forEach(a=>a.revert())};reduce.addEventListener('change',changed);
  return()=>{observer.disconnect();animations.forEach(a=>a.revert());reduce.removeEventListener('change',changed)};
 });
</script>
<svelte:head>
 <title>Euro Suits · Every thread. One connected workflow.</title>
 <meta name="description" content="Explore the Euro Suits manufacturing ERP: connected company setup, fabric sourcing, sample approvals, planning, production and dispatch. Sign in to your department workspace."/>
 <meta name="theme-color" content="#14273c"/>
</svelte:head>
<div class="es-site" bind:this={root} class:motion-paused={paused}>
 <a class="es-skip" href="#main">Skip to content</a>
 <AIAnnouncement {paused}/>
 <header class="es-nav">
  <a class="es-brand" href="/" aria-label="Euro Suits home"><span class="es-logo"><img src="/logo.png" alt="" width="42" height="42"/></span><span>EURO SUITS<span class="brand-caption">MANUFACTURING ERP</span></span></a>
  <nav class:open={menu} aria-label="Main navigation"><a href="#blazer" onclick={()=>menu=false}>Inside the blazer</a><a href="#about" onclick={()=>menu=false}>The company</a><a href="#workflow" onclick={()=>menu=false}>How it works</a><a href="#workspaces" onclick={()=>menu=false}>Workspaces</a></nav>
  <a class="es-nav-login" href="/signin">Sign in <ArrowUpRight size={16}/></a>
  <button class="es-menu" aria-label={menu?'Close navigation':'Open navigation'} aria-expanded={menu} onclick={()=>menu=!menu}>{#if menu}<X size={22}/>{:else}<Menu size={22}/>{/if}</button>
 </header>
 <main id="main">
  <section class="es-hero" aria-labelledby="hero-title" use:spotlight={{paused:()=>paused}}>
   <div class="es-threads" aria-hidden="true" use:threadDraw={{paused:()=>paused}}><svg viewBox="0 0 1600 800" preserveAspectRatio="none"><path data-draw d="M-100 155C400 30 540 440 1010 180S1490 160 1700 90"/><path data-draw d="M-100 240C460 185 450 350 940 270S1400 -30 1700 195"/><path data-draw d="M-100 630C260 720 450 590 920 620S1360 300 1700 580"/><path data-draw d="M-100 740C450 500 680 880 1110 710S1430 530 1700 690"/><path class="es-thread-pulse" d="M-100 155C400 30 540 440 1010 180S1490 160 1700 90"/><path class="es-thread-pulse" d="M-100 240C460 185 450 350 940 270S1400 -30 1700 195"/><path class="es-thread-pulse" d="M-100 740C450 500 680 880 1110 710S1430 530 1700 690"/></svg></div>
   <DustField {paused}/>
   <div class="es-hero-copy">
    <div class="es-kicker hero-reveal"><span class="es-dot"></span> PRECISION IN EVERY PROCESS</div>
    <h1 id="hero-title" use:splitReveal={{paused:()=>paused}}>Every thread.<br/><span>One connected</span><br/>workflow.</h1>
    <p class="es-lead hero-reveal">From the first fabric swatch to the final dispatch. Bring people, production and progress together with Euro Suits ERP.</p>
    <div class="es-actions hero-reveal"><a class="es-btn es-primary" href="/signin" use:magnetic={{paused:()=>paused}}>Enter your workspace <ArrowUpRight size={18}/></a><a class="es-btn es-secondary" href="#blazer" use:magnetic={{paused:()=>paused}}>Explore the blazer <ArrowDown size={17}/></a></div>
    <div class="es-micro hero-reveal">DESIGNED FOR GARMENTS. BUILT AROUND PEOPLE.</div>
   </div>
   <div class="es-sculpture" use:parallax={{paused:()=>paused,speed:.07}}>
    <GarmentScene {paused}/>
    {#each orbital as o,i}<div class="es-orb-label" style={`--x:${o.x}%;--y:${o.y}%;--i:${i}`}><span class="es-orb-dot"></span><div><span class="es-orb-num">0{i+1}</span><strong>{o.label}</strong><span>{o.sub}</span></div></div>{/each}
    <div class="es-model-caption">FIG. 01 / THE CONNECTED GARMENT</div>
   </div>
   <div class="es-hero-foot"><span>SUITS · TROUSERS · SHIRTS</span><a href="#about">SCROLL TO DISCOVER <ArrowDown size={14}/></a><button class="es-motion" onclick={()=>paused=!paused} aria-pressed={paused}>{#if paused}<Play size={13}/> Enable motion{:else}<Pause size={13}/> Pause motion{/if}</button></div>
  </section>
  <div class="es-metrics" data-reveal>{#each metrics as m}<div><strong use:countUp={{paused:()=>paused,to:m.value}} data-pad="2">00</strong><span>{m.label}</span></div>{/each}</div>
  <div class="es-ticker" aria-hidden="true"><div class="es-ticker-track">{#each [0,1] as copy}<span data-copy={copy}>{#each ticker as item}<em>{item}</em><i>&#9670;</i>{/each}</span>{/each}</div></div>
  <BlazerStory {paused}/>
  <section id="about" class="es-about es-section" data-reveal>
   <div class="es-section-line"><span><i></i>02 / THE EURO SUITS WAY</span><span>CRAFT MEETS COORDINATION</span></div>
   <div class="es-about-grid"><h2 use:splitReveal={{paused:()=>paused}}>Good garments.<br/><span>Great coordination.</span></h2><div><p class="es-intro">A garment passes through many hands.<br/>Its information should move just as smoothly.</p><p>Euro Suits brings sourcing, merchandising, development, production and dispatch into a shared manufacturing workspace. Every department has its part. Every handover has a place.</p><a class="es-text-link" href="#workspaces">Meet the workspaces <ArrowUpRight size={18}/></a></div></div>
   <div class="es-principles"><article><span>01 — CLARITY</span><h3>Know what comes next.</h3><p>Follow requirements, approvals and the next department in the workflow.</p></article><article><span>02 — ACCOUNTABILITY</span><h3>Give each team its space.</h3><p>Work with the companies, orders and sewing lines assigned to your role.</p></article><article><span>03 — CONTINUITY</span><h3>Keep the details together.</h3><p>Connect fabric references, sample documents and production records.</p></article></div>
  </section>
  <WorkflowStory {stages} {paused}/>
  <section class="es-detail-band" data-reveal>
   <div class="es-detail-inner"><div><div class="es-micro">THE DETAILS MAKE THE DIFFERENCE</div><h2>Small details.<br/>Shared understanding.</h2><p>Fabric references. Sample sizes. Approved specifications.<br/>The information a team needs, connected to its work.</p></div><div class="es-material-study" aria-label="Illustration of fabric information"><div class="es-swatch swatch-back"></div><div class="es-swatch swatch-mid"></div><div class="es-swatch swatch-front"><span>EURO SUITS / MATERIAL LIBRARY</span><div class="es-weave"></div><strong>From swatch to specification.</strong><div class="es-swatch-fields"><span>ETF / ESF</span><span>COMPOSITION</span><span>COLOUR</span></div></div><span class="es-illustration">ILLUSTRATIVE MATERIAL CARD</span></div></div>
  </section>
  <section id="workspaces" class="es-section es-workspaces">
   <div class="es-section-line"><span><i></i>04 / A PLACE FOR EVERY TEAM</span><span>YOUR ROLE. YOUR WORKSPACE.</span></div>
   <div class="es-flow-heading" data-reveal><h2 use:splitReveal={{paused:()=>paused}}>Different responsibilities.<br/><span>The same direction.</span></h2><p>Choose your department to sign in.<br/>Your account determines your access.</p></div>
   <div class="es-department-grid">{#each departments as d,i}<article data-reveal use:tilt={{paused:()=>paused}}><div class="es-dept-index">0{i+1}<ArrowUpRight size={20}/></div><h3>{d.name}</h3><p>{d.text}</p><div class="es-portal-links">{#each d.roles as role,j}<a href={`/${d.links[j]}`}>{role}<ArrowUpRight size={13}/></a>{/each}</div></article>{/each}</div>
   <div class="es-access-note"><LockKeyhole size={17}/><span>Accounts are created by your administrator. Selecting a workspace does not change your permissions.</span><a href="/signin">General sign in <ArrowRight size={15}/></a></div>
  </section>
  <section class="es-section es-faq" data-reveal>
   <div><div class="es-micro">A LITTLE MORE CLARITY</div><h2>Before you<br/><span>step inside.</span></h2></div>
   <div class="es-questions"><details><summary>How do I get access?<span>+</span></summary><p>Your administrator creates your account, assigns a role and enables the features you need. Use your own username and password on your department’s sign-in page.</p></details><details><summary>Can I see another team’s orders?<span>+</span></summary><p>Your access follows the company, team, line and order permissions assigned to your account. Contact your administrator if your responsibilities change.</p></details><details><summary>Where do I find my daily work?<span>+</span></summary><p>Sign in to your workspace to see the records and features available to you. Production teams can follow assigned work and record progress in their department.</p></details><details><summary>Who can help with an account or workflow issue?<span>+</span></summary><p>Contact your company administrator or the Euro Suits IT team at <a href="mailto:it@eurosuitsmfg.com">it@eurosuitsmfg.com</a>. Never share passwords or verification codes.</p></details></div>
  </section>
  <section class="es-cta"><div class="es-micro">THE NEXT STEP IS YOURS</div><h2>Let’s get<br/>to work<span>.</span></h2><a class="es-btn" href="/signin">Open Euro Suits ERP <ArrowUpRight size={21}/></a><div class="es-cta-orbit" aria-hidden="true"></div></section>
 </main>
 <button class="es-motion-dock" onclick={()=>paused=!paused} aria-label={paused?'Enable page animations':'Pause page animations'} aria-pressed={paused}>{#if paused}<Play size={14}/><span>Motion off</span>{:else}<Pause size={14}/><span>Motion on</span>{/if}</button>
 <footer class="es-footer"><a class="es-brand" href="/"><span class="es-logo"><img src="/logo.png" width="40" height="40" alt=""/></span><span>EURO SUITS<span class="brand-caption">MANUFACTURING ERP</span></span></a><div><a href="#workflow">Workflow</a><a href="/guide.html">Dashboard guide</a><a href="/privacy">Privacy</a><a href="mailto:it@eurosuitsmfg.com">Contact IT <ArrowUpRight size={13}/></a></div><span>© {new Date().getFullYear()} Euro Suits MFG</span></footer>
</div>
