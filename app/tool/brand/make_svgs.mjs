// Builds the Athlix "glass icon" as SVG variants on a 1024 canvas.
// Geometry measured from the approved concept (8 · Glass icon).
import { writeFileSync } from 'node:fs';
const out = process.argv[2];

const defs = `
  <defs>
    <linearGradient id="bg" x1="0" y1="1" x2="1" y2="0">
      <stop offset="0" stop-color="#050C1A"/><stop offset="1" stop-color="#0A1E30"/>
    </linearGradient>
    <radialGradient id="glow" cx="0.82" cy="0.05" r="0.95">
      <stop offset="0" stop-color="#11788A" stop-opacity="1"/>
      <stop offset="0.4" stop-color="#0C4F63" stop-opacity="0.8"/>
      <stop offset="0.8" stop-color="#08253A" stop-opacity="0.35"/>
      <stop offset="1" stop-color="#050C1A" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="glass" x1="0" y1="0" x2="0.3" y2="1">
      <stop offset="0" stop-color="#0A2A3E" stop-opacity="0.9"/><stop offset="1" stop-color="#081A2B" stop-opacity="0.95"/>
    </linearGradient>
    <!-- The X is cyan on its left ends and blue on its right ends (measured from the concept). -->
    <linearGradient id="x" gradientUnits="userSpaceOnUse" x1="290" y1="0" x2="720" y2="0">
      <stop offset="0" stop-color="#00C8E4"/><stop offset="1" stop-color="#1696F0"/>
    </linearGradient>
  </defs>`;

// White "A": three legs (left, middle, right) meeting at one apex, over a cyan-to-blue "X".
const mark = `
  <g fill="none" stroke-linecap="round" stroke-linejoin="round">
    <line x1="369" y1="369" x2="646" y2="712" stroke="url(#x)" stroke-width="40"/>
    <line x1="720" y1="369" x2="290" y2="712" stroke="url(#x)" stroke-width="40"/>
    <path d="M326 698 L569 319 L609 698" stroke="#FFFFFF" stroke-width="48"/>
    <line x1="460" y1="698" x2="569" y2="319" stroke="#FFFFFF" stroke-width="48"/>
  </g>`;

const glass = `<rect x="125" y="123" width="773" height="773" rx="143" fill="url(#glass)" stroke="#FFFFFF" stroke-opacity="0.13" stroke-width="2.5"/>`;
const background = `<rect width="1024" height="1024" fill="url(#bg)"/><rect width="1024" height="1024" fill="url(#glow)"/>`;
const svg = (body, rx = 0) => `<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">${defs}
  ${rx ? `<clipPath id="r"><rect width="1024" height="1024" rx="${rx}"/></clipPath><g clip-path="url(#r)">` : '<g>'}${body}</g></svg>\n`;
// Scale content around the centre (adaptive icon safe zone / splash).
const scaled = (s, body) => `<g transform="translate(512 512) scale(${s}) translate(-512 -512)">${body}</g>`;

const files = {
  'athlix_icon.svg': svg(background + glass + mark),                 // full-bleed, opaque (iOS, Play Store)
  'athlix_icon_rounded.svg': svg(background + glass + mark, 184),     // legacy Android launcher
  'athlix_mark.svg': svg(scaled(1.25, mark)),                         // mark only, transparent (in-app, splash)
  'athlix_adaptive_foreground.svg': svg(scaled(0.56, glass + mark)),  // Android adaptive foreground (safe zone)
  'athlix_adaptive_background.svg': svg(background),                  // Android adaptive background
  'athlix_splash.svg': svg(scaled(0.62, glass + mark)),               // launch screens (transparent)
  'athlix_splash_v31.svg': svg(scaled(0.46, glass + mark)),           // Android 12+ splash (inside its 160dp circle mask)
};
for (const [name, body] of Object.entries(files)) writeFileSync(`${out}/${name}`, body);
console.log(Object.keys(files).join('\n'));
