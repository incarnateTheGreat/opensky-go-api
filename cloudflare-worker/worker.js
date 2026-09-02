/**
 * Cloudflare Worker - OpenSky API Proxy
 * 
 * Deploy this to Cloudflare Workers to proxy requests to OpenSky Network.
 * This bypasses IP-based blocking from certain hosting providers.
 * 
 * Setup:
 * 1. Go to https://dash.cloudflare.com/ → Workers & Pages → Create Worker
 * 2. Paste this code and deploy
 * 3. (Optional) Add API_KEY secret in Settings → Variables for security
 * 4. Set OPENSKY_BASE_URL in Railway to your worker URL
 * 
 * Usage from Railway:
 *   OPENSKY_BASE_URL=https://your-worker.your-subdomain.workers.dev
 */

export default {
  async fetch(request, env) {
    // Optional: Verify API key for security
    // Set API_KEY as a secret in Cloudflare Worker settings
    if (env.API_KEY) {
      const authHeader = request.headers.get('X-Proxy-Key');
      if (authHeader !== env.API_KEY) {
        return new Response('Unauthorized', { status: 401 });
      }
    }

    const url = new URL(request.url);
    
    // Health check endpoints
    // Support both / and /health so opening the worker URL in a browser
    // doesn't proxy to OpenSky root and produce confusing 522 errors.
    if (url.pathname === '/' || url.pathname === '/health') {
      return new Response(JSON.stringify({ status: 'ok', proxy: 'cloudflare-worker' }), {
        headers: { 'Content-Type': 'application/json' }
      });
    }

    // Proxy to OpenSky - rewrite the URL
    const openSkyUrl = `https://opensky-network.org${url.pathname}${url.search}`;
    
    try {
      const response = await fetch(openSkyUrl, {
        method: request.method,
        headers: {
          'User-Agent': 'OpenSky-Proxy/1.0',
          'Accept': 'application/json',
        },
      });

      // Return the response with CORS headers (in case you need browser access too)
      const data = await response.text();
      
      return new Response(data, {
        status: response.status,
        headers: {
          'Content-Type': response.headers.get('Content-Type') || 'application/json',
          'Access-Control-Allow-Origin': '*',
          'X-Proxied-By': 'cloudflare-worker',
          'X-Upstream-Status': String(response.status),
        },
      });
    } catch (error) {
      return new Response(JSON.stringify({
        error: error.message,
        proxy: 'cloudflare-worker',
        upstream: openSkyUrl,
      }), {
        status: 502,
        headers: { 'Content-Type': 'application/json' },
      });
    }
  },
};
