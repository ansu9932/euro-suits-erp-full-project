<script lang="ts">
 import {Download,Eye} from '@lucide/svelte';
 import {companyFileCanPreview,companyFilePreviewLabel} from './companyDocuments';
 import type {Row} from './types';
 let {doc}:{doc:Row}=$props();
</script>
{#if companyFileCanPreview(doc)}
 <a class="text-button" href={'/api/company-files/'+doc.id+'?preview=1'} target="_blank" rel="noopener noreferrer" aria-label={companyFilePreviewLabel(doc)+' '+doc.name+' in a new tab'}><Eye size={16}/>{companyFilePreviewLabel(doc)}</a>
 <a class="text-button" href={'/api/company-files/'+doc.id} download aria-label={'Download '+doc.name}><Download size={16}/>Download</a>
{:else}
 <a class="text-button" href={'/api/company-files/'+doc.id} download aria-label={'Download '+doc.name+' to preview'}><Download size={16}/>Download to preview</a>
{/if}
