# One-time SolarEdge Site Access OAuth. Opens a browser, catches the redirect
# on http://127.0.0.1:8765/callback, exchanges the code for tokens, and writes
# config.local.json fields + a local token file for deploy.
#
# In the SolarEdge developer portal, set the app redirect URI to exactly:
#   http://127.0.0.1:8765/callback
#
# Usage (repo root): powershell -File deploy/pi/solaredge-auth.ps1
$ErrorActionPreference = 'Stop'
$Root = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$cfgPath = Join-Path $Root 'config.local.json'
if (-not (Test-Path $cfgPath)) { throw "Missing $cfgPath" }
$cfg = Get-Content $cfgPath -Raw | ConvertFrom-Json

$clientId = [string]$cfg.solaredge_client_id
$clientSecret = [string]$cfg.solaredge_client_secret
$siteId = [string]$cfg.solaredge_site_id
if (-not $clientId -or -not $clientSecret) {
  throw "config.local.json needs solaredge_client_id and solaredge_client_secret"
}
if (-not $siteId) { throw "config.local.json needs solaredge_site_id" }

$redirect = 'http://127.0.0.1:8765/callback'
$listen = [System.Net.HttpListener]::new()
$listen.Prefixes.Add('http://127.0.0.1:8765/')
$listen.Start()

$q = @(
  "response_type=code",
  "client_id=$([uri]::EscapeDataString($clientId))",
  "redirect_uri=$([uri]::EscapeDataString($redirect))",
  "scope=$([uri]::EscapeDataString('SITE_DATA DEVICE_DATA'))",
  "access_duration=1051200",
  "external_id=kindle-shazam"
) -join '&'
$auth = "https://connect.solaredge.com/authorize?$q"

Write-Host "Opening SolarEdge authorize URL…"
Write-Host $auth
Start-Process $auth

Write-Host "Waiting for redirect on $redirect …"
Write-Host "(If the page errors, add this exact redirect URI in the SolarEdge developer portal.)"
$ctx = $listen.GetContext()
$code = $ctx.Request.QueryString['code']
$redirSite = $ctx.Request.QueryString['site_id']
$html = '<html><body><h2>KindleID SolarEdge auth OK</h2><p>You can close this tab.</p></body></html>'
$bytes = [Text.Encoding]::UTF8.GetBytes($html)
$ctx.Response.ContentType = 'text/html; charset=utf-8'
$ctx.Response.OutputStream.Write($bytes, 0, $bytes.Length)
$ctx.Response.Close()
$listen.Stop()

if (-not $code) {
  throw "No code in redirect: $($ctx.Request.Url)"
}
if ($redirSite) {
  Write-Host "SolarEdge returned site_id=$redirSite"
  $siteId = $redirSite
}

$pair = '{0}:{1}' -f $clientId, $clientSecret
$basic = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($pair))
$body = 'grant_type=authorization_code&code={0}&redirect_uri={1}' -f [uri]::EscapeDataString($code), [uri]::EscapeDataString($redirect)
$resp = Invoke-RestMethod -Method Post -Uri 'https://monitoringapi.solaredge.com/v2/oauth2/token' `
  -Headers @{ Authorization = "Basic $basic"; Accept = 'application/json' } `
  -ContentType 'application/x-www-form-urlencoded' `
  -Body $body

if (-not $resp.refresh_token) { throw "No refresh_token in response: $($resp | ConvertTo-Json)" }

$cfg | Add-Member -NotePropertyName solaredge_site_id -NotePropertyValue $siteId -Force
$cfg | Add-Member -NotePropertyName solaredge_refresh_token -NotePropertyValue ([string]$resp.refresh_token) -Force
$cfg | ConvertTo-Json -Depth 5 | Set-Content -Path $cfgPath -Encoding utf8

$tokenPath = Join-Path $Root 'solaredge-token.json'
$exp = (Get-Date).ToUniversalTime().AddSeconds([int]($resp.expires_in))
$tokenObj = [ordered]@{
  access_token  = [string]$resp.access_token
  refresh_token = [string]$resp.refresh_token
  expires_at    = $exp.ToString('o')
  site_id       = $siteId
}
$tokenObj | ConvertTo-Json | Set-Content -Path $tokenPath -Encoding utf8

Write-Host "Saved refresh token to config.local.json and $tokenPath"
Write-Host "Next: powershell -File deploy/pi/deploy.ps1"
