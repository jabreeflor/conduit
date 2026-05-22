import { useCallback, useEffect, useState } from "react";

// The design tokens ship three modes (design/dist/web/tokens.css): light is
// the cream default, dark is the warm-charcoal complement, hc is high-contrast.
// We drive them by setting `data-theme` on <html>, persisted across launches.
export type Theme = "light" | "dark" | "hc";

const THEMES: Theme[] = ["light", "dark", "hc"];
const STORAGE_KEY = "conduit.theme";

function readStored(): Theme {
  if (typeof window === "undefined") return "light";
  const v = window.localStorage.getItem(STORAGE_KEY);
  return v === "dark" || v === "hc" || v === "light" ? v : "light";
}

export function useTheme(): {
  theme: Theme;
  setTheme: (t: Theme) => void;
  cycleTheme: () => void;
} {
  const [theme, setThemeState] = useState<Theme>(readStored);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    window.localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  const setTheme = useCallback((t: Theme) => setThemeState(t), []);
  const cycleTheme = useCallback(
    () => setThemeState((t) => THEMES[(THEMES.indexOf(t) + 1) % THEMES.length]),
    [],
  );

  return { theme, setTheme, cycleTheme };
}

// Glyph + label for each mode, used by the toggle control.
export const THEME_META: Record<Theme, { icon: string; label: string }> = {
  light: { icon: "light_mode", label: "Light" },
  dark: { icon: "dark_mode", label: "Dark" },
  hc: { icon: "contrast", label: "High contrast" },
};
