param(
  [Parameter(Mandatory = $true)]
  [string]$Reference,
  [Parameter(Mandatory = $true)]
  [string]$Candidate,
  [Parameter(Mandatory = $true)]
  [string]$Diff,
  [int]$X = 0,
  [int]$Y = 0,
  [int]$Width = 0,
  [int]$Height = 0
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not ('VisualRegressionDiffEngine' -as [type])) {
  Add-Type -ReferencedAssemblies @(
    (Join-Path $PSHOME 'System.Drawing.Common.dll'),
    (Join-Path $PSHOME 'System.Drawing.Primitives.dll'),
    (Join-Path $PSHOME 'System.Private.Windows.GdiPlus.dll'),
    (Join-Path $PSHOME 'System.Private.Windows.Core.dll')
  ) -TypeDefinition @'
using System;
using System.Drawing;
using System.Drawing.Imaging;

public sealed class VisualRegressionDiffResult
{
    public int ReferenceWidth { get; set; }
    public int ReferenceHeight { get; set; }
    public long TotalPixels { get; set; }
    public long DifferentPixels { get; set; }
    public long TotalChannelDelta { get; set; }
}

public static class VisualRegressionDiffEngine
{
    public static VisualRegressionDiffResult Compare(string referencePath, string candidatePath, string diffPath, int x, int y, int width, int height)
    {
        using (var reference = new Bitmap(referencePath))
        using (var candidate = new Bitmap(candidatePath))
        {
            if (reference.Width != candidate.Width || reference.Height != candidate.Height)
            {
                throw new InvalidOperationException(string.Format("截图尺寸不一致：参考 {0}x{1}，候选 {2}x{3}", reference.Width, reference.Height, candidate.Width, candidate.Height));
            }
            if (x < 0 || y < 0 || width <= 0 || height <= 0 || x + width > reference.Width || y + height > reference.Height)
            {
                throw new InvalidOperationException(string.Format("差异区域越界：x={0}, y={1}, width={2}, height={3}", x, y, width, height));
            }

            var result = new VisualRegressionDiffResult
            {
                ReferenceWidth = reference.Width,
                ReferenceHeight = reference.Height,
                TotalPixels = (long)width * height
            };

            using (var difference = new Bitmap(width, height, PixelFormat.Format24bppRgb))
            {
                for (var row = 0; row < height; row++)
                {
                    for (var column = 0; column < width; column++)
                    {
                        var referencePixel = reference.GetPixel(x + column, y + row);
                        var candidatePixel = candidate.GetPixel(x + column, y + row);
                        var delta = Math.Abs(referencePixel.R - candidatePixel.R) + Math.Abs(referencePixel.G - candidatePixel.G) + Math.Abs(referencePixel.B - candidatePixel.B);
                        result.TotalChannelDelta += delta;
                        if (delta > 0) result.DifferentPixels++;
                        difference.SetPixel(column, row, Color.FromArgb(Math.Min(255, delta), 0, 0));
                    }
                }
                difference.Save(diffPath, ImageFormat.Png);
            }
            return result;
        }
    }
}
'@
}

$referencePath = [System.IO.Path]::GetFullPath($Reference)
$candidatePath = [System.IO.Path]::GetFullPath($Candidate)
$diffPath = [System.IO.Path]::GetFullPath($Diff)

if (-not (Test-Path -LiteralPath $referencePath -PathType Leaf)) { throw "未找到参考截图：$referencePath" }
if (-not (Test-Path -LiteralPath $candidatePath -PathType Leaf)) { throw "未找到候选截图：$candidatePath" }

$referenceProbe = [System.Drawing.Bitmap]::new($referencePath)
try {
  if ($Width -eq 0) { $Width = $referenceProbe.Width - $X }
  if ($Height -eq 0) { $Height = $referenceProbe.Height - $Y }
} finally {
  $referenceProbe.Dispose()
}

$diffDirectory = Split-Path -Parent $diffPath
if (-not (Test-Path -LiteralPath $diffDirectory)) { New-Item -ItemType Directory -Path $diffDirectory | Out-Null }
$result = [VisualRegressionDiffEngine]::Compare($referencePath, $candidatePath, $diffPath, $X, $Y, $Width, $Height)
[pscustomobject]@{
  reference = $referencePath
  candidate = $candidatePath
  diff = $diffPath
  dimensions = "$($result.ReferenceWidth)x$($result.ReferenceHeight)"
  crop = "$X,$Y,$Width,$Height"
  totalPixels = $result.TotalPixels
  differentPixels = $result.DifferentPixels
  differentPixelRatio = [Math]::Round($result.DifferentPixels / [double]$result.TotalPixels, 8)
  meanAbsoluteChannelDelta = [Math]::Round($result.TotalChannelDelta / ([double]$result.TotalPixels * 3), 8)
} | ConvertTo-Json -Compress
