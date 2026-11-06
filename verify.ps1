# verify.ps1 - strict acceptance check for the Product Inventory project.
# Run from the project folder:  powershell -ExecutionPolicy Bypass -File .\verify.ps1
# It uses defaults from .env.example (inventory/inventory), the API on :8080 and Docker.
# It runs "docker compose down" (WITHOUT -v) once to test persistence, and leaves the stack running.

$ErrorActionPreference = "Continue"
$base = "http://localhost:8080"
$script:pass = 0; $script:fail = 0; $script:failed = @()

function Check($name, $ok, $detail) {
  if ($ok) { $script:pass++; Write-Host ("  [PASS] " + $name) -ForegroundColor Green }
  else { $script:fail++; $script:failed += $name; Write-Host ("  [FAIL] " + $name + "  " + $detail) -ForegroundColor Red }
}
function Section($t) { Write-Host ""; Write-Host ("== " + $t) -ForegroundColor Cyan }

function Call($method, $path, $body) {
  $out = [System.IO.Path]::GetTempFileName()
  $curlArgs = @("-s", "-o", $out, "-w", "%{http_code}", "-X", $method, ($base + $path))
  $bf = $null
  if ($body) {
    $bf = [System.IO.Path]::GetTempFileName()
    [System.IO.File]::WriteAllText($bf, $body, (New-Object System.Text.UTF8Encoding($false)))
    $curlArgs += @("-H", "Content-Type: application/json", "--data", ("@" + $bf))
  }
  $code = (& curl.exe @curlArgs)
  $txt = [System.IO.File]::ReadAllText($out)
  Remove-Item $out -ErrorAction SilentlyContinue
  if ($bf) { Remove-Item $bf -ErrorAction SilentlyContinue }
  $json = $null
  try { if ($txt) { $json = $txt | ConvertFrom-Json } } catch {}
  return [pscustomobject]@{ Code = [int]$code; Text = $txt; Json = $json }
}
function HasErrShape($r) { return ($r.Json -and $r.Json.error -and $r.Json.message) }
function WaitHealthy($secs) {
  for ($i = 0; $i -lt $secs; $i++) {
    $h = Call "GET" "/healthz" $null
    if ($h.Code -eq 200) { return $true }
    Start-Sleep -Seconds 1
  }
  return $false
}
$dbUrl = "-database=postgres://inventory:inventory@db:5432/inventory?sslmode=disable"

Section "1. Project files"
foreach ($f in @("Dockerfile","docker-compose.yml","Makefile","README.md","requests.http","sqlc.yaml","internal/product/queries.sql","cmd/seeder","cmd/api",".env.example")) {
  Check ("exists: " + $f) (Test-Path $f) "missing"
}
$ups = @(Get-ChildItem migrations -Filter "*_create_products_table.up.sql" -ErrorAction SilentlyContinue)
$downs = @(Get-ChildItem migrations -Filter "*_create_products_table.down.sql" -ErrorAction SilentlyContinue)
Check "migration up + down pair present" (($ups.Count -eq 1) -and ($downs.Count -eq 1)) "need one .up.sql and one .down.sql"
if ($ups.Count -eq 1) {
  $up = (Get-Content $ups[0].FullName -Raw)
  Check "up: price >= 0 check" ($up -match "price\s*>=\s*0") ""
  Check "up: stock_quantity >= 0 check" ($up -match "stock_quantity\s*>=\s*0") ""
  Check "up: unique sku" ($up -match "(?i)unique") ""
  Check "up: index on status" ($up -match "(?i)index[^;]*status") ""
  Check "up: status allowed values" (($up -match "draft") -and ($up -match "active") -and ($up -match "archived")) ""
}
$q = Get-Content "internal/product/queries.sql" -Raw
foreach ($n in @("CreateProduct :one","GetProductByID :one","GetProductBySKU :one","ListProducts :many","CountProducts :one","UpdateProduct :one","DeleteProduct :exec")) {
  Check ("query: " + $n) ($q -match [regex]::Escape("-- name: " + $n)) ""
}
$df = Get-Content "Dockerfile" -Raw
Check "Dockerfile: multi-stage (2+ FROM)" ((([regex]::Matches($df, "(?m)^FROM ")).Count) -ge 2) ""
Check "Dockerfile: -ldflags=-s -w" ($df -match "-s -w") ""
Check "Dockerfile: non-root USER" ($df -match "(?m)^USER\s+\S+") ""
$dc = Get-Content "docker-compose.yml" -Raw
Check "compose: pg_isready healthcheck" ($dc -match "pg_isready") ""
Check "compose: depends_on service_healthy" ($dc -match "service_healthy") ""
Check "compose: named volume" ($dc -match "(?m)^volumes:") ""
Check "compose: DB_URL / PORT / ENV env vars" (($dc -match "DB_URL") -and ($dc -match "PORT") -and ($dc -match "ENV")) ""
Check "Makefile targets: migrate-up migrate-down seed sqlc" (((Get-Content Makefile -Raw) -match "(?m)^migrate-up:") -and ((Get-Content Makefile -Raw) -match "(?m)^migrate-down:") -and ((Get-Content Makefile -Raw) -match "(?m)^seed:") -and ((Get-Content Makefile -Raw) -match "(?m)^sqlc:")) ""

