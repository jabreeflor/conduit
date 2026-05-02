package guardrails

import (
	"net/url"
	"strings"
)

// Classifier holds the bundle-ID, URL, and keyword indices used to label an
// Action with one or more risk Categories. The lists are intentionally
// pragmatic for v1 — extend Config to add custom entries.
type Classifier struct {
	// Bundle ID prefixes / exact matches keyed by category. Matched
	// case-insensitively against Action.BundleID.
	bundleIDs map[Category][]string
	// Host suffixes keyed by category. Matched case-insensitively against
	// the host parsed from Action.URL (e.g. "chase.com" matches
	// "secure.chase.com").
	hostSuffixes map[Category][]string
	// Verb / target / text keyword fragments keyed by category. Matched
	// case-insensitively as a substring against the joined action text.
	keywords map[Category][]string
}

// Config customizes the classifier. Each field is OPTIONAL — missing
// categories fall back to DefaultClassifier's curated list.
type Config struct {
	BundleIDs    map[Category][]string
	HostSuffixes map[Category][]string
	Keywords     map[Category][]string
	// FinancialDefaultDeny, when true (the package default), causes the
	// policy layer to prefer VerdictDeny over VerdictRequireConfirmation
	// when a financial signal is present but ambiguous (e.g. unknown
	// bundle ID for a checkout-shaped target).
	FinancialDefaultDeny bool
}

// DefaultClassifier returns a curated v1 classifier for macOS.
func DefaultClassifier() *Classifier {
	return NewClassifier(Config{FinancialDefaultDeny: true})
}

// NewClassifier merges cfg with the curated defaults and returns a ready
// classifier. Custom entries APPEND to defaults — they don't replace them,
// so callers cannot accidentally weaken the safety floor.
func NewClassifier(cfg Config) *Classifier {
	c := &Classifier{
		bundleIDs:    cloneMap(defaultBundleIDs),
		hostSuffixes: cloneMap(defaultHostSuffixes),
		keywords:     cloneMap(defaultKeywords),
	}
	for cat, ids := range cfg.BundleIDs {
		c.bundleIDs[cat] = append(c.bundleIDs[cat], ids...)
	}
	for cat, hosts := range cfg.HostSuffixes {
		c.hostSuffixes[cat] = append(c.hostSuffixes[cat], hosts...)
	}
	for cat, kws := range cfg.Keywords {
		c.keywords[cat] = append(c.keywords[cat], kws...)
	}
	return c
}

// Classify returns every category triggered by the action. The slice is
// stable in priority order: Financial, System, Filesystem, Communication —
// so the first element is the dominant risk for default-deny logic.
func (c *Classifier) Classify(a Action) []Category {
	bundle := strings.ToLower(strings.TrimSpace(a.BundleID))
	app := strings.ToLower(strings.TrimSpace(a.AppName))
	host := hostFromURL(a.URL)
	combined := strings.ToLower(strings.Join([]string{
		a.Verb, a.Target, a.Path, a.Text, a.Description,
	}, " "))

	hits := make(map[Category]bool, 4)
	check := func(cat Category) {
		if matchPrefix(bundle, c.bundleIDs[cat]) ||
			matchContainsAny(app, c.bundleIDs[cat]) ||
			matchHostSuffix(host, c.hostSuffixes[cat]) ||
			matchSubstring(combined, c.keywords[cat]) {
			hits[cat] = true
		}
	}
	for _, cat := range categoryPriority {
		check(cat)
	}

	out := make([]Category, 0, len(hits))
	for _, cat := range categoryPriority {
		if hits[cat] {
			out = append(out, cat)
		}
	}
	return out
}

// categoryPriority orders categories from most-severe to least-severe for
// default-deny tie-breaking. Financial first by PRD direction.
var categoryPriority = []Category{
	CategoryFinancial,
	CategorySystem,
	CategoryFilesystem,
	CategoryCommunication,
}

// --- curated v1 lists ---------------------------------------------------

