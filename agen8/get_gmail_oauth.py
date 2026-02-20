#!/usr/bin/env python3
"""
Quick OAuth2 helper to get a refresh token for Gmail.
Run this once to get your refresh token.
"""

import http.server
import urllib.parse
import webbrowser
import json

# You'll need to fill these in from Google Cloud Console
CLIENT_ID = input("Enter your OAuth Client ID: ").strip()
CLIENT_SECRET = input("Enter your OAuth Client Secret: ").strip()

# OAuth endpoints
AUTH_URL = "https://accounts.google.com/o/oauth2/v2/auth"
TOKEN_URL = "https://oauth2.googleapis.com/token"
REDIRECT_URI = "http://localhost:8080"
SCOPE = "https://mail.google.com/"

# Step 1: Build authorization URL
auth_params = {
    "client_id": CLIENT_ID,
    "redirect_uri": REDIRECT_URI,
    "response_type": "code",
    "scope": SCOPE,
    "access_type": "offline",  # This gets us a refresh token
    "prompt": "consent",  # Force consent to get refresh token
}
auth_url = f"{AUTH_URL}?{urllib.parse.urlencode(auth_params)}"

print("\n" + "="*60)
print("Step 1: Authorize the app")
print("="*60)
print(f"\nOpening browser to:\n{auth_url}\n")
print("After authorizing, you'll be redirected to localhost:8080")
print("Don't close this terminal!")
print("="*60 + "\n")

# Step 2: Start local server to catch the redirect
class OAuthHandler(http.server.BaseHTTPRequestHandler):
    auth_code = None
    
    def do_GET(self):
        # Parse the authorization code from the URL
        query = urllib.parse.urlparse(self.path).query
        params = urllib.parse.parse_qs(query)
        
        if 'code' in params:
            OAuthHandler.auth_code = params['code'][0]
            
            # Send success response
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b"""
                <html><body style="font-family: sans-serif; text-align: center; padding: 50px;">
                    <h1>Success!</h1>
                    <p>You can close this window and return to the terminal.</p>
                </body></html>
            """)
        else:
            self.send_response(400)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b"<html><body><h1>Error: No code received</h1></body></html>")
    
    def log_message(self, format, *args):
        pass  # Suppress logging

# Open browser
webbrowser.open(auth_url)

# Start server
print("Starting local server on port 8080...")
server = http.server.HTTPServer(('localhost', 8080), OAuthHandler)
server.handle_request()  # Handle one request then stop

if not OAuthHandler.auth_code:
    print("\n❌ Failed to get authorization code")
    exit(1)

print("\n✅ Got authorization code!")

# Step 3: Exchange code for tokens
print("\nExchanging code for tokens...")

import urllib.request

token_data = {
    "code": OAuthHandler.auth_code,
    "client_id": CLIENT_ID,
    "client_secret": CLIENT_SECRET,
    "redirect_uri": REDIRECT_URI,
    "grant_type": "authorization_code",
}

req = urllib.request.Request(
    TOKEN_URL,
    data=urllib.parse.urlencode(token_data).encode(),
    headers={"Content-Type": "application/x-www-form-urlencoded"},
)

try:
    with urllib.request.urlopen(req) as response:
        tokens = json.loads(response.read())
        
    print("\n" + "="*60)
    print("✅ SUCCESS! Copy these to your .env file:")
    print("="*60)
    print(f'\nGMAIL_USER="your-email@gmail.com"')
    print(f'GMAIL_FROM="your-email@gmail.com"')
    print(f'GOOGLE_OAUTH_CLIENT_ID="{CLIENT_ID}"')
    print(f'GOOGLE_OAUTH_CLIENT_SECRET="{CLIENT_SECRET}"')
    print(f'GOOGLE_OAUTH_REFRESH_TOKEN="{tokens["refresh_token"]}"')
    print("\n" + "="*60)
    
except urllib.error.HTTPError as e:
    error_body = e.read().decode()
    print(f"\n❌ Error exchanging code: {error_body}")
    exit(1)
