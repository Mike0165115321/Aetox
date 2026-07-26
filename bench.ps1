#Requires -Version 7
<#
  bench.ps1 — วัด Aetox เทียบกับเครื่องมือ AI ตัวอื่นบนเครื่องเดียวกัน

  กติกา ผลลัพธ์ และเหตุผลอยู่ใน BENCHMARK.md — ไฟล์นี้คือตัวที่ทำให้ตัวเลขซ้ำได้
  ตัวเลขที่วัดด้วยมือแล้วจดใส่เอกสารเฉย ๆ ไม่นับ เพราะพิสูจน์ไม่ได้ว่าวัดในสภาพไหน

  โหมด
    .\bench.ps1 -Disk                  ขนาดที่กินในเครื่องหลังติดตั้ง
    .\bench.ps1 -Start                 เวลาเปิด + RAM ตอนนิ่ง (เปิด/ปิดเองอัตโนมัติ)
    .\bench.ps1 -Snapshot              อ่าน RAM ของตัวที่เปิดค้างอยู่ตอนนี้ (ไม่แตะอะไร)
    .\bench.ps1 -Soak -Hours 8         เปิดทิ้งไว้แล้วเก็บ RAM ทุก 5 นาที หา memory leak

  ตัวเลือกร่วม
    -App <ชื่อ>       วัดเฉพาะตัวเดียว เช่น -App Aetox
    -Runs <n>         จำนวนรอบของ -Start (ค่าเริ่มต้น 5 · รอบแรกทิ้ง ที่เหลือเอา median)
    -SettleSec <n>    รอให้แอปนิ่งกี่วินาทีก่อนอ่าน RAM (ค่าเริ่มต้น 60)

  "เปิดครั้งแรก (cold)" ที่แท้จริงต้องรีบูตก่อนแล้วรัน -Start -Runs 1 เพราะรอบต่อ ๆ ไป
  ไฟล์ของแอปยังค้างอยู่ใน file cache ของ Windows — สคริปต์เตือนให้เองตอนรัน
#>

param(
  [switch]$Disk,
  [switch]$Start,
  [switch]$Snapshot,
  [switch]$Soak,
  [string]$App,
  [int]$Runs      = 5,
  [int]$SettleSec = 60,
  [int]$Hours     = 8,
  [int]$EverySec  = 300
)

$ErrorActionPreference = 'Stop'

