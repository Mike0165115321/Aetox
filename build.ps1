# clean old logs before build (ignore if files are locked)
if (Test-Path "logs") {
    Get-ChildItem "logs" -File | ForEach-Object {
        Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
    }
}
go build -o aetox.exe ./cmd/aetox
Write-Output "[$(Get-Date -Format HH:mm:ss)] build done, binary: $((Get-Item aetox.exe).Length) bytes"