// defaultBundleIDs maps macOS app bundle IDs (and human app-name fragments)
// to risk categories. Match is prefix-on-bundle, contains-on-app-name.
var defaultBundleIDs = map[Category][]string{
	CategoryCommunication: {
		"com.apple.mail",       // Apple Mail
		"com.apple.mobilemail", // Mail (macOS Catalina+ id variant)
		"com.apple.messages",   // Messages
		"com.apple.ichat",      // legacy Messages bundle id
		"com.tinyspeck.slack",  // Slack (covers slackmacgap suffix)
		"com.microsoft.teams",
		"com.microsoft.outlook",
		"us.zoom.xos",
		"com.hnc.discord",
		"com.tdesktop.telegram",
		"net.whatsapp.whatsapp",
		"com.readdle.smartemail-mac", // Spark
		"mail",                       // app-name fragment fallback
		"slack",
		"messages",
		"outlook",
		"discord",
	},
	CategoryFinancial: {
		// Apple Wallet / Pay
		"com.apple.passbook",
		// Common consumer banking / brokerage / payments app prefixes
		"com.chase.sig.chase",
		"com.bankofamerica.bofa",
		"com.wellsfargo.wellsfargomobile",
		"com.citi.citimobile",
		"com.paypal.ppclient",
		"com.venmo",
		"com.squareup.cash",
		"com.robinhood",
		"com.coinbase",
		"com.intuit.quickbooks",
		"com.intuit.turbotax",
		"com.mint",
		// app-name fragment fallbacks
		"chase",
		"paypal",
		"venmo",
		"robinhood",
		"coinbase",
		"wallet",
	},
	CategoryFilesystem: {
		"com.apple.finder",
	},
	CategorySystem: {
		"com.apple.systempreferences",
		"com.apple.systemsettings",
		"com.apple.terminal",
		"com.googlecode.iterm2",
	},
}

// defaultHostSuffixes maps URL host suffixes (matched right-to-left) to
// risk categories. Used for browser computer-use actions.
var defaultHostSuffixes = map[Category][]string{
	CategoryCommunication: {
		"mail.google.com",
		"outlook.live.com",
		"outlook.office.com",
		"outlook.office365.com",
		"twitter.com",
		"x.com",
		"facebook.com",
		"instagram.com",
		"linkedin.com",
		"reddit.com",
		"tiktok.com",
		"threads.net",
		"bsky.app",
		"mastodon.social",
		"slack.com",
		"discord.com",
		"web.whatsapp.com",
		"messages.google.com",
	},
	CategoryFinancial: {
		"chase.com",
		"bankofamerica.com",
		"wellsfargo.com",
		"citi.com",
		"capitalone.com",
		"americanexpress.com",
		"discover.com",
		"usbank.com",
		"schwab.com",
		"fidelity.com",
		"vanguard.com",
		"robinhood.com",
		"coinbase.com",
		"binance.com",
		"kraken.com",
		"paypal.com",
		"venmo.com",
		"cash.app",
		"stripe.com",
		"checkout.stripe.com",
		"checkout.shopify.com",
		"checkout.amazon.com",
		"pay.amazon.com",
		"buy.itunes.apple.com",
		"finance.apple.com",
		"plaid.com",
	},
}

// defaultKeywords maps verb/target/description substrings to categories.
// Lower-cased; matched as substring against the joined action text.
var defaultKeywords = map[Category][]string{
	CategoryCommunication: {
		"send email",
		"send message",
		"send dm",
		"send tweet",
		"send post",
		"reply all",
		"compose mail",
		"publish post",
		"post publicly",
		"share publicly",
		"tweet",
		"send to ",
	},
	CategoryFinancial: {
		"confirm purchase",
		"complete purchase",
		"place order",
		"submit order",
		"checkout",
		"check out",
		"pay now",
		"send money",
		"transfer funds",
		"wire transfer",
		"buy now",
		"add card",
		"authorize payment",
	},
	CategoryFilesystem: {
		"rm -rf",
		"sudo rm",
		"empty trash",
		"empty bin",
		"move to trash",
		"delete file",
		"delete folder",
		"format disk",
		"erase disk",
		"uninstall ",
		"diskutil erase",
	},
	CategorySystem: {
		"shutdown",
		"restart computer",
		"reboot",
		"log out",
		"sign out",
		"sudo ",
		"sudo\t",
		"system preferences",
		"disable firewall",
		"disable gatekeeper",
		"sudo -s",
	},
}

// --- helpers -------------------------------------------------------------

func cloneMap(in map[Category][]string) map[Category][]string {
	out := make(map[Category][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func hostFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		// Try a relaxed parse for bare hosts ("chase.com/login").
		// Strip scheme-less leading slashes and a trailing path.
		host := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		return strings.ToLower(host)
	}
	return strings.ToLower(u.Host)
}

func matchPrefix(s string, prefixes []string) bool {
	if s == "" {
		return false
	}
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func matchContainsAny(s string, fragments []string) bool {
	if s == "" {
		return false
	}
	for _, f := range fragments {
		if f == "" {
			continue
		}
		// App-name fallbacks are short fragments — only match those that
		// don't look like reverse-DNS bundle IDs to avoid false positives
		// from app names like "Chase" matching "com.chase.sig.chase".
		if strings.Contains(f, ".") {
			continue
		}
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}

func matchHostSuffix(host string, suffixes []string) bool {
	if host == "" {
		return false
	}
	for _, suf := range suffixes {
		if suf == "" {
			continue
		}
		if host == suf || strings.HasSuffix(host, "."+suf) {
			return true
		}
	}
	return false
}

func matchSubstring(s string, fragments []string) bool {
	if s == "" {
		return false
	}
	for _, f := range fragments {
		if f == "" {
			continue
		}
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}
