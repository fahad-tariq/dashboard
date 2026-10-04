package test

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/httputil"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.0/24")}
	tests := map[string]struct {
		remote  string
		headers map[string]string
		trusted []netip.Prefix
		want    string
	}{
		"untrusted peer, port stripped":     {remote: "203.0.113.5:51234", want: "203.0.113.5"},
		"untrusted peer ignores XFF":        {remote: "203.0.113.5:51234", headers: map[string]string{"X-Forwarded-For": "198.51.100.7"}, trusted: trusted, want: "203.0.113.5"},
		"untrusted peer ignores X-Real-IP":  {remote: "203.0.113.5:51234", headers: map[string]string{"X-Real-IP": "198.51.100.7"}, trusted: trusted, want: "203.0.113.5"},
		"no trusted proxies configured":     {remote: "172.30.0.2:4000", headers: map[string]string{"X-Forwarded-For": "198.51.100.7"}, want: "172.30.0.2"},
		"trusted peer uses rightmost XFF":   {remote: "172.30.0.2:4000", headers: map[string]string{"X-Forwarded-For": "6.6.6.6, 198.51.100.7"}, trusted: trusted, want: "198.51.100.7"},
		"trusted peer never uses X-Real-IP": {remote: "172.30.0.2:4000", headers: map[string]string{"X-Real-IP": "6.6.6.6"}, trusted: trusted, want: "172.30.0.2"},
		"trusted peer, garbage XFF":         {remote: "172.30.0.2:4000", headers: map[string]string{"X-Forwarded-For": "not-an-ip"}, trusted: trusted, want: "172.30.0.2"},
		"ipv6 peer":                         {remote: "[2001:db8::1]:443", want: "2001:db8::1"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := httputil.ClientIP(r, tc.trusted); got != tc.want {
				t.Errorf("ClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func loginAttempt(h http.Handler, remote string, headers map[string]string, email, password string) *httptest.ResponseRecorder {
	form := url.Values{"email": {email}, "password": {password}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Each attempt comes from a new source port and claims a different client
// via forwarding headers; none of that may reset the per-IP limit.
func TestLoginRateLimitIgnoresSpoofedHeaders(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	h := sm.LoadAndSave(http.HandlerFunc(auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl).LoginSubmit))

	var last *httptest.ResponseRecorder
	for i := range 6 {
		last = loginAttempt(h, fmt.Sprintf("203.0.113.5:%d", 40000+i), map[string]string{
			"X-Forwarded-For": fmt.Sprintf("198.51.100.%d", i),
			"X-Real-IP":       fmt.Sprintf("192.0.2.%d", i),
		}, "alice@test.com", "wrong")
	}
	if !strings.Contains(last.Body.String(), "Too many attempts") {
		t.Errorf("6th attempt was not rate limited; body: %s", last.Body.String())
	}
}

func TestLoginRateLimitPerClientBehindTrustedProxy(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	trusted := []netip.Prefix{netip.MustParsePrefix("172.30.0.0/24")}
	h := sm.LoadAndSave(http.HandlerFunc(auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl, auth.WithTrustedProxies(trusted)).LoginSubmit))

	proxy := "172.30.0.2:5000"
	for range 6 {
		loginAttempt(h, proxy, map[string]string{"X-Forwarded-For": "198.51.100.1"}, "alice@test.com", "wrong")
	}
	rr := loginAttempt(h, proxy, map[string]string{"X-Forwarded-For": "198.51.100.2"}, "alice@test.com", "secretpw")
	if rr.Code != http.StatusSeeOther {
		t.Errorf("a different client behind the proxy was blocked: %d %s", rr.Code, rr.Body.String())
	}
}

func TestLoginRateLimiterIsBoundedAndNeverResetsWholesale(t *testing.T) {
	rl := auth.NewRateLimiterWithCapacity(3)
	for range 6 {
		rl.Allow("10.0.0.1")
	}
	// Touching more distinct IPs than the capacity evicts the least recently
	// seen entries one at a time; the hot offender stays tracked.
	for i := range 2 {
		rl.Allow(fmt.Sprintf("10.0.1.%d", i))
		if rl.Allow("10.0.0.1") {
			t.Fatalf("offender unblocked after %d other IPs", i+1)
		}
	}
	for i := 2; i < 10; i++ {
		rl.Allow(fmt.Sprintf("10.0.1.%d", i))
	}
	if n := rl.Len(); n > 3 {
		t.Errorf("limiter tracks %d IPs, want at most 3", n)
	}
}

func TestAccountDelayIsProgressiveNotALockout(t *testing.T) {
	d := auth.NewAccountDelay()
	var delays []time.Duration
	for range 8 {
		delays = append(delays, d.Delay("Alice@Test.com"))
		d.Fail("alice@test.com", true)
	}
	if delays[0] != 0 || delays[1] != 0 || delays[2] != 0 {
		t.Errorf("first attempts delayed: %v", delays[:3])
	}
	if !slices.IsSorted(delays) || delays[len(delays)-1] == 0 {
		t.Errorf("delays not progressive: %v", delays)
	}
	if max := delays[len(delays)-1]; max > 10*time.Second {
		t.Errorf("delay %v exceeds cap; a long delay is a lockout in practice", max)
	}
	if got := d.Delay("bob@test.com"); got != 0 {
		t.Errorf("another account delayed by %v", got)
	}
	d.Succeed("ALICE@test.com")
	if got := d.Delay("alice@test.com"); got != 0 {
		t.Errorf("delay after success = %v, want 0", got)
	}
}

// Unknown emails must cost a bcrypt comparison too, or response time reveals
// which accounts exist.
func TestLoginUnknownEmailRunsBcrypt(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	h := sm.LoadAndSave(http.HandlerFunc(auth.NewHandler(sm, database, auth.NewRateLimiterWithCapacity(1024), loginTmpl).LoginSubmit))

	median := func(email string, base int) time.Duration {
		var ds []time.Duration
		for i := range 3 {
			start := time.Now()
			loginAttempt(h, fmt.Sprintf("192.0.2.%d:1000", base+i), nil, email, "wrong")
			ds = append(ds, time.Since(start))
		}
		slices.Sort(ds)
		return ds[1]
	}
	known := median("alice@test.com", 10)
	unknown := median("nobody@test.com", 20)
	if unknown < known/2 {
		t.Errorf("unknown email took %v vs %v for a known one; bcrypt skipped", unknown, known)
	}
}

func TestLoginDoesNotLogSubmittedEmail(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	h := sm.LoadAndSave(http.HandlerFunc(auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl).LoginSubmit))

	var logBuf bytes.Buffer
	orig := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(orig) })

	loginAttempt(h, "192.0.2.50:1000", nil, "nobody-secret@test.com", "wrong")
	loginAttempt(h, "192.0.2.51:1000", nil, "alice@test.com", "wrong")
	loginAttempt(h, "192.0.2.52:1000", nil, "alice@test.com", "secretpw")
	for _, email := range []string{"nobody-secret@test.com", "alice@test.com"} {
		if strings.Contains(logBuf.String(), email) {
			t.Errorf("log contains submitted email %q:\n%s", email, logBuf.String())
		}
	}
}

