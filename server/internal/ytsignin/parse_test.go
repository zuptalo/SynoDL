package ytsignin

import (
	"errors"
	"strings"
	"testing"
)

const marker = "SECRETVALUE-7f3a"

func header(names ...string) string {
	var p []string
	for _, n := range names {
		p = append(p, n+"="+marker+n)
	}
	return strings.Join(p, "; ")
}

func TestParse_HeaderShapes(t *testing.T) {
	base := header("SAPISID", "__Secure-3PSID", "LOGIN_INFO", "YSC")
	for name, in := range map[string]string{
		"bare":           base,
		"prefixed":       "Cookie: " + base,
		"prefixed lower": "cookie:" + base,
		"whitespace":     "  \n" + base + "\n\n",
		"wrapped":        strings.ReplaceAll(base, "; ", ";\n "),
		"crlf":           "Cookie: " + strings.ReplaceAll(base, "; ", ";\r\n"),
		"trailing semi":  base + ";",
	} {
		t.Run(name, func(t *testing.T) {
			r, err := Parse(in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if r.Count != 4 || len(r.Found) != 3 || r.Warning() != "" {
				t.Fatalf("count=%d found=%v warning=%q", r.Count, r.Found, r.Warning())
			}
			s := string(r.Netscape)
			if !strings.HasPrefix(s, "# Netscape HTTP Cookie File\n") {
				t.Fatalf("no header: %q", s[:30])
			}
			if !strings.Contains(s, ".youtube.com\tTRUE\t/\tTRUE\t2000000000\tSAPISID\t"+marker+"SAPISID\n") {
				t.Fatalf("secure SAPISID line missing:\n%s", s)
			}
			if !strings.Contains(s, "\tFALSE\t2000000000\tYSC\t") {
				t.Fatalf("YSC should not be secure:\n%s", s)
			}
		})
	}
}

func TestParse_SecureFlagRules(t *testing.T) {
	r, err := Parse(header("__Secure-1PSID", "__Host-X", "LOGIN_INFO", "PREF"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(r.Netscape)
	for _, n := range []string{"__Secure-1PSID", "__Host-X", "LOGIN_INFO"} {
		if !strings.Contains(s, "\t/\tTRUE\t2000000000\t"+n+"\t") {
			t.Errorf("%s should be secure", n)
		}
	}
	if !strings.Contains(s, "\t/\tFALSE\t2000000000\tPREF\t") {
		t.Error("PREF should not be secure")
	}
}

func TestParse_NetscapePassesThroughAndFiltersDomains(t *testing.T) {
	in := "# Netscape HTTP Cookie File\n" +
		".youtube.com\tTRUE\t/\tTRUE\t1900000000\tSAPISID\ta1\n" +
		"#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t1900000000\t__Secure-3PSID\ta2\n" +
		".google.com\tTRUE\t/\tTRUE\t1900000000\tSID\ta3\n" +
		"accounts.google.com\tFALSE\t/\tTRUE\t1900000000\tLOGIN_INFO\ta4\n" +
		".evil.example\tTRUE\t/\tTRUE\t1900000000\tSTOLEN\tx\n" +
		".notyoutube.com\tTRUE\t/\tTRUE\t1900000000\tALSO\tx\n"
	r, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Count != 4 {
		t.Fatalf("count=%d want 4 (foreign domains dropped)", r.Count)
	}
	s := string(r.Netscape)
	if strings.Contains(s, "STOLEN") || strings.Contains(s, "ALSO") {
		t.Fatalf("foreign cookie kept:\n%s", s)
	}
	if !strings.Contains(s, "#HttpOnly_.youtube.com\tTRUE\t/\tTRUE\t1900000000\t__Secure-3PSID\ta2\n") {
		t.Fatalf("HttpOnly line not preserved:\n%s", s)
	}
	if len(r.Missing) != 0 {
		t.Fatalf("missing=%v", r.Missing)
	}
}

func TestParse_NetscapeWithoutHeaderLine(t *testing.T) {
	in := ".youtube.com\tTRUE\t/\tFALSE\t1\tA\t1\n.youtube.com\tTRUE\t/\tFALSE\t1\tB\t1\n.youtube.com\tTRUE\t/\tFALSE\t1\tC\t1\n"
	if r, err := Parse(in); err != nil || r.Count != 3 {
		t.Fatalf("%v %d", err, r.Count)
	}
}

func TestParse_WarnsWhenNoLoginCookies(t *testing.T) {
	r, err := Parse(header("YSC", "PREF", "VISITOR_INFO1_LIVE"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Warning() != "no_login_cookies" || len(r.Missing) != len(LoginCookies) {
		t.Fatalf("warning=%q missing=%v", r.Warning(), r.Missing)
	}
}

func TestParse_Rejects(t *testing.T) {
	long := strings.Repeat("a", MaxPasteBytes+1)
	var many []string
	for i := 0; i <= MaxCookies; i++ {
		many = append(many, "c"+string(rune('a'+i%26))+strings.Repeat("x", i%7)+string(rune('0'+i%10))+"="+"v")
	}
	// distinct names are not needed: only the count matters
	cases := map[string]struct {
		in   string
		want error
	}{
		"empty":        {"   \n ", ErrUnrecognised},
		"too large":    {long, ErrTooLarge},
		"too many":     {strings.Join(many, "; "), ErrTooLarge},
		"one cookie":   {"SAPISID=" + marker, ErrTooFew},
		"no equals":    {"just some words, not a header at all", ErrUnrecognised},
		"control char": {"A=1; B=2; C=x\x01y", ErrInvalid},
		"bad netscape": {"# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\n", ErrInvalid},
		"only foreign": {"# Netscape HTTP Cookie File\n.evil.example\tTRUE\t/\tTRUE\t1\tA\t1\n.evil.example\tTRUE\t/\tTRUE\t1\tB\t1\n.evil.example\tTRUE\t/\tTRUE\t1\tC\t1\n", ErrTooFew},
		"long value":   {"A=1; B=2; C=" + strings.Repeat("v", maxValueLen+1), ErrInvalid},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("err=%v want %v", err, c.want)
			}
		})
	}
}

// The one property that matters most: an error never carries pasted text.
func TestParse_ErrorsNeverLeakValues(t *testing.T) {
	for _, in := range []string{
		"SAPISID=" + marker,
		"A=" + marker + "; B=2; C=x\x01",
		"# Netscape HTTP Cookie File\n.youtube.com\tTRUE\t/\tTRUE\t1\tA\t" + marker + "\n",
		"nonsense " + marker,
	} {
		_, err := Parse(in)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), marker) || strings.Contains(err.Error(), "SAPISID") {
			t.Fatalf("error leaks pasted text: %q", err)
		}
	}
}
