package auth

import (
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"github.com/fahad/dashboard/internal/httputil"
)

// dummyHash is a real cost-10 bcrypt hash of random bytes. Unknown emails are
// compared against it so a failed login costs the same whether or not the
// account exists.
const dummyHash = "$2a$10$xldbDwxTwAEN1f78OsHC9ek7W9iuoqqpD5S.NyLYYs5R6BHGTQ7w6"

type Handler struct {
	sm      *scs.SessionManager
	db      *sql.DB
	limiter *RateLimiter
	delay   *AccountDelay
	trusted []netip.Prefix
	tmpl    *template.Template
}

// Option configures a Handler.
type Option func(*Handler)

// WithTrustedProxies sets the proxies whose X-Forwarded-For is believed when
// rate limiting by client IP.
func WithTrustedProxies(prefixes []netip.Prefix) Option {
	return func(h *Handler) { h.trusted = prefixes }
}

func NewHandler(sm *scs.SessionManager, db *sql.DB, limiter *RateLimiter, tmpl *template.Template, opts ...Option) *Handler {
	h := &Handler{
		sm:      sm,
		db:      db,
		limiter: limiter,
		delay:   NewAccountDelay(),
		tmpl:    tmpl,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	expired := r.URL.Query().Get("expired") == "1"
	h.renderLogin(w, r.URL.Query().Get("next"), "", "", expired)
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	ip := httputil.ClientIP(r, h.trusted)
	next := r.FormValue("next")
	email := strings.TrimSpace(r.FormValue("email"))

	key := rateLimitKey(ip)
	if !h.limiter.Allow(key) {
		retryAfter := h.limiter.RetryAfter(key)
		mins := int(retryAfter.Minutes()) + 1
		msg := fmt.Sprintf("Too many attempts. Try again in %d minute(s).", mins)
		slog.Warn("login rate limited", "ip", ip)
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		w.WriteHeader(http.StatusTooManyRequests)
		h.renderLogin(w, next, msg, email, false)
		return
	}

	password := r.FormValue("password")

	if d := h.delay.Delay(email); d > 0 {
		select {
		case <-time.After(d):
		case <-r.Context().Done():
			return
		}
	}

	user, err := FindByEmail(h.db, email)
	if err != nil {
		slog.Error("finding user", "error", err)
		h.renderLogin(w, next, "Internal error.", email, false)
		return
	}
	hash := dummyHash
	if user != nil {
		hash = user.PasswordHash
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil || user == nil {
		h.delay.Fail(email, user != nil)
		slog.Warn("login failed", "ip", ip)
		h.renderLogin(w, next, "Incorrect email or password.", email, false)
		return
	}
	h.delay.Succeed(email)

	if err := h.sm.RenewToken(r.Context()); err != nil {
		slog.Error("renewing session token", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Store user_id in session data. The session store extracts it from the
	// gob-encoded blob during CommitCtx to tag the sessions row -- no shared
	// mutable state needed.
	h.sm.Put(r.Context(), "user_id", user.ID)
	h.sm.Put(r.Context(), "user_email", user.Email)
	h.sm.Put(r.Context(), "is_admin", user.Role == "admin")
	h.sm.Put(r.Context(), "first_name", user.FirstName)

	slog.Info("login successful", "ip", ip, "user_id", user.ID)

	dest := "/"
	if httputil.IsLocalPath(next) {
		dest = next
	}
	http.Redirect(w, r, dest, http.StatusSeeOther) //nolint:gosec // G710: dest checked by httputil.IsLocalPath
}

// rateLimitKey buckets IPv6 clients by /64, the block one holder typically
// controls; IPv4 addresses are used as-is.
func rateLimitKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.Is4() || addr.Is4In6() {
		return ip
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.sm.Destroy(r.Context()); err != nil {
		slog.Error("destroying session", "error", err)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) renderLogin(w http.ResponseWriter, next, errMsg, email string, sessionExpired bool) {
	data := map[string]any{
		"Error":          errMsg,
		"Next":           next,
		"Email":          email,
		"SessionExpired": sessionExpired,
	}
	if err := h.tmpl.Execute(w, data); err != nil {
		slog.Error("rendering login page", "error", err)
	}
}
