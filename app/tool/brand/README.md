# app/tool/brand

Source of the Athlix logo (the approved **"8 · Glass icon"** concept) and the scripts that turn it
into every app icon and splash image. The SVGs are a hand-drawn vector of the concept: white
three-legged "A" over a cyan-to-blue "X", on a frosted rounded square over navy with a teal glow.
If an original designer file exists, replace the SVGs with it and re-render.

| File | Used for |
|---|---|
| `athlix_icon.svg` | full-bleed opaque icon: iOS AppIcon (all sizes), Play Store |
| `athlix_icon_rounded.svg` | legacy Android launcher `mipmap-*/ic_launcher.png`, in-app logo `assets/brand/athlix_icon.png` |
| `athlix_adaptive_foreground.svg` / `_background.svg` | Android 8+ adaptive icon layers (`mipmap-*/ic_launcher_foreground|background.png`) |
| `athlix_splash.svg` | Android launch bitmap `drawable-*/splash_logo.png`, iOS `LaunchImage` |
| `athlix_splash_v31.svg` | Android 12+ splash icon `drawable-xxxhdpi/splash_icon_v31.png` (fits the 160dp circle mask) |
| `athlix_mark.svg` | the mark alone on transparent (spare, for print/web) |
| `play_store_icon_512.png` | Play Console listing icon |

## Regenerate

Needs Node 20+ and Google Chrome (rendering is done by headless Chrome, so no npm packages).

```bash
cd app/tool/brand
node make_svgs.mjs .                       # rewrites the SVGs from the geometry in make_svgs.mjs
node render.mjs athlix_icon.svg:/tmp/icon_1024.png:1024   # <svg>:<png>:<size>, any number of jobs
```

Then resize with `sips -z <px> <px>` into the files listed above. iOS icons must have **no alpha
channel** (App Store rejects them): round-trip through JPEG (`sips -s format jpeg -s formatOptions 100`)
before writing the PNGs. Sizes: Android launcher 48/72/96/144/192, adaptive layers 108/162/216/324/432,
splash logo 120/180/240/360/480 (mdpi to xxxhdpi); iOS sizes are in `AppIcon.appiconset/Contents.json`.
