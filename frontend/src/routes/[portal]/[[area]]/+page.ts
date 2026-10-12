import {error, redirect} from '@sveltejs/kit';
import type {PageLoad} from './$types';

export const ssr = false;

const portals = ['developer','admin','md','finance','management','accounts','merchandising','planning','cad','sourcing','stores','sampling','cutting','sewing','finishing','quality','warehouse'];
const areas = ['companies','reminders','fields','users','operations'];

export const load: PageLoad = ({params, url}) => {
 const portal = params.portal.toLowerCase();
 const area = params.area?.toLowerCase();
 if (!portals.includes(portal) || (area && !areas.includes(area))) {
  error(404, 'This workspace page does not exist.');
 }
 if (params.portal !== portal || params.area !== area) {
  redirect(307, `/${portal}${area ? '/'+area : ''}${url.search}`);
 }
 return {portal, area: area || 'companies'};
};
