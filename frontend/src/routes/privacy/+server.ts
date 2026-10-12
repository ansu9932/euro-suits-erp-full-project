import policy from '../../../static/privacy.html?raw';
export function GET() {
 return new Response(policy, {headers: {'Content-Type':'text/html; charset=utf-8','Cache-Control':'public, max-age=300'}});
}