Section "2. Go build, vet, tests"
& go build ./... 2>&1 | Out-Null; Check "go build ./..." ($LASTEXITCODE -eq 0) ""
& go vet ./... 2>&1 | Out-Null; Check "go vet ./..." ($LASTEXITCODE -eq 0) ""
$t = (& go test -count=1 ./... 2>&1 | Out-String)
Check "go test ./... (no FAIL)" (($LASTEXITCODE -eq 0) -and ($t -notmatch "FAIL")) $t
Check "tests cover handlers (httptest) + panic recovery" ((Select-String -Path "internal/server/*_test.go" -Pattern "httptest" -Quiet) -and (Select-String -Path "internal/httpx/*_test.go" -Pattern "(?i)panic" -Quiet)) "no httptest / panic test found"

Section "3. sqlc generates with no changes (needs Docker)"
$before = (Get-ChildItem internal/product/productdb -Filter *.go | Get-FileHash | ForEach-Object { $_.Hash }) -join ","
& docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.27.0 generate 2>&1 | Out-Null
$sqlcCode = $LASTEXITCODE
$after = (Get-ChildItem internal/product/productdb -Filter *.go | Get-FileHash | ForEach-Object { $_.Hash }) -join ","
Check "sqlc generate succeeds" ($sqlcCode -eq 0) "docker/sqlc failed"
Check "generated code is stable (already up to date)" ($before -eq $after) "generated files changed - rerun go build/test"
& go build ./... 2>&1 | Out-Null; Check "go build after sqlc" ($LASTEXITCODE -eq 0) ""

Section "4. Docker stack, migrations, seeder"
& docker compose up -d --build 2>&1 | Out-Null
Check "docker compose up --build" ($LASTEXITCODE -eq 0) ""
Check "API healthy within 60s" (WaitHealthy 60) "API did not become healthy"
$ps = (& docker compose ps 2>&1 | Out-String)
Check "db container healthy" ($ps -match "db-1.*healthy") $ps
Check "api container healthy" ($ps -match "api-1.*healthy") $ps
$h = Call "GET" "/healthz" $null
Check "GET /healthz 200 + database up" (($h.Code -eq 200) -and ($h.Text -match '"database":\s*"up"')) $h.Text

$d = (& docker compose run --rm migrate "-path=/migrations" $dbUrl down 1 2>&1 | Out-String)
Check "migrate down 1" (($LASTEXITCODE -eq 0) -and ($d -match "1/d")) $d
$u = (& docker compose run --rm migrate "-path=/migrations" $dbUrl up 2>&1 | Out-String)
Check "migrate up" (($LASTEXITCODE -eq 0) -and ($u -match "1/u")) $u
$s1 = (& docker compose --profile tools run --rm seed 2>&1 | Out-String)
Check "seed inserts 14 on empty table" ($s1 -match "14 inserted, 0 skipped") $s1
$s2 = (& docker compose --profile tools run --rm seed 2>&1 | Out-String)
Check "seed is idempotent (0 inserted, 14 skipped)" ($s2 -match "0 inserted, 14 skipped") $s2
& docker compose restart api 2>&1 | Out-Null
Check "API healthy after restart" (WaitHealthy 60) ""

Section "5. Seed data quality"
$all = Call "GET" "/api/v1/products?page_size=100" $null
$items = @($all.Json.data)
Check ">= 10 products" ($items.Count -ge 10) ("count=" + $items.Count)
Check "all three statuses present" ((@($items | Where-Object { $_.status -eq "draft" }).Count -gt 0) -and (@($items | Where-Object { $_.status -eq "active" }).Count -gt 0) -and (@($items | Where-Object { $_.status -eq "archived" }).Count -gt 0)) ""
Check "at least one stock_quantity = 0" (@($items | Where-Object { $_.stock_quantity -eq 0 }).Count -ge 1) ""
Check "price variation (min != max)" ((($items | Measure-Object price -Min).Minimum) -ne (($items | Measure-Object price -Max).Maximum)) ""

