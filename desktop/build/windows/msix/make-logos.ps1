# Renders the Microsoft Store logo set from desktop/build/windows/icon.ico.
#
# The PNGs it writes are committed next to it, so neither CI nor a release
# needs to run anything: this script exists to make them reproducible, not to
# run on every build. Re-run it when the app icon changes.
#
#   pwsh desktop/build/windows/msix/make-logos.ps1
#
# The source is icon.ico rather than assets/logo.svg, and that is the whole
# point. logo.svg is the bare mark: a white fill with a 10-unit stroke on an
# 820-unit canvas, which comes out as a hairline outline at 44x44 and is
# unreadable on a taskbar. icon.ico is the same mark as it was actually
# designed to be seen -- white on a black rounded square -- and it is what
# Windows already shows for this app everywhere else. Two icons for one app
# that disagree at small sizes is not a thing worth building on purpose.
#
# ---------------------------------------------------------------------------
# Why there are 50 files here and not 6 (2026-08-29, DECISIONS §207)
#
# The owner put the Store build and the installer build side by side on one
# taskbar: "ไอที่ขอบแดงๆพังๆอ่ะ คือโปรแกรมเราที่โหลด จาก ไมโครซอฟสโตร ครับ แต่
# พอโหลดของเราตรงๆกลับไม่เป็น". Same app, same mark, and only the Store copy
# had a frayed edge.
#
# icon.ico carries nine hand-sized frames -- 16, 20, 24, 32, 40, 48, 64, 128,
# 256 -- so the installed build hands Windows the exact pixel size the taskbar
# asks for and Windows resamples nothing. The Store build shipped ONE 44x44
# PNG. Every taskbar size (24 and 32 logical, 30/36/40/48 at the DPI scales
# people actually run) was Windows downscaling that 44 by a non-integer factor,
# through a mark whose outline is about one pixel wide. That is the fringe.
#
# So this writes what the .ico already knows:
#
#   * targetsize-N       the exact icon sizes Windows asks for. Taskbar, Alt-
#                        Tab, Start's list, the jump list. Where the .ico has
#                        a frame of exactly that size it is copied out
#                        untouched -- the hand-tuned pixels, not a rescale of
#                        them.
#   * _altform-unplated  the same size again, for the surfaces that draw no
#                        plate behind the icon. The taskbar is one of them, so
#                        this is not an optional nicety.
#   * scale-N            the tiles at each DPI. 44x44 at 150% is 66 real
#                        pixels, and shipping only the 100% asset means
#                        Windows invents the other four.
#
# NONE of this resolves without a resources.pri in the package: with no PRI
# index Windows falls back to the literal path in AppxManifest.xml and reads
# the one unqualified file. .github/workflows/release.yml runs makepri
# immediately before makeappx for exactly that reason, and the unqualified
# files below are kept as the fallback for the day somebody forgets.
#
# Sizes come from the Store's app-icon requirements. Square44x44 is the taskbar
# and Start list icon, Square150x150 the default tile, StoreLogo the one
# Package/Properties/Logo names, and the rest are the tiles Windows falls back
# to when a user resizes.

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

$ico = Join-Path $PSScriptRoot "..\icon.ico"
$outDir = Join-Path $PSScriptRoot "Assets"

if (-not (Test-Path $ico)) { throw "app icon not found at $ico" }
New-Item -ItemType Directory -Force $outDir | Out-Null

# Read every frame out of the ICONDIR by hand rather than asking
# [System.Drawing.Icon] for one. Asked for 256x256 it hands back the 128x128
# one: the 256 frame in this file is PNG-compressed, and GDI+'s icon reader
# quietly prefers a frame it understands over the one that was requested.
function Read-Frames([string]$path) {
    $bytes = [System.IO.File]::ReadAllBytes($path)
    if ([BitConverter]::ToUInt16($bytes, 2) -ne 1) { throw "$path is not an .ico" }
    $count = [BitConverter]::ToUInt16($bytes, 4)

    $frames = @{}
    for ($i = 0; $i -lt $count; $i++) {
        $o = 6 + ($i * 16)
        # 0 in the width/height byte means 256 -- the field is one byte wide.
        $w = if ($bytes[$o] -eq 0) { 256 } else { [int]$bytes[$o] }
        $h = if ($bytes[$o + 1] -eq 0) { 256 } else { [int]$bytes[$o + 1] }
        if ($w -ne $h) { continue } # nothing in this repo ships a non-square frame
        $size = [BitConverter]::ToUInt32($bytes, $o + 8)
        $offset = [BitConverter]::ToUInt32($bytes, $o + 12)

        $blob = New-Object byte[] $size
        [Array]::Copy($bytes, $offset, $blob, 0, $size)

        # A PNG-compressed frame is a whole PNG file sitting inside the .ico, so
        # it loads directly. A BMP-compressed one does not, and rebuilding its
        # AND mask by hand is not worth it while every icon this repo ships is
        # PNG -- but skipping it silently would leave a size looking absent when
        # it is merely unreadable, so it says so.
        $png = 0x89, 0x50, 0x4E, 0x47
        $isPng = $true
        for ($b = 0; $b -lt 4; $b++) { if ($blob[$b] -ne $png[$b]) { $isPng = $false } }
        if (-not $isPng) {
            Write-Warning "the ${w}x${h} frame is BMP-compressed and was skipped"
            continue
        }

        $ms = New-Object System.IO.MemoryStream(, $blob)
        $frames[$w] = [System.Drawing.Bitmap]::FromStream($ms)
    }
    if ($frames.Count -eq 0) { throw "$path carried no readable frame" }
    return $frames
}