# ── เครื่องมือที่เอามาเทียบ ───────────────────────────────────────────────────
# path เป็นค่าเริ่มต้นของ Windows — ถ้าเครื่องไหนลงไว้คนละที่ แก้ตรงนี้
# ตัวไหนหาไม่เจอสคริปต์ข้ามให้เอง ไม่ล้ม จะเพิ่มคู่แข่งตัวใหม่ก็แค่เติมแถว
#
# Kind : desktop = มีหน้าต่าง จับเวลาเปิดได้ · cli = ไม่มีหน้าต่าง วัดได้แค่ขนาดกับ RAM ตอนรัน
# Cat  : เอาไว้จัดกลุ่มในตาราง — เทียบข้ามกลุ่มต้องบอกผู้อ่านเสมอว่าคนละประเภทกัน
# Dir  : สิ่งที่ผู้ใช้ได้ลงไปในเครื่องจริง ๆ ชี้ไฟล์เดียวก็ได้ถ้าแอปนั้นเป็นไฟล์เดียว
#        ห้ามชี้โฟลเดอร์ที่มีโปรไฟล์/แคชผู้ใช้ปน (เช่น Comet\User Data 1.2 GB) — นั่นไม่ใช่ขนาดติดตั้ง
$Apps = @(
  # ไม่ใช่ทั้งโฟลเดอร์ build — ในนั้นมีไฟล์ dev ค้างกับตัวติดตั้งปนอยู่ ที่ส่งถึงผู้ใช้จริงคือ exe ตัวเดียว
  @{ Name='Aetox';         Kind='desktop'; Cat='ผู้ช่วย AI'; Procs=@('aetox','msedgewebview2')
     CmdMatch='aetox'
     Exe="$PSScriptRoot\desktop\build\bin\aetox.exe"
     Dir="$PSScriptRoot\desktop\build\bin\aetox.exe" }

  # ── เอดิเตอร์/IDE ที่มี AI ในตัว ──
  @{ Name='Zed';           Kind='desktop'; Cat='IDE'; Procs=@('Zed','zed')
     Exe="$env:LOCALAPPDATA\Programs\Zed\Zed.exe"
     Dir="$env:LOCALAPPDATA\Programs\Zed" }

  @{ Name='Cursor';        Kind='desktop'; Cat='IDE'; Procs=@('Cursor')
     Exe="$env:LOCALAPPDATA\Programs\cursor\Cursor.exe"
     Dir="$env:LOCALAPPDATA\Programs\cursor" }

  @{ Name='Windsurf';      Kind='desktop'; Cat='IDE'; Procs=@('Windsurf')
     Exe="$env:LOCALAPPDATA\Programs\Windsurf\Windsurf.exe"
     Dir="$env:LOCALAPPDATA\Programs\Windsurf" }

  @{ Name='Antigravity';   Kind='desktop'; Cat='IDE'; Procs=@('Antigravity IDE','Antigravity')
     Exe="$env:LOCALAPPDATA\Programs\Antigravity IDE\Antigravity IDE.exe"
     Dir="$env:LOCALAPPDATA\Programs\Antigravity IDE" }

  @{ Name='VS Code';       Kind='desktop'; Cat='IDE'; Procs=@('Code')
     Exe="$env:LOCALAPPDATA\Programs\Microsoft VS Code\Code.exe"
     Dir="$env:LOCALAPPDATA\Programs\Microsoft VS Code" }

  # ── แอปเดสก์ท็อปสาย AI ที่ไม่ใช่ IDE — ใกล้เคียง Aetox ที่สุด ──
  @{ Name='Comet';         Kind='desktop'; Cat='ผู้ช่วย AI'; Procs=@('comet')
     Exe="$env:LOCALAPPDATA\Perplexity\Comet\Application\comet.exe"
     Dir="$env:LOCALAPPDATA\Perplexity\Comet\Application" }

  @{ Name='Qwen';          Kind='desktop'; Cat='ผู้ช่วย AI'; Procs=@('Qwen')
     Exe="$env:LOCALAPPDATA\Programs\Qwen\Qwen.exe"
     Dir="$env:LOCALAPPDATA\Programs\Qwen" }

  # ── กลุ่มพิมพ์คำสั่ง ไม่มีหน้าต่างให้จับเวลาเปิด ──
  # .local\bin มีเครื่องมืออื่นของเจ้าของเครื่องปนอยู่ (uv, python shim) นับเฉพาะ exe ของมันเอง
  @{ Name='Claude Code';   Kind='cli'; Cat='CLI'; Procs=@('claude')
     Dir="$env:USERPROFILE\.local\bin\claude.exe" }

  # Codex ลงได้สองทาง ขนาดไม่เท่ากัน — วัดทั้งคู่ไว้ อย่าเอามารวมกัน
  @{ Name='Codex (npm)';   Kind='cli'; Cat='CLI'; Procs=@('codex')
     Dir="$env:APPDATA\npm\node_modules\@openai" }

  @{ Name='Codex (app)';   Kind='cli'; Cat='CLI'; Procs=@('codex')
     Dir="$env:LOCALAPPDATA\OpenAI\Codex" }

  @{ Name='OpenCode';      Kind='cli'; Cat='CLI'; Procs=@('opencode')
     Dir="$env:APPDATA\npm\node_modules\opencode-ai" }

  @{ Name='Copilot CLI';   Kind='cli'; Cat='CLI'; Procs=@('copilot')
     Dir="$env:LOCALAPPDATA\copilot" }

  # ── คนละประเภท เก็บไว้อ้างอิงเฉย ๆ ห้ามเอาไปเทียบตรง ๆ กับ Aetox ──
  @{ Name='Ollama';        Kind='cli'; Cat='รันโมเดล'; Procs=@('ollama','ollama app')
     Dir="$env:LOCALAPPDATA\Programs\Ollama" }
)

