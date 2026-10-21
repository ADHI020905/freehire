package handler

import (
	"net/url"
	"testing"
)

func TestRedirectWithFragment(t *testing.T) {
	t.Run("puts values in the fragment, not the query", func(t *testing.T) {
		got, err := redirectWithFragment("https://abc.chromiumapp.org/cb", url.Values{
			"token": {"secret-123"},
			"state": {"s1"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "https://abc.chromiumapp.org/cb#state=s1&token=secret-123"
		if got != want {
			t.Errorf("redirectWithFragment = %q, want %q", got, want)
		}
	})

	t.Run("replaces any pre-existing fragment", func(t *testing.T) {
		got, err := redirectWithFragment("https://abc.chromiumapp.org/#old", url.Values{
			"token": {"t"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "https://abc.chromiumapp.org/#token=t"; got != want {
			t.Errorf("redirectWithFragment = %q, want %q", got, want)
		}
	})

	t.Run("percent-encodes special characters", func(t *testing.T) {
		got, err := redirectWithFragment("https://abc.chromiumapp.org/", url.Values{
			"state": {"a b&c"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "https://abc.chromiumapp.org/#state=a+b%26c"; got != want {
			t.Errorf("redirectWithFragment = %q, want %q", got, want)
		}
	})
}

func TestValidateExtensionRedirect(t *testing.T) {
	firefox := "13f2644fb08a641dc52d5b7c11faf2fb9775ee26"
	allow := []string{"abcdefghijklmnop", "qrstuvwxyzabcdef", firefox}

	cases := []struct {
		name    string
		uri     string
		allow   []string
		wantErr bool
	}{
		{"allowlisted id", "https://abcdefghijklmnop.chromiumapp.org/", allow, false},
		{"allowlisted id with path", "https://abcdefghijklmnop.chromiumapp.org/cb", allow, false},
		{"second allowlisted id", "https://qrstuvwxyzabcdef.chromiumapp.org/", allow, false},
		{"unknown id", "https://zzzzzzzzzzzzzzzz.chromiumapp.org/", allow, true},
		{"wrong scheme", "http://abcdefghijklmnop.chromiumapp.org/", allow, true},
		{"wrong host", "https://abcdefghijklmnop.evil.com/", allow, true},
		{"id as sibling label only", "https://abcdefghijklmnop.chromiumapp.org.evil.com/", allow, true},
		{"multi-label before suffix", "https://a.abcdefghijklmnop.chromiumapp.org/", allow, true},
		{"port present", "https://abcdefghijklmnop.chromiumapp.org:8080/", allow, true},
		{"malformed url", "https://%zz", allow, true},
		{"allowlisted firefox hash", "https://" + firefox + ".extensions.allizom.org/", allow, false},
		{"unknown firefox hash", "https://ffffffffffffffffffffffffffffffffffffffff.extensions.allizom.org/", allow, true},
		{"firefox hash under a sibling label", "https://" + firefox + ".allizom.org/", allow, true},
		{"multi-label before firefox suffix", "https://a." + firefox + ".extensions.allizom.org/", allow, true},
		{"empty allowlist disables", "https://abcdefghijklmnop.chromiumapp.org/", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExtensionRedirect(tc.uri, tc.allow)
			if tc.wantErr && err == nil {
				t.Errorf("validateExtensionRedirect(%q) = nil, want error", tc.uri)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateExtensionRedirect(%q) = %v, want nil", tc.uri, err)
			}
		})
	}
}
