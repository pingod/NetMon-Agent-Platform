$loginBody = @{ username = "admin"; password = "Admin@2026" } | ConvertTo-Json
$loginRes = Invoke-RestMethod -Uri "http://localhost:8088/api/v1/auth/login" -Method Post -Body $loginBody -ContentType "application/json"
$token = $loginRes.data.token
$headers = @{ "Authorization" = "Bearer $token" }

$endpoints = @(
  # 1. 认证
  "GET /api/v1/auth/me",
  
  # 2. 仪表盘与监控分析
  "GET /api/v1/dashboard/full",
  "GET /api/v1/dashboard/server-metrics",
  "GET /api/v1/dashboard/agent-stats",
  "GET /api/v1/dashboard/alert-trends",
  "GET /api/v1/dashboard/task-trends",
  "GET /api/v1/dashboard/task-distribution",
  "GET /api/v1/dashboard/remediation-stats",
  "GET /api/v1/dashboard/sla-stats",
  
  # 3. 网络设备与SNMP
  "GET /api/v1/network-devices",
  "GET /api/v1/network-devices/stats",
  "GET /api/v1/network-devices/categories",
  "GET /api/v1/network-devices/vendors",
  "GET /api/v1/snmp/status",
  "GET /api/v1/snmp-collect/tasks",
  "GET /api/v1/snmp-collect/mib/modules",
  "GET /api/v1/snmp-collect/custom-metrics",
  "GET /api/v1/snmp-collect/metrics/all",
  "GET /api/v1/config-backup/tasks",
  "GET /api/v1/config-backup/history",
  "GET /api/v1/topology",
  "GET /api/v1/network-subnets",
  "GET /api/v1/network-discovery/tasks",
  
  # 4. 服务器与SSH
  "GET /api/v1/servers",
  "GET /api/v1/server-groups",
  "GET /api/v1/server-commands",
  "GET /api/v1/ssh-keys",
  "GET /api/v1/agents",
  "GET /api/v1/ai-models",
  
  # 5. 数据库连接
  "GET /api/v1/db-connections",
  
  # 6. 数据中心 DC
  "GET /api/v1/dc/rooms",
  "GET /api/v1/dc/racks",
  "GET /api/v1/dc/devices",
  "GET /api/v1/dc/power-panels",
  "GET /api/v1/dc/cables",
  "GET /api/v1/dc/manufacturers",
  "GET /api/v1/dc/device-types",
  
  # 7. 告警中心与策略
  "GET /api/v1/alerts",
  "GET /api/v1/alert-rules",
  "GET /api/v1/alert-groups",
  "GET /api/v1/alert-providers",
  "GET /api/v1/alert-mappings",
  "GET /api/v1/alert-noise",
  "GET /api/v1/alert-auto-analysis",
  "GET /api/v1/alert-escalation-history",
  
  # 8. 任务、工作流与自愈
  "GET /api/v1/tasks",
  "GET /api/v1/workflows",
  "GET /api/v1/workflows/providers/list",
  "GET /api/v1/remediations/dashboard",
  "GET /api/v1/remediations/policies",
  "GET /api/v1/remediations/executions",
  
  # 9. 容器、K8s与虚拟机
  "GET /api/v1/containers",
  "GET /api/v1/containers/images",
  "GET /api/v1/containers/volumes",
  "GET /api/v1/containers/compose",
  "GET /api/v1/kubernetes/contexts",
  "GET /api/v1/virtual-machines",
  "GET /api/v1/virtual-machines/stats",
  "GET /api/v1/virtual-machines/platforms",
  
  # 10. 系统设置与辅助
  "GET /api/v1/settings",
  "GET /api/v1/audit/logs",
  "GET /api/v1/tool-links",
  "GET /api/v1/knowledge",
  "GET /api/v1/approvals"
)

$passed = 0
$failed = 0
$results = @()

foreach ($item in $endpoints) {
  $parts = $item.Split(" ")
  $method = $parts[0]
  $url = "http://localhost:8088" + $parts[1]
  
  try {
    $res = Invoke-RestMethod -Uri $url -Method $method -Headers $headers -ErrorAction Stop
    $passed++
    $results += [PSCustomObject]@{
      Endpoint = $parts[1]
      Status = "PASS (200)"
      Error = $null
    }
  } catch {
    $failed++
    $results += [PSCustomObject]@{
      Endpoint = $parts[1]
      Status = "FAIL (" + $_.Exception.Response.StatusCode.value__ + ")"
      Error = $_.Exception.Message
    }
  }
}

Write-Host "================================================"
Write-Host "API Test Summary: Total: $($endpoints.Count) | Passed: $passed | Failed: $failed"
Write-Host "================================================"
$results | Format-Table -AutoSize