if ($App) {
  $Apps = @($Apps | Where-Object { $_.Name -like "*$App*" })
  if (-not $Apps) { throw "ไม่รู้จักแอปชื่อ '$App'" }
}

# ── ตัวช่วย ──────────────────────────────────────────────────────────────────

function Median($values) {
  $s = @($values | Where-Object { $null -ne $_ } | Sort-Object)
  if ($s.Count -eq 0) { return $null }
  if ($s.Count % 2) { return $s[[math]::Floor($s.Count / 2)] }
  return [math]::Round(($s[$s.Count/2 - 1] + $s[$s.Count/2]) / 2, 1)
}

function Get-PidSet($names) {
  $h = @{}
  foreach ($n in $names) {
    foreach ($p in @(Get-Process $n -ErrorAction SilentlyContinue)) { $h[$p.Id] = $true }
  }
  $h
}

# นับเฉพาะ process ที่ "เกิดใหม่" หลัง $exclude — Electron/WebView2 ตัดสายพ่อลูกทิ้ง
# ตอน launcher จบ การไล่ parent จึงเก็บไม่ครบ ใช้วิธีเทียบก่อน/หลังแทน
function Measure-Procs($app, $exclude) {
  $priv = 0; $ws = 0; $n = 0

  # msedgewebview2.exe เป็นชื่อกลางที่ทุกแอป WebView2 บนเครื่องใช้ร่วมกัน ถ้าไม่กรอง
  # จะนับ process ของแอปอื่นมาเป็นของเรา — โหมดที่ไม่มี baseline ก่อน/หลังต้องพึ่งข้อนี้
  $ok = $null
  if ($app.CmdMatch) {
    $ok = @{}
    foreach ($ci in @(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue)) {
      if ($app.Procs -notcontains ($ci.Name -replace '\.exe$','')) { continue }
      if ($ci.CommandLine -and $ci.CommandLine -match $app.CmdMatch) { $ok[[int]$ci.ProcessId] = $true }
    }
  }

  foreach ($nm in $app.Procs) {
    foreach ($p in @(Get-Process $nm -ErrorAction SilentlyContinue)) {
      if ($exclude -and $exclude.ContainsKey($p.Id)) { continue }
      if ($null -ne $ok -and -not $ok.ContainsKey($p.Id)) { continue }
      try { $p.Refresh(); $priv += $p.PrivateMemorySize64; $ws += $p.WorkingSet64; $n++ } catch {}
    }
  }
  # private bytes คือตัวหลัก — working set ขึ้นลงตาม RAM ที่เครื่องเหลือ เทียบข้ามเครื่องไม่ได้
  [pscustomobject]@{ Procs=$n; PrivateMB=[math]::Round($priv/1MB,0); WorkingSetMB=[math]::Round($ws/1MB,0) }
}

# "เปิดเสร็จ" = หน้าต่างแรกโผล่บนจอ — ต้องมองหา process ที่เกิดใหม่ด้วย ไม่ใช่แค่ตัวที่สั่งเปิด
# เพราะ Cursor / Windsurf / Antigravity เป็น VS Code fork ที่ launcher โยนงานให้ตัวอื่นแล้วจบตัวเองทันที
function Wait-Window($proc, $names, $before, $sw, $timeoutMs) {
  while ($sw.ElapsedMilliseconds -lt $timeoutMs) {
    if ($proc) {
      try { if (-not $proc.HasExited) { $proc.Refresh(); if ($proc.MainWindowHandle -ne 0) { return $sw.ElapsedMilliseconds } } } catch {}
    }
    foreach ($nm in $names) {
      foreach ($q in @(Get-Process $nm -ErrorAction SilentlyContinue)) {
        if ($before.ContainsKey($q.Id)) { continue }
        try { if ($q.MainWindowHandle -ne 0) { return $sw.ElapsedMilliseconds } } catch {}
      }
    }
    Start-Sleep -Milliseconds 20
  }
  $null
}

