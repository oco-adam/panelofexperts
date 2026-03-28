[CmdletBinding()]
param(
    [string]$Version = $env:POE_VERSION,
    [string]$InstallDir = $(if ($env:POE_INSTALL_DIR) { $env:POE_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\poe\bin" }),
    [string]$BaseUrl = $env:POE_BASE_URL,
    [switch]$Force,
    [switch]$NoPathUpdate
)

$ErrorActionPreference = "Stop"
$Repository = if ($env:POE_REPOSITORY) { $env:POE_REPOSITORY } else { "oco-adam/panelofexperts" }

function Resolve-Version {
    if ($Version) {
        return $Version
    }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repository/releases/latest"
    return [string]$release.tag_name
}

function Resolve-Arch {
    switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()) {
        "x64" { return "amd64" }
        "arm64" { return "arm64" }
        default { throw "Unsupported Windows architecture." }
    }
}

function Copy-FromSource {
    param(
        [Parameter(Mandatory = $true)][string]$Source,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    if ($Source.StartsWith("http://") -or $Source.StartsWith("https://")) {
        Invoke-WebRequest -Uri $Source -OutFile $Destination
        return
    }

    $resolved = if ($Source.StartsWith("file://")) { $Source.Substring(7) } else { $Source }
    Copy-Item -Path $resolved -Destination $Destination -Force
}

$Version = Resolve-Version
if (-not $Version) {
    throw "Unable to resolve a release version. Set POE_VERSION explicitly."
}

$Arch = Resolve-Arch
$AssetName = "poe_${Version}_windows_${Arch}.zip"
$ReleaseRoot = if ($BaseUrl) { $BaseUrl } else { "https://github.com/$Repository/releases/download/$Version" }

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("poe-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $TempDir | Out-Null
try {
    $ArchivePath = Join-Path $TempDir $AssetName
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"

    Copy-FromSource -Source "$ReleaseRoot/$AssetName" -Destination $ArchivePath
    Copy-FromSource -Source "$ReleaseRoot/checksums.txt" -Destination $ChecksumsPath

    $expected = Select-String -Path $ChecksumsPath -Pattern ([regex]::Escape($AssetName)) | ForEach-Object {
        ($_ -split '\s+')[0]
    } | Select-Object -First 1
    if (-not $expected) {
        throw "Checksum for $AssetName not found."
    }

    $actual = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected.ToLowerInvariant()) {
        throw "Checksum verification failed for $AssetName."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $TargetPath = Join-Path $InstallDir "poe.exe"
    $Existing = Get-Command poe -ErrorAction SilentlyContinue
    if ($Existing -and $Existing.Source -ne $TargetPath -and -not $Force) {
        throw "Found poe at $($Existing.Source), which does not match $TargetPath. Re-run with -Force to override."
    }

    $ExtractDir = Join-Path $TempDir "extract"
    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
    Copy-Item -Path (Join-Path $ExtractDir "poe.exe") -Destination $TargetPath -Force

    if (-not $NoPathUpdate) {
        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        $segments = @()
        if ($userPath) {
            $segments = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
        }
        if (-not ($segments -contains $InstallDir)) {
            $newPath = if ($userPath) { "$userPath;$InstallDir" } else { $InstallDir }
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        }
    }
    if (-not ($env:PATH.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries) -contains $InstallDir)) {
        $env:PATH = "$InstallDir;$env:PATH"
    }

    $homeDir = if ($env:POE_HOME) {
        $env:POE_HOME
    } else {
        Join-Path ([Environment]::GetFolderPath("ApplicationData")) "poe"
    }
    New-Item -ItemType Directory -Path $homeDir -Force | Out-Null
    @{
        version = $Version
        channel = "direct"
        installed_at = [DateTime]::UtcNow.ToString("yyyy-MM-ddTHH:mm:ssZ")
        install_path = $TargetPath
        source_url = "$ReleaseRoot/$AssetName"
        repository = $Repository
    } | ConvertTo-Json | Set-Content -Path (Join-Path $homeDir "install-receipt.json")

    Write-Host "Installed poe $Version to $TargetPath"
    if ($NoPathUpdate) {
        Write-Host "Add $InstallDir to PATH before opening a new shell."
    }
}
finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force
    }
}
