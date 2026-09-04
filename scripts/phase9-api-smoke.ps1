param(
  [string]$BaseUrl = 'http://127.0.0.1:8080/api',
  [string]$Account = 'admin',
  [string]$Password = 'admin'
)

$ErrorActionPreference = 'Stop'
$BaseUrl = $BaseUrl.TrimEnd('/')

function Assert-Status([string]$Name, [int]$Actual, [int]$Expected) {
  if ($Actual -ne $Expected) {
    throw "$Name returned HTTP $Actual; expected HTTP $Expected"
  }
  Write-Host "PASS $Name ($Actual)"
}

function Get-Token($LoginResponse) {
  if ($LoginResponse.data -and $LoginResponse.data.token) { return $LoginResponse.data.token }
  if ($LoginResponse.token) { return $LoginResponse.token }
  throw 'login response does not contain a token'
}

$health = Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/health"
Assert-Status 'health' $health.StatusCode 200
$healthBody = $health.Content | ConvertFrom-Json
if ($healthBody.data.status -ne 'ok' -or $healthBody.data.database -ne 'ok') {
  throw 'health response does not report database/status ok'
}

$loginBody = @{ account = $Account; password = $Password; rememberMe = $false } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/auth/login" -ContentType 'application/json' -Body $loginBody
$token = Get-Token $login
Write-Host 'PASS auth/login (200)'
$headers = @{ Authorization = "Bearer $token" }

foreach ($path in @('/drawings?page=1&page_size=5', '/drawing-attributes', '/drawing-relations/branches', '/drawing-relations/borrows', '/attachments')) {
  $response = Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri "$BaseUrl$path"
  Assert-Status $path $response.StatusCode 200
}

try {
  Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri "$BaseUrl/data/structure" | Out-Null
  throw 'legacy /data/structure unexpectedly exists'
} catch {
  if ($_.Exception.Response.StatusCode.value__ -ne 404) { throw }
  Write-Host 'PASS legacy /data/structure (404)'
}

Write-Host 'Phase 9 API smoke test passed.'
