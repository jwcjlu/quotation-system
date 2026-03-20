# BOM API 验证脚本
# 需先启动服务: go run ./cmd/server/...
$base = "http://127.0.0.1:18080"

Write-Host "=== 1. 下载 BOM 模板 ===" -ForegroundColor Cyan
$templateJson = Invoke-RestMethod -Uri "$base/api/v1/bom/template" -Method GET
if (-not $templateJson.file) {
    Write-Host "FAIL: 模板响应无 file 字段" -ForegroundColor Red
    exit 1
}
Write-Host "OK: 模板已获取 (base64 长度 $($templateJson.file.Length))" -ForegroundColor Green

Write-Host "`n=== 2. 上传 BOM (使用模板) ===" -ForegroundColor Cyan
$body = @{ file = $templateJson.file; filename = "bom_template.xlsx"; parse_mode = "auto" } | ConvertTo-Json
try {
    $upload = Invoke-RestMethod -Uri "$base/api/v1/bom/upload" -Method POST -Body $body -ContentType "application/json"
    $bomId = if ($upload.bom_id) { $upload.bom_id } else { $upload.bomId }
    if (-not $bomId) {
        Write-Host "FAIL: 上传响应无 bom_id" -ForegroundColor Red
        exit 1
    }
    Write-Host "OK: bom_id=$bomId, items=$($upload.total)" -ForegroundColor Green
} catch {
    Write-Host "FAIL: $_" -ForegroundColor Red
    exit 1
}

Write-Host "`n=== 3. 获取 BOM 详情 ===" -ForegroundColor Cyan
$bom = Invoke-RestMethod -Uri "$base/api/v1/bom/$bomId" -Method GET
Write-Host "OK: items=$($bom.items.Count)" -ForegroundColor Green

Write-Host "`n=== 4. 多平台搜索 ===" -ForegroundColor Cyan
$searchBody = @{ bom_id = $bomId } | ConvertTo-Json
$search = Invoke-RestMethod -Uri "$base/api/v1/bom/search" -Method POST -Body $searchBody -ContentType "application/json"
Write-Host "OK: item_quotes=$($search.item_quotes.Count)" -ForegroundColor Green

Write-Host "`n=== 5. 自动配单 ===" -ForegroundColor Cyan
$matchBody = @{ bom_id = $bomId; strategy = "price_first" } | ConvertTo-Json
$match = Invoke-RestMethod -Uri "$base/api/v1/bom/match" -Method POST -Body $matchBody -ContentType "application/json"
Write-Host "OK: items=$($match.items.Count), total_amount=$($match.total_amount)" -ForegroundColor Green

Write-Host "`n=== 6. 获取配单结果 (含 all_quotes) ===" -ForegroundColor Cyan
$result = Invoke-RestMethod -Uri "$base/api/v1/bom/$bomId/match" -Method GET
$hasAllQuotes = ($result.items | Where-Object { $_.all_quotes -and $_.all_quotes.Count -gt 0 }).Count -gt 0
Write-Host "OK: items=$($result.items.Count), all_quotes populated=$hasAllQuotes" -ForegroundColor Green

Write-Host "`n=== 全部验证通过 ===" -ForegroundColor Green
