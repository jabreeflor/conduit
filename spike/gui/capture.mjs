import { chromium } from "playwright";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const OUT = resolve(__dirname, "../../docs/design-system/screenshots");
const BASE = "http://localhost:1420";

const shots = [
  { url: `${BASE}/`, file: "01-welcome.png" },
  { url: `${BASE}/?demo=chat`, file: "02-chat.png" },
  { url: `${BASE}/?demo=spotlight`, file: "03-spotlight.png" },
  { url: `${BASE}/?demo=projects`, file: "04-projects.png" },
  { url: `${BASE}/?demo=new-project`, file: "05-new-project.png", height: 1850 },
  { url: `${BASE}/?demo=workspace`, file: "06-project-dashboard.png" },
  { url: `${BASE}/`, file: "07-welcome-dark.png", theme: "dark" },
  { url: `${BASE}/?demo=soul`, file: "09-soul-page.png", height: 1100 },
  { url: `${BASE}/?demo=settings`, file: "10-settings.png" },
  { url: `${BASE}/?demo=plugins`, file: "11-plugins.png" },
];

const browser = await chromium.launch();

for (const { url, file, height, theme } of shots) {
  // Fresh context per shot so a theme seed lands in localStorage BEFORE any
  // app script runs (avoids racing the useTheme hook's mount effect).
  const context = await browser.newContext({
    viewport: { width: 1512, height: height ?? 945 },
    deviceScaleFactor: 2,
  });
  if (theme) {
    await context.addInitScript(
      (t) => localStorage.setItem("conduit.theme", t),
      theme,
    );
  }
  const page = await context.newPage();
  await page.goto(url, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(600);
  await page.screenshot({ path: resolve(OUT, file) });
  console.log("wrote", file);
  await context.close();
}

await browser.close();