# ปิดแบบกดกากบาทก่อน ค่อยบังคับปิดตัวที่ยังเหลือ — ฆ่าดิบ ๆ ทุกรอบจะทำให้รอบถัดไป
# เจอหน้าต่าง "กู้คืนเซสชันที่ปิดผิดปกติ" ของพวก VS Code fork แล้วเวลาเปิดเพี้ยนทั้งชุด
function Stop-App($app, $before) {
  $mine = { @(Get-Process -Name $app.Procs -ErrorAction SilentlyContinue) |
            Where-Object { -not $before.ContainsKey($_.Id) } }
  foreach ($q in & $mine) { try { $null = $q.CloseMainWindow() } catch {} }
  Start-Sleep -Seconds 3
  foreach ($q in & $mine) { Stop-Process -Id $q.Id -Force -ErrorAction SilentlyContinue }
  Start-Sleep -Seconds 2
}

function Show-Machine {
  $cpu = Get-CimInstance Win32_Processor | Select-Object -First 1
  $cs  = Get-CimInstance Win32_ComputerSystem
  $os  = Get-CimInstance Win32_OperatingSystem
  $up  = (Get-Date) - $os.LastBootUpTime
  Write-Host ("เครื่อง : {0} · RAM {1} GB · {2} build {3}" -f `
    $cpu.Name.Trim(), [math]::Round($cs.TotalPhysicalMemory/1GB,0), $os.Caption.Trim(), $os.BuildNumber)
  Write-Host ("บูตมาแล้ว : {0} ชม. {1} นาที" -f [int]$up.TotalHours, $up.Minutes)
  Write-Host ""
}

# ── โหมด: ขนาดในเครื่อง ──────────────────────────────────────────────────────
function Invoke-Disk {
  Write-Host "── พื้นที่ที่กินในเครื่องหลังติดตั้ง ──`n"
  $rows = foreach ($a in $Apps) {
    if (-not (Test-Path $a.Dir)) { Write-Host ("{0,-12} ข้าม — ไม่พบ {1}" -f $a.Name, $a.Dir); continue }
    $item = Get-Item $a.Dir -Force
    $b = if ($item.PSIsContainer) {
      (Get-ChildItem $a.Dir -Recurse -File -Force -ErrorAction SilentlyContinue | Measure-Object Length -Sum).Sum
    } else { $item.Length }   # แอปไฟล์เดียวก็ชี้มาที่ไฟล์ได้เลย
    [pscustomobject]@{ App=$a.Name; Cat=$a.Cat; MB=[math]::Round($b/1MB,0)
                       'เท่าของ Aetox'=''; Path=$a.Dir }
  }
  $base = ($rows | Where-Object App -eq 'Aetox').MB
  foreach ($r in $rows) { if ($base) { $r.'เท่าของ Aetox' = if ($r.MB -eq $base) { '—' } else { '{0}×' -f [math]::Round($r.MB/$base,0) } } }
  $rows | Sort-Object MB | Format-Table App, Cat, MB, 'เท่าของ Aetox', Path -AutoSize
  Write-Host "หมายเหตุ: เป็นขนาดของโฟลเดอร์ติดตั้ง ไม่รวมแคช/โมเดล/ส่วนเสริมที่โหลดทีหลัง`n"
}

# ── โหมด: เวลาเปิด + RAM ตอนนิ่ง ─────────────────────────────────────────────
function Invoke-Start {
  $os = Get-CimInstance Win32_OperatingSystem
  $upH = ((Get-Date) - $os.LastBootUpTime).TotalHours
  if ($upH -gt 0.25) {
    Write-Host "⚠ เครื่องบูตมานานแล้ว — รอบแรกจะไม่ใช่ cold จริง เพราะไฟล์ยังค้างใน file cache" -ForegroundColor Yellow
    Write-Host "  ถ้าอยากได้ตัวเลข cold: รีบูตก่อน แล้วรัน .\bench.ps1 -Start -Runs 1`n" -ForegroundColor Yellow
  }

  foreach ($a in $Apps) {
    if ($a.Kind -ne 'desktop') { Write-Host ("{0,-12} ข้าม — ไม่มีหน้าต่าง จับเวลาเปิดไม่ได้" -f $a.Name); continue }
    if (-not (Test-Path $a.Exe)) { Write-Host ("{0,-12} ข้าม — ไม่พบ {1}" -f $a.Name, $a.Exe); continue }

    $running = Get-PidSet $a.Procs
    if ($running.Count -gt 0) {
      Write-Host ("{0,-12} ข้าม — เปิดค้างอยู่ ({1} process) ปิดก่อนแล้วรันใหม่ หรือใช้ -Snapshot" -f $a.Name, $running.Count) -ForegroundColor Yellow
      continue
    }

    Write-Host "`n── $($a.Name) ──"
    $ms=@(); $priv=@(); $ws=@(); $np=@()

    for ($i=1; $i -le $Runs; $i++) {
      $before = Get-PidSet $a.Procs
      $sw = [System.Diagnostics.Stopwatch]::StartNew()
      $p  = Start-Process -FilePath $a.Exe -PassThru
      $shown = Wait-Window $p $a.Procs $before $sw 60000
      $sw.Stop()

      if ($null -eq $shown) {
        Write-Host ("  รอบ {0}: ไม่มีหน้าต่างโผล่ใน 60 วิ — ข้ามตัวนี้ ลองเปิดเองแล้วใช้ -Snapshot แทน" -f $i) -ForegroundColor Yellow
        Stop-App $a $before
        break
      }

      Start-Sleep -Seconds $SettleSec
      $m = Measure-Procs $a $before
      $ms += $shown; $priv += $m.PrivateMB; $ws += $m.WorkingSetMB; $np += $m.Procs
      Write-Host ("  รอบ {0}: {1,5} ms · {2} process · private {3} MB · WS {4} MB" -f $i, $shown, $m.Procs, $m.PrivateMB, $m.WorkingSetMB)

      Stop-App $a $before   # ปิดเฉพาะที่รอบนี้เปิดขึ้นมา ไม่ไปยุ่งกับของเดิมบนเครื่อง
    }

    if ($ms.Count -eq 0) { continue }
    Write-Host ("  → รอบแรก {0} ms · median รอบถัดไป {1} ms · RAM {2} MB private · {3} process" -f `
      $ms[0], (Median ($ms | Select-Object -Skip 1)), (Median ($priv | Select-Object -Skip 1)), (Median $np)) -ForegroundColor Green
  }
  Write-Host ""
}

# ── โหมด: อ่านของที่เปิดค้างอยู่ ──────────────────────────────────────────────
function Invoke-Snapshot {
  Write-Host "── RAM ของตัวที่เปิดค้างอยู่ตอนนี้ (ไม่แตะอะไรทั้งนั้น) ──`n"
  $rows = foreach ($a in $Apps) {
    $m = Measure-Procs $a $null
    if ($m.Procs -eq 0) { continue }
    [pscustomobject]@{ App=$a.Name; Procs=$m.Procs; PrivateMB=$m.PrivateMB; WorkingSetMB=$m.WorkingSetMB }
  }
  if (-not $rows) { Write-Host "ไม่มีตัวไหนเปิดอยู่เลย"; return }
  $rows | Sort-Object PrivateMB | Format-Table -AutoSize
  Write-Host "⚠ ตัวเลขจากโหมดนี้เทียบกันตรง ๆ ไม่ได้ — แต่ละตัวเปิดมานานไม่เท่ากันและทำงานคนละอย่าง" -ForegroundColor Yellow
  Write-Host "  จดสภาพตอนวัดลง BENCHMARK.md ทุกครั้ง ไม่งั้นตัวเลขไม่มีความหมาย`n" -ForegroundColor Yellow
}

# ── โหมด: เปิดทิ้งไว้นาน ๆ หา memory leak ────────────────────────────────────
function Invoke-Soak {
  $log = Join-Path $PSScriptRoot 'logs'
  if (-not (Test-Path $log)) { New-Item -ItemType Directory $log | Out-Null }
  $csv = Join-Path $log ('soak-{0}.csv' -f (Get-Date -Format 'yyyyMMdd-HHmm'))

  $targets = @($Apps | Where-Object { (Measure-Procs $_ $null).Procs -gt 0 })
  if (-not $targets) { throw "ยังไม่มีแอปไหนเปิดอยู่ — เปิดตัวที่จะวัดก่อนแล้วค่อยรันโหมดนี้" }

  Write-Host ("จะเก็บทุก {0} วิ เป็นเวลา {1} ชม. → {2}" -f $EverySec, $Hours, $csv)
  Write-Host ("ตัวที่กำลังวัด: {0}`n" -f (($targets.Name) -join ', '))
  Write-Host "ปล่อยหน้าต่างเปิดทิ้งไว้ ใช้งานตามปกติได้ · Ctrl+C เพื่อหยุดก่อนเวลา`n"

  $deadline = (Get-Date).AddHours($Hours)
  $first = @{}
  while ((Get-Date) -lt $deadline) {
    $t = Get-Date
    foreach ($a in $targets) {
      $m = Measure-Procs $a $null
      if (-not $first.ContainsKey($a.Name) -and $m.Procs -gt 0) { $first[$a.Name] = $m.PrivateMB }
      $drift = if ($first.ContainsKey($a.Name)) { $m.PrivateMB - $first[$a.Name] } else { 0 }
      [pscustomobject]@{
        Time=$t.ToString('s'); App=$a.Name; Procs=$m.Procs
        PrivateMB=$m.PrivateMB; WorkingSetMB=$m.WorkingSetMB; DriftMB=$drift
      } | Export-Csv $csv -Append -NoTypeInformation
      # process หายไป = แอปตายระหว่างทาง ซึ่งคือผลลัพธ์ของการทดสอบความเสถียร ไม่ใช่ error
      $flag = if ($m.Procs -eq 0) { '  ← ไม่เหลือ process แล้ว (แอปดับ?)' } else { '' }
      Write-Host ("{0:HH:mm}  {1,-12} {2,5} MB  ({3,+5} MB จากตอนเริ่ม){4}" -f $t, $a.Name, $m.PrivateMB, $drift, $flag)
    }
    Start-Sleep -Seconds $EverySec
  }

  Write-Host "`n── สรุป ──"
  Import-Csv $csv | Group-Object App | ForEach-Object {
    $v = @($_.Group.PrivateMB | ForEach-Object { [int]$_ })
    Write-Host ("{0,-12} เริ่ม {1} MB → จบ {2} MB · สูงสุด {3} MB · โต {4} MB" -f `
      $_.Name, $v[0], $v[-1], ($v | Measure-Object -Maximum).Maximum, ($v[-1] - $v[0]))
  }
  Write-Host "`nโตขึ้นเรื่อย ๆ ไม่ลงเลย = น่าจะรั่ว · ขึ้นแล้วลง = แค่ GC ทำงานตามปกติ"
  Write-Host "ไฟล์ดิบ: $csv`n"
}

# ── ทางเข้า ─────────────────────────────────────────────────────────────────
Show-Machine
if     ($Disk)     { Invoke-Disk }
elseif ($Start)    { Invoke-Start }
elseif ($Snapshot) { Invoke-Snapshot }
elseif ($Soak)     { Invoke-Soak }
else {
  Write-Host "เลือกโหมด: -Disk | -Start | -Snapshot | -Soak"
  Write-Host "กติกาและผลที่วัดได้แล้วอยู่ใน BENCHMARK.md`n"
}
