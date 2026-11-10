$url = "http://localhost:8088/assets/index-C6Mo56A2.js"
$content = (Invoke-WebRequest -Uri $url -UseBasicParsing).Content
Write-Host "File length: " $content.Length
Write-Host "Contains 'dc-rack-2d': " $content.Contains("dc-rack-2d")
Write-Host "Contains 'dcRack2D': " $content.Contains("dcRack2D")
Write-Host "Contains '平面机架监控': " $content.Contains("平面机架监控")
