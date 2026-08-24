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

# Read the biggest frame out of the ICONDIR by hand rather than asking
# [System.Drawing.Icon] for it. Asked for 256x256 it hands back the 128x128
# one: the 256 frame in this file is PNG-compressed, and GDI+'s icon reader
# quietly prefers a frame it understands over the one that was requested.
function Read-LargestFrame([string]$path) {
    $bytes = [System.IO.File]::ReadAllBytes($path)
    if ([BitConverter]::ToUInt16($bytes, 2) -ne 1) { throw "$path is not an .ico" }
    $count = [BitConverter]::ToUInt16($bytes, 4)

    $best = $null
    for ($i = 0; $i -lt $count; $i++) {
        $o = 6 + ($i * 16)
        # 0 in the width/height byte means 256 -- the field is one byte wide.
        $w = if ($bytes[$o] -eq 0) { 256 } else { [int]$bytes[$o] }
        $h = if ($bytes[$o + 1] -eq 0) { 256 } else { [int]$bytes[$o + 1] }
        $entry = @{
            w      = $w
            h      = $h
            size   = [BitConverter]::ToUInt32($bytes, $o + 8)
            offset = [BitConverter]::ToUInt32($bytes, $o + 12)
        }
        if ($null -eq $best -or ($w * $h) -gt ($best.w * $best.h)) { $best = $entry }
    }

    $blob = New-Object byte[] $best.size
    [Array]::Copy($bytes, $best.offset, $blob, 0, $best.size)

    # A PNG-compressed frame is a whole PNG file sitting inside the .ico, so it
    # loads directly. A BMP-compressed one does not, and rebuilding its AND
    # mask by hand is not worth it while every icon this repo ships is PNG.
    $png = 0x89, 0x50, 0x4E, 0x47
    for ($i = 0; $i -lt 4; $i++) {
        if ($blob[$i] -ne $png[$i]) {
            throw "the $($best.w)x$($best.h) frame is BMP-compressed, which this script cannot read"
        }
    }

    $ms = New-Object System.IO.MemoryStream(, $blob)
    return [System.Drawing.Bitmap]::FromStream($ms)
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

$src = Read-LargestFrame (Resolve-Path $ico)
try {
    if ((Get-Ink $src) -eq 0) { throw "the source frame is empty" }
    Write-Host ("source frame {0}x{1}" -f $src.Width, $src.Height)

    $sizes = @(
        @{ name = "Square44x44Logo.png";   w = 44;  h = 44 },
        @{ name = "Square71x71Logo.png";   w = 71;  h = 71 },
        @{ name = "Square150x150Logo.png"; w = 150; h = 150 },
        @{ name = "Square310x310Logo.png"; w = 310; h = 310 },
        @{ name = "Wide310x150Logo.png";   w = 310; h = 150 },
        @{ name = "StoreLogo.png";         w = 50;  h = 50 }
    )

    foreach ($s in $sizes) {
        $out = Join-Path $outDir $s.name
        $dst = New-Object System.Drawing.Bitmap($s.w, $s.h, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $g = [System.Drawing.Graphics]::FromImage($dst)
        try {
            $g.Clear([System.Drawing.Color]::Transparent)
            $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality

            # Drawn edge to edge on the square tiles: the icon carries its own
            # rounded square and its own margin already, and adding a second
            # margin around it only makes the mark smaller than every other
            # app's. The wide tile letterboxes rather than stretches, so the
            # square stays square.
            $scale = [Math]::Min($s.w / $src.Width, $s.h / $src.Height)
            $w = [Math]::Max(1, [int][Math]::Round($src.Width * $scale))
            $h = [Math]::Max(1, [int][Math]::Round($src.Height * $scale))
            $g.DrawImage($src, [int](($s.w - $w) / 2), [int](($s.h - $h) / 2), $w, $h)
        } finally {
            $g.Dispose()
        }

        $ink = Get-Ink $dst
        $dst.Save($out, [System.Drawing.Imaging.ImageFormat]::Png)
        $dst.Dispose()

        if ($ink -eq 0) { throw "$($s.name) came out empty" }
        "{0,-24} {1}x{2}  {3:N0} bytes  {4} opaque samples" -f $s.name, $s.w, $s.h, (Get-Item $out).Length, $ink
    }
} finally {
    $src.Dispose()
}
