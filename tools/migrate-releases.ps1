# Preview by default; -Apply creates version tags and retargets existing releases.
param([switch]$Apply)

$ErrorActionPreference = 'Stop'
$repository = 'Losobie/evela'

function Invoke-GitHub {
    $result = gh @args
    if ($LASTEXITCODE -ne 0) { throw "GitHub command failed: gh $args" }
    return $result
}

if ((git rev-parse --is-shallow-repository) -ne 'false' -or $LASTEXITCODE -ne 0) {
    throw 'Run this script from a full Evela checkout. Fetch missing history first.'
}
$pages = Invoke-GitHub api "repos/$repository/releases?per_page=100" --paginate --slurp | ConvertFrom-Json
$releases = @($pages | ForEach-Object { $_ })
$tags = @(Invoke-GitHub api "repos/$repository/git/matching-refs/tags/" | ConvertFrom-Json)
$plan = @(
    foreach ($release in $releases) {
        if ($release.draft -or $release.prerelease -or $release.tag_name -notmatch '^build-([a-fA-F0-9]{40})$') { continue }
        $revision = $Matches[1]
        if ($release.immutable) { throw "Release $($release.tag_name) is immutable and cannot be retagged." }
        $count = git rev-list --count $revision
        if ($LASTEXITCODE -ne 0) { throw "Fetch the history containing $revision first." }
        $tag = "v0.1.$($count.Trim())"
        if ($releases.tag_name -contains $tag) { throw "Release $tag already exists; resolve the duplicate before migrating." }
        $exists = @($tags | Where-Object { $_.ref -eq "refs/tags/$tag" }).Count -gt 0
        if ($exists) {
            $target = Invoke-GitHub api "repos/$repository/commits/$tag" --jq '.sha'
            if ($target.Trim() -ne $revision) { throw "Tag $tag already identifies another commit." }
        }
        [PSCustomObject]@{OldTag=$release.tag_name; Tag=$tag; Revision=$revision; CreateTag=(-not $exists)}
    }
)
if ($plan.Count -eq 0) {
    Write-Host 'No legacy releases to migrate.'
    if (-not $Apply) { return }
}
$plan = @($plan | Sort-Object { [version]$_.Tag.Substring(1) })
$plan | Format-Table OldTag, Tag
if (-not $Apply) { Write-Host 'Preview only. Run again with -Apply to migrate these releases.'; return }

foreach ($item in $plan) {
    if ($item.CreateTag) {
        Invoke-GitHub api "repos/$repository/git/refs" --method POST -f "ref=refs/tags/$($item.Tag)" -f "sha=$($item.Revision)" | Out-Null
    }
    # Editing the existing release preserves its notes, assets and download counts.
    # Retain the old Git tag as a source-history alias.
    Invoke-GitHub release edit $item.OldTag --repo $repository --tag $item.Tag --verify-tag --title "Evela $($item.Tag)" --latest=false
}
$pages = Invoke-GitHub api "repos/$repository/releases?per_page=100" --paginate --slurp | ConvertFrom-Json
$releases = @($pages | ForEach-Object { $_ })
$versions = @($plan.Tag) + @($releases | Where-Object { -not $_.draft -and -not $_.prerelease -and $_.tag_name -match '^v\d+\.\d+\.\d+$' } | ForEach-Object { $_.tag_name })
if ($versions.Count -eq 0) { return }
$latest = $versions | Sort-Object { [version]$_.Substring(1) } | Select-Object -Last 1
Invoke-GitHub release edit $latest --repo $repository --latest
