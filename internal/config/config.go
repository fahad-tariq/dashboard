package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fahad/dashboard/internal/atomicfile"
)

type Config struct {
	IdeasPath         string
	UploadsDir        string
	PersonalPath      string
	FamilyPath        string
	MaintenancePath   string
	HouseProjectsPath string
	UserDataDir       string
	DBPath            string
	APIToken          string
	Addr              string
	PasswordHash      string
	AuthDisabled      bool // DASHBOARD_AUTH=disabled: local development on loopback only.
	TrustedProxies    []netip.Prefix
	SessionLifetime   time.Duration
	SecureCookies     bool
	HasUsers          bool // Set at startup after checking the users table.
	Location          *time.Location
}

func Load() (*Config, error) {
	sessionLifetime, err := time.ParseDuration(envOr("SESSION_LIFETIME", "720h"))
	if err != nil {
		return nil, fmt.Errorf("parsing SESSION_LIFETIME: %w", err)
	}

	// A value that does not parse is refused rather than read as false, so a
	// typo cannot drop the Secure flag.
	secureCookies := true
	if v := os.Getenv("DASHBOARD_SECURE_COOKIES"); v != "" {
		var err error
		if secureCookies, err = strconv.ParseBool(v); err != nil {
			return nil, fmt.Errorf("parsing DASHBOARD_SECURE_COOKIES %q: want true or false", v)
		}
	}

	authDisabled := false
	switch v := os.Getenv("DASHBOARD_AUTH"); v {
	case "", "enabled":
	case "disabled":
		authDisabled = true
	default:
		return nil, fmt.Errorf("DASHBOARD_AUTH must be \"enabled\" or \"disabled\", got %q", v)
	}

	trusted, err := parsePrefixes(os.Getenv("DASHBOARD_TRUSTED_PROXIES"))
	if err != nil {
		return nil, fmt.Errorf("parsing DASHBOARD_TRUSTED_PROXIES: %w", err)
	}

	loc := time.Local
	if tz := os.Getenv("DASHBOARD_TIMEZONE"); tz != "" {
		var err error
		loc, err = time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("parsing DASHBOARD_TIMEZONE %q: %w", tz, err)
		}
	}

	// IDEAS_PATH takes precedence. Fall back to IDEAS_DIR for backwards
	// compatibility: if IDEAS_DIR is set, derive the file path from it.
	ideasPath := os.Getenv("IDEAS_PATH")
	if ideasPath == "" {
		ideasDir := os.Getenv("IDEAS_DIR")
		if ideasDir != "" {
			ideasPath = filepath.Join(filepath.Dir(ideasDir), "ideas.md")
		} else {
			ideasPath = "/data/ideas.md"
		}
	}

	c := &Config{
		IdeasPath:         ideasPath,
		UploadsDir:        envOr("UPLOADS_DIR", "/data/uploads"),
		PersonalPath:      envOr("PERSONAL_PATH", "/data/personal.md"),
		FamilyPath:        envOr("FAMILY_PATH", "/data/family.md"),
		MaintenancePath:   envOr("MAINTENANCE_PATH", "/data/maintenance.md"),
		HouseProjectsPath: envOr("HOUSE_PROJECTS_PATH", "/data/house-projects.md"),
		UserDataDir:       envOr("USER_DATA_DIR", "/data/users"),
		DBPath:            envOr("DB_PATH", "/data/db/dashboard.db"),
		APIToken:          os.Getenv("DASHBOARD_API_TOKEN"),
		Addr:              envOr("ADDR", ":8080"),
		PasswordHash:      os.Getenv("DASHBOARD_PASSWORD_HASH"),
		AuthDisabled:      authDisabled,
		TrustedProxies:    trusted,
		SessionLifetime:   sessionLifetime,
		SecureCookies:     secureCookies,
		Location:          loc,
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	// Remove temp files left by writes interrupted in an earlier run, before
	// anything below writes.
	cleaned := map[string]bool{}
	for _, dir := range []string{
		filepath.Dir(c.PersonalPath), filepath.Dir(c.FamilyPath), filepath.Dir(c.IdeasPath),
		filepath.Dir(c.MaintenancePath), filepath.Dir(c.HouseProjectsPath), c.UserDataDir,
	} {
		if cleaned[dir] {
			continue
		}
		cleaned[dir] = true
		if err := atomicfile.CleanStale(dir); err != nil {
			return fmt.Errorf("removing stale temp files in %s: %w", dir, err)
		}
	}

	if err := os.MkdirAll(c.UploadsDir, 0o755); err != nil {
		return fmt.Errorf("creating uploads dir %q: %w", c.UploadsDir, err)
	}

	// Create skeleton files for tracker markdown files and ideas.
	for _, entry := range []struct {
		path    string
		heading string
	}{
		{c.PersonalPath, "Personal"},
		{c.FamilyPath, "Family"},
		{c.IdeasPath, "Ideas"},
		{c.MaintenancePath, "Maintenance"},
		{c.HouseProjectsPath, "House"},
	} {
		if err := os.MkdirAll(filepath.Dir(entry.path), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", entry.path, err)
		}
		if _, err := os.Stat(entry.path); os.IsNotExist(err) {
			skeleton := "# " + entry.heading + "\n\n"
			if err := atomicfile.Write(entry.path, []byte(skeleton), 0o644); err != nil {
				return fmt.Errorf("creating %s skeleton: %w", entry.path, err)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0o755); err != nil {
		return fmt.Errorf("creating DB directory: %w", err)
	}

	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// AuthEnabled returns true if authentication should be enforced.
func (c *Config) AuthEnabled() bool {
	return !c.AuthDisabled && (c.PasswordHash != "" || c.HasUsers)
}

// CheckAuthMode refuses configurations that would serve an open dashboard by
// accident. Call it once HasUsers is known. A lost data volume (no users, no
// hash) must stop the server, and turning auth off is only allowed on a
// loopback address.
func (c *Config) CheckAuthMode() error {
	if c.AuthDisabled {
		if !isLoopbackAddr(c.Addr) {
			return fmt.Errorf("DASHBOARD_AUTH=disabled requires a loopback ADDR (127.0.0.1, ::1 or localhost), got %q", c.Addr)
		}
		return nil
	}
	if c.PasswordHash == "" && !c.HasUsers {
		return errors.New("no users and no DASHBOARD_PASSWORD_HASH: refusing to start without authentication " +
			"(create a user, set DASHBOARD_PASSWORD_HASH, or set DASHBOARD_AUTH=disabled with a loopback ADDR for local development)")
	}
	return nil
}

// parsePrefixes reads a comma-separated list of CIDRs; a bare IP means that
// single address.
func parsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for field := range strings.SplitSeq(s, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		// Peers are compared unmapped, so store IPv4-mapped forms as IPv4.
		if p, err := netip.ParsePrefix(field); err == nil {
			if p.Addr().Is4In6() && p.Bits() >= 96 {
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			out = append(out, p.Masked())
			continue
		}
		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, fmt.Errorf("%q is not a CIDR or IP address", field)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