Section "6. GET /api/v1/products (list, filters, bonus)"
$r = Call "GET" "/api/v1/products" $null
Check "default list: 200 + defaults page=1 page_size=10" (($r.Code -eq 200) -and ($r.Json.pagination.page -eq 1) -and ($r.Json.pagination.page_size -eq 10) -and (@($r.Json.data).Count -eq 10)) $r.Text
Check "pagination has total_items,page,page_size,total_pages" ($r.Json.pagination.total_items -and $r.Json.pagination.total_pages) ""
$r = Call "GET" "/api/v1/products?page=2&page_size=5" $null
Check "page=2&page_size=5 -> 5 items, page 2" (($r.Code -eq 200) -and (@($r.Json.data).Count -eq 5) -and ($r.Json.pagination.page -eq 2)) $r.Text
$r = Call "GET" "/api/v1/products?limit=3" $null
Check "limit=3 alias works" (($r.Code -eq 200) -and (@($r.Json.data).Count -eq 3)) $r.Text
$r = Call "GET" "/api/v1/products?page_size=1000" $null
Check "page_size max is 100" (($r.Code -eq 200) -and ($r.Json.pagination.page_size -le 100)) $r.Text
foreach ($st in @("draft","active","archived")) {
  $r = Call "GET" ("/api/v1/products?status=" + $st + "&page_size=100") $null
  $rows = @($r.Json.data)
  Check ("status=" + $st + " filter") (($r.Code -eq 200) -and ($rows.Count -gt 0) -and (@($rows | Where-Object { $_.status -ne $st }).Count -eq 0)) $r.Text
}
$r = Call "GET" "/api/v1/products?status=bogus" $null
Check "invalid status filter -> 400 + error shape" (($r.Code -eq 400) -and (HasErrShape $r)) $r.Text
$r = Call "GET" "/api/v1/products?page=0" $null
Check "page=0 -> 400" ($r.Code -eq 400) $r.Text
$r1 = Call "GET" "/api/v1/products?q=usb" $null
$r2 = Call "GET" "/api/v1/products?q=USB" $null
Check "q= search, case-insensitive partial" (($r1.Code -eq 200) -and (@($r1.Json.data).Count -ge 1) -and (@($r1.Json.data).Count -eq @($r2.Json.data).Count)) $r1.Text
$r = Call "GET" "/api/v1/products?low_stock=true&page_size=100" $null
Check "low_stock=true -> only stock <= 5" (($r.Code -eq 200) -and (@($r.Json.data).Count -gt 0) -and (@(@($r.Json.data) | Where-Object { $_.stock_quantity -gt 5 }).Count -eq 0)) $r.Text

Section "7. POST / GET / PUT / DELETE"
$sku = "VERIFY-" + (Get-Random -Minimum 10000 -Maximum 99999)
$r = Call "POST" "/api/v1/products" ('{"name":"Verify Item","sku":"' + $sku + '","price":10.5}')
Check "POST minimal body -> 201" ($r.Code -eq 201) $r.Text
Check "POST defaults: stock 0, status draft, description null" (($r.Json.stock_quantity -eq 0) -and ($r.Json.status -eq "draft") -and ($null -eq $r.Json.description)) $r.Text
Check "POST returns id, created_at, updated_at" (($r.Json.id) -and ($r.Json.created_at) -and ($r.Json.updated_at)) $r.Text
$id = $r.Json.id
$r = Call "POST" "/api/v1/products" ('{"name":"Verify Item","sku":"' + $sku + '","price":10.5}')
Check "POST duplicate SKU -> 409 + error shape" (($r.Code -eq 409) -and (HasErrShape $r)) $r.Text
$r = Call "POST" "/api/v1/products" '{bad json'
Check "POST invalid JSON -> 400 + error shape" (($r.Code -eq 400) -and (HasErrShape $r)) $r.Text
$r = Call "POST" "/api/v1/products" '{"name":"","sku":"BAD-1","price":-5,"status":"weird"}'
Check "POST validation -> 400 with details (name, price, status)" (($r.Code -eq 400) -and $r.Json.details.name -and $r.Json.details.price -and $r.Json.details.status) $r.Text
$r = Call "POST" "/api/v1/products" '{"name":"X","sku":"lower-case","price":1}'
Check "POST bad SKU format -> 400" ($r.Code -eq 400) $r.Text
$r = Call "POST" "/api/v1/products" '{"name":"X","sku":"NEG-STOCK","price":1,"stock_quantity":-1}'
Check "POST negative stock -> 400" ($r.Code -eq 400) $r.Text
$r = Call "POST" "/api/v1/products" '{"sku":"NO-NAME","price":1}'
Check "POST missing name -> 400" ($r.Code -eq 400) $r.Text

