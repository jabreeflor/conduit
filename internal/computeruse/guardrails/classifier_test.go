package guardrails

import (
	"slices"
	"testing"
)

func TestClassifier_Communication(t *testing.T) {
	c := DefaultClassifier()

	cases := []struct {
		name   string
		action Action
	}{
		{
			name:   "apple mail bundle id",
			action: Action{Verb: "click", BundleID: "com.apple.mail", Target: "Send"},
		},
		{
			name:   "slack bundle id",
			action: Action{Verb: "click", BundleID: "com.tinyspeck.slackmacgap", Target: "Send"},
		},
		{
			name:   "gmail browser host",
			action: Action{Verb: "click", URL: "https://mail.google.com/mail/u/0/#inbox", Target: "Send"},
		},
		{
			name:   "twitter post browser host",
			action: Action{Verb: "click", URL: "https://x.com/home", Target: "Post"},
		},
		{
			name:   "send email by description",
			action: Action{Verb: "click", Description: "send email to investors with quarterly numbers"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cats := c.Classify(tc.action)
			if !slices.Contains(cats, CategoryCommunication) {
				t.Fatalf("expected CategoryCommunication; got %v", cats)
			}
		})
	}
}

func TestClassifier_Financial(t *testing.T) {
	c := DefaultClassifier()
	cases := []struct {
		name   string
		action Action
	}{
		{
			name:   "chase mobile app",
			action: Action{Verb: "click", BundleID: "com.chase.sig.chase", Target: "Send Money"},
		},
		{
			name:   "venmo app",
			action: Action{Verb: "tap", BundleID: "com.venmo.iphone"},
		},
		{
			name:   "stripe checkout url",
			action: Action{Verb: "click", URL: "https://checkout.stripe.com/c/pay/abc", Target: "Pay"},
		},
		{
			name:   "amazon checkout button",
			action: Action{Verb: "click", URL: "https://www.amazon.com/cart", Target: "Place order"},
		},
		{
			name:   "confirm purchase keyword",
			action: Action{Verb: "click", Description: "confirm purchase of $4500 chair"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cats := c.Classify(tc.action)
			if !slices.Contains(cats, CategoryFinancial) {
				t.Fatalf("expected CategoryFinancial; got %v", cats)
			}
			if cats[0] != CategoryFinancial {
				t.Fatalf("Financial should be highest priority; got order %v", cats)
			}
		})
	}
}

func TestClassifier_Filesystem(t *testing.T) {
	c := DefaultClassifier()
	cases := []struct {
		name   string
		action Action
	}{
		{
			name:   "rm -rf in shell",
			action: Action{Verb: "type", Text: "sudo rm -rf /Users/me/work"},
		},
		{
			name:   "empty trash button in finder",
			action: Action{Verb: "click", BundleID: "com.apple.finder", Target: "Empty Trash"},
		},
		{
			name:   "uninstall keyword",
			action: Action{Verb: "click", Description: "uninstall application Acme"},
		},
		{
			name:   "diskutil erase",
			action: Action{Verb: "type", Text: "diskutil eraseDisk APFS NewName disk2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cats := c.Classify(tc.action)
			if !slices.Contains(cats, CategoryFilesystem) {
				t.Fatalf("expected CategoryFilesystem; got %v", cats)
			}
		})
	}
}

func TestClassifier_System(t *testing.T) {
	c := DefaultClassifier()
	cases := []struct {
		name   string
		action Action
	}{
		{
			name:   "shutdown verb",
			action: Action{Verb: "click", Description: "shutdown the computer"},
		},
		{
			name:   "sudo in terminal",
			action: Action{Verb: "type", BundleID: "com.apple.terminal", Text: "sudo shutdown -h now"},
		},
		{
			name:   "reboot keyword",
			action: Action{Verb: "click", Target: "Restart computer"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cats := c.Classify(tc.action)
			if !slices.Contains(cats, CategorySystem) {
				t.Fatalf("expected CategorySystem; got %v", cats)
			}
		})
	}
}

func TestClassifier_BenignAction(t *testing.T) {
	c := DefaultClassifier()
	benign := []Action{
		{Verb: "screenshot"},
		{Verb: "click", BundleID: "com.apple.safari", URL: "https://news.ycombinator.com/", Target: "Read more"},
		{Verb: "type", Text: "hello world"},
		{Verb: "scroll", Description: "scroll down"},
	}
	for _, a := range benign {
		cats := c.Classify(a)
		if len(cats) != 0 {
			t.Fatalf("expected no categories for %v; got %v", a, cats)
		}
	}
}

func TestClassifier_CustomConfigExtends(t *testing.T) {
	c := NewClassifier(Config{
		BundleIDs: map[Category][]string{
			CategoryFinancial: {"com.example.brokerage"},
		},
		HostSuffixes: map[Category][]string{
			CategoryCommunication: {"my-internal-forum.example.com"},
		},
		Keywords: map[Category][]string{
			CategoryFilesystem: {"shred file"},
		},
	})

	if got := c.Classify(Action{BundleID: "com.example.brokerage.app"}); !slices.Contains(got, CategoryFinancial) {
		t.Errorf("custom bundle did not match: %v", got)
	}
	if got := c.Classify(Action{URL: "https://my-internal-forum.example.com/post"}); !slices.Contains(got, CategoryCommunication) {
		t.Errorf("custom host did not match: %v", got)
	}
	if got := c.Classify(Action{Description: "shred file ~/foo"}); !slices.Contains(got, CategoryFilesystem) {
		t.Errorf("custom keyword did not match: %v", got)
	}

	// Defaults still apply.
	if got := c.Classify(Action{BundleID: "com.apple.mail"}); !slices.Contains(got, CategoryCommunication) {
		t.Errorf("default bundle disappeared after custom extension: %v", got)
	}
}

func TestHostFromURL(t *testing.T) {
	cases := map[string]string{
		"https://Mail.Google.com/inbox": "mail.google.com",
		"http://chase.com/login":        "chase.com",
		"checkout.stripe.com/c/pay/abc": "checkout.stripe.com",
		"https://example.com:8080/path": "example.com:8080",
		"":                              "",
		"notaurl":                       "notaurl",
	}
	for in, want := range cases {
		if got := hostFromURL(in); got != want {
			t.Errorf("hostFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}
