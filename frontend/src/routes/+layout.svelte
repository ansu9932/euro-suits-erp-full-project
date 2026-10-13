<script lang="ts">
 import {onMount} from 'svelte';
 import {reportClientIssue} from '$lib/diagnostics';
 import {installActionLocks} from '$lib/actionLock';
 import '../interactions.css';
 let {children}=$props();
 onMount(()=>{
  const removeActionLocks=installActionLocks();
  const runtime=()=>reportClientIssue('runtime'),promise=()=>reportClientIssue('promise');
  window.addEventListener('error',runtime);window.addEventListener('unhandledrejection',promise);
  return()=>{removeActionLocks();window.removeEventListener('error',runtime);window.removeEventListener('unhandledrejection',promise)};
 });
</script>
{@render children()}
