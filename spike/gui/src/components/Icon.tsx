// Icon renders a Google Material Symbols (Outlined) glyph by name. The font is
// loaded in index.html. We standardize on Material Symbols (over an icon
// library) so glyph names map 1:1 to the design mockups — e.g. `terminal`,
// `psychology`, `auto_mode`. `size` sets the optical size + font-size; `fill`
// toggles the filled variant via font-variation-settings.
export function Icon({
  name,
  size = 20,
  fill = false,
  weight = 400,
  className,
  title,
}: {
  name: string;
  size?: number;
  fill?: boolean;
  weight?: number;
  className?: string;
  title?: string;
}) {
  return (
    <span
      className={`material-symbols-outlined${className ? ` ${className}` : ""}`}
      style={{
        fontSize: size,
        fontVariationSettings: `'FILL' ${fill ? 1 : 0}, 'wght' ${weight}, 'GRAD' 0, 'opsz' ${size}`,
      }}
      aria-hidden={title ? undefined : true}
      title={title}
      role={title ? "img" : undefined}
    >
      {name}
    </span>
  );
}