func TestConfigTrustedProxies(t *testing.T) {
	tests := map[string]struct {
		value   string
		want    []string
		wantErr bool
	}{
		"unset means none":   {value: "", want: nil},
		"cidr list":          {value: "172.30.0.0/24, 10.1.2.3/32", want: []string{"172.30.0.0/24", "10.1.2.3/32"}},
		"bare ip is a /32":   {value: "172.30.0.2", want: []string{"172.30.0.2/32"}},
		"garbage is refused": {value: "proxy.local", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			paths := tempPaths(t)
			paths["DASHBOARD_TRUSTED_PROXIES"] = tc.value
			setEnvForConfig(t, paths)
			cfg, err := config.Load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("config.Load accepted an invalid proxy list")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, p := range cfg.TrustedProxies {
				got = append(got, p.String())
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("TrustedProxies = %v, want %v", got, tc.want)
			}
		})
	}
}

// Flooding made-up emails must not evict a real account's failure history,
// or an attacker could reset its delay at will.
func TestAccountDelaySurvivesUnknownEmailFlood(t *testing.T) {
	d := auth.NewAccountDelay()
	for range 6 {
		d.Fail("owner@test.com", true)
	}
	before := d.Delay("owner@test.com")
	for i := range 5000 {
		d.Fail(fmt.Sprintf("nobody-%d@test.com", i), false)
	}
	if got := d.Delay("owner@test.com"); got != before || got == 0 {
		t.Errorf("owner delay after flood = %v, want %v", got, before)
	}
	// Unknown emails are delayed on the same schedule, so the delay does not
	// reveal which accounts exist.
	for range 6 {
		d.Fail("ghost@test.com", false)
	}
	if got := d.Delay("ghost@test.com"); got != before {
		t.Errorf("unknown email delay = %v, want %v like a real account", got, before)
	}
}

// One IPv6 holder controls a whole /64, so the limit must apply per /64.
func TestLoginRateLimitBucketsIPv6By64(t *testing.T) {
	sm, database := newTestSessionManager(t)
	createTestUser(t, database, "alice@test.com", "secretpw")
	h := sm.LoadAndSave(http.HandlerFunc(auth.NewHandler(sm, database, auth.NewRateLimiter(), loginTmpl).LoginSubmit))

	var last *httptest.ResponseRecorder
	for i := range 6 {
		last = loginAttempt(h, fmt.Sprintf("[2001:db8:1:2::%x]:443", i+1), nil, "nobody@test.com", "wrong")
	}
	if !strings.Contains(last.Body.String(), "Too many attempts") {
		t.Errorf("6th attempt from the same /64 was not rate limited")
	}
	if got := last.Header().Get("Retry-After"); got == "" {
		t.Error("rate-limited login lacks a Retry-After header")
	}
	other := loginAttempt(h, "[2001:db8:1:3::1]:443", nil, "nobody@test.com", "wrong")
	if strings.Contains(other.Body.String(), "Too many attempts") {
		t.Error("a different /64 was rate limited")
	}
}

func TestTrustedProxyAcceptsIPv4MappedForm(t *testing.T) {
	paths := tempPaths(t)
	paths["DASHBOARD_TRUSTED_PROXIES"] = "::ffff:172.30.0.2"
	setEnvForConfig(t, paths)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "172.30.0.2:4000"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := httputil.ClientIP(r, cfg.TrustedProxies); got != "198.51.100.7" {
		t.Errorf("ClientIP = %q; an IPv4-mapped trusted proxy was not matched", got)
	}
}