# The frame to draw a given output from: the exact size when the .ico has one,
# otherwise the smallest frame larger than it, otherwise the largest there is.
#
# Nearest-larger rather than always-largest is the half of this that matters at
# small sizes. Going 256 -> 24 throws away 99% of the pixels in one step and
# fringes the outline; 32 -> 24 barely touches it.
function Select-Source([hashtable]$frames, [int]$want) {
    if ($frames.ContainsKey($want)) { return $frames[$want] }
    $larger = $frames.Keys | Where-Object { $_ -gt $want } | Sort-Object
    if ($larger) { return $frames[$larger[0]] }
    $all = $frames.Keys | Sort-Object -Descending
    return $frames[$all[0]]
}

# Counts pixels that are not fully transparent. Sampling every other row and
# column is plenty to tell "an icon" from "nothing at all", which is the only
# question being asked -- and it is a question worth asking, because an empty
# PNG of the right dimensions passes every other check there is.
function Get-Ink([System.Drawing.Bitmap]$bmp) {
    $n = 0
    for ($y = 0; $y -lt $bmp.Height; $y += 2) {
        for ($x = 0; $x -lt $bmp.Width; $x += 2) {
            if ($bmp.GetPixel($x, $y).A -gt 10) { $n++ }
        }
    }
    return $n
}

# Draws one output file. Square outputs whose source frame is already exactly
# that size are copied pixel for pixel -- resampling an image onto itself is
# not free, it softens edges that were drawn deliberately.
function Write-Logo([hashtable]$frames, [string]$path, [int]$w, [int]$h) {
    $src = Select-Source $frames ([Math]::Max($w, $h))
    $dst = New-Object System.Drawing.Bitmap($w, $h, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [System.Drawing.Graphics]::FromImage($dst)
    try {
        $g.Clear([System.Drawing.Color]::Transparent)
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality

        if ($src.Width -eq $w -and $src.Height -eq $h) {
            # Blitted, not resampled. DrawImage at 1:1 still runs the
            # interpolator, and running it over pixels somebody drew by hand at
            # exactly this size is the thing this whole file exists to stop.
            $g.DrawImageUnscaled($src, 0, 0)
        } else {
            # Drawn edge to edge on the square tiles: the icon carries its own
            # rounded square and its own margin already, and adding a second
            # margin around it only makes the mark smaller than every other
            # app's. The wide tile letterboxes rather than stretches, so the
            # square stays square.
            $scale = [Math]::Min($w / $src.Width, $h / $src.Height)
            $dw = [Math]::Max(1, [int][Math]::Round($src.Width * $scale))
            $dh = [Math]::Max(1, [int][Math]::Round($src.Height * $scale))
            $g.DrawImage($src, [int](($w - $dw) / 2), [int](($h - $dh) / 2), $dw, $dh)
        }
    } finally {
        $g.Dispose()
    }

    $ink = Get-Ink $dst
    $dst.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
    $dst.Dispose()
    if ($ink -eq 0) { throw "$(Split-Path $path -Leaf) came out empty" }
    return @{ ink = $ink; from = $src.Width }
}

$frames = Read-Frames (Resolve-Path $ico)
try {
    Write-Host ("icon.ico frames: {0}" -f (($frames.Keys | Sort-Object) -join ", "))

    # The tiles, at every DPI Windows asks for. scale-400 is included only on
    # the small square: it is 176px there and 1240x600 on the wide tile, which
    # is a megabyte of package for a size no Store surface requests.
    $tiles = @(
        @{ name = "Square44x44Logo";   w = 44;  h = 44;  scales = @(100, 125, 150, 200, 400) },
        @{ name = "Square71x71Logo";   w = 71;  h = 71;  scales = @(100, 125, 150, 200) },
        @{ name = "Square150x150Logo"; w = 150; h = 150; scales = @(100, 125, 150, 200) },
        @{ name = "Square310x310Logo"; w = 310; h = 310; scales = @(100, 125, 150, 200) },
        @{ name = "Wide310x150Logo";   w = 310; h = 150; scales = @(100, 125, 150, 200) },
        @{ name = "StoreLogo";         w = 50;  h = 50;  scales = @(100, 125, 150, 200) }
    )

    # The sizes Windows asks for by pixel rather than by tile. 24 and 32 are the
    # taskbar at 100%; 30, 36, 40 and 48 are the same taskbar at 125%, 150%,
    # 175% and 200%. The rest are Start, Alt-Tab, the jump list and search.
    $targetSizes = @(16, 20, 24, 30, 32, 36, 40, 48, 56, 60, 64, 72, 80, 96, 256)

    $written = 0
    foreach ($t in $tiles) {
        # The unqualified name the manifest points at, kept as the fallback for
        # a package built without a resources.pri.
        $r = Write-Logo $frames (Join-Path $outDir "$($t.name).png") $t.w $t.h
        "{0,-46} {1}x{2}  from {3}px  {4:N0} opaque samples" -f "$($t.name).png", $t.w, $t.h, $r.from, $r.ink
        $written++

        foreach ($s in $t.scales) {
            $w = [int][Math]::Round($t.w * $s / 100)
            $h = [int][Math]::Round($t.h * $s / 100)
            $name = "$($t.name).scale-$s.png"
            $null = Write-Logo $frames (Join-Path $outDir $name) $w $h
            $written++
        }
    }

    foreach ($n in $targetSizes) {
        foreach ($form in @("", "_altform-unplated")) {
            $name = "Square44x44Logo.targetsize-$n$form.png"
            $r = Write-Logo $frames (Join-Path $outDir $name) $n $n
            if ($form -eq "") {
                "{0,-46} {1}x{1}  from {2}px  {3:N0} opaque samples" -f $name, $n, $r.from, $r.ink
            }
            $written++
        }
    }

    "`n{0} files in {1}" -f $written, $outDir
} finally {
    foreach ($f in $frames.Values) { $f.Dispose() }
}