$r = Call "GET" ("/api/v1/products/" + $id) $null
Check "GET /{id} -> 200" (($r.Code -eq 200) -and ($r.Json.sku -eq $sku)) $r.Text
$r = Call "GET" "/api/v1/products/abc" $null
Check "GET invalid id -> 400 + error shape" (($r.Code -eq 400) -and (HasErrShape $r)) $r.Text
$r = Call "GET" "/api/v1/products/999999" $null
Check "GET missing id -> 404 + error shape" (($r.Code -eq 404) -and (HasErrShape $r)) $r.Text

$r = Call "PUT" ("/api/v1/products/" + $id) '{"name":"Verify Item v2","price":24.5,"stock_quantity":10,"status":"active"}'
Check "PUT update -> 200 with new values" (($r.Code -eq 200) -and ($r.Json.name -eq "Verify Item v2") -and ($r.Json.price -eq 24.5) -and ($r.Json.stock_quantity -eq 10) -and ($r.Json.status -eq "active")) $r.Text
Check "PUT bumps updated_at" ([datetime]$r.Json.updated_at -gt [datetime]$r.Json.created_at) $r.Text
$r = Call "PUT" ("/api/v1/products/" + $id) '{"price":-1}'
Check "PUT validation -> 400" ($r.Code -eq 400) $r.Text
$r = Call "PUT" ("/api/v1/products/" + $id) '{"sku":"CAB-USBC-2M"}'
Check "PUT duplicate SKU -> 409" ($r.Code -eq 409) $r.Text
$r = Call "PUT" "/api/v1/products/999999" '{"name":"Nope"}'
Check "PUT missing id -> 404" ($r.Code -eq 404) $r.Text
$r = Call "PUT" "/api/v1/products/abc" '{"name":"Nope"}'
Check "PUT invalid id -> 400" ($r.Code -eq 400) $r.Text

$r = Call "DELETE" ("/api/v1/products/" + $id) $null
Check "DELETE -> 204 empty body" (($r.Code -eq 204) -and (-not $r.Text)) $r.Text
$r = Call "GET" ("/api/v1/products/" + $id) $null
Check "GET after DELETE -> 404" ($r.Code -eq 404) $r.Text
$r = Call "DELETE" ("/api/v1/products/" + $id) $null
Check "DELETE again -> 404" ($r.Code -eq 404) $r.Text
$r = Call "DELETE" "/api/v1/products/abc" $null
Check "DELETE invalid id -> 400" ($r.Code -eq 400) $r.Text

Section "8. Persistence across docker compose down / up"
$cb = (Call "GET" "/api/v1/products?page_size=1" $null).Json.pagination.total_items
& docker compose down 2>&1 | Out-Null
Check "docker compose down" ($LASTEXITCODE -eq 0) ""
& docker compose up -d 2>&1 | Out-Null
Check "docker compose up -d" ($LASTEXITCODE -eq 0) ""
Check "API healthy again" (WaitHealthy 90) ""
$ca = (Call "GET" "/api/v1/products?page_size=1" $null).Json.pagination.total_items
Check ("data persisted (before=" + $cb + ", after=" + $ca + ")") (($cb -ge 14) -and ($cb -eq $ca)) ""

Write-Host ""
Write-Host "=========================================" -ForegroundColor Cyan
Write-Host ("PASSED: " + $script:pass + "    FAILED: " + $script:fail) -ForegroundColor $(if ($script:fail -eq 0) { "Green" } else { "Red" })
if ($script:fail -gt 0) { Write-Host "Failed checks:" -ForegroundColor Red; $script:failed | ForEach-Object { Write-Host ("  - " + $_) -ForegroundColor Red } }
else { Write-Host "ALL CHECKS PASSED - project is ready to submit." -ForegroundColor Green }
Write-Host "Note: panic recovery (Recoverer) is verified by the Go tests in section 2."
exit $script:fail
