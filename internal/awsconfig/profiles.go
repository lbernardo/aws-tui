package awsconfig

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is a named profile found in the shared AWS files.
type Profile struct {
	Name   string
	Region string
	// Source says where it was found: "config", "credentials" or "sso".
	Source string
}

// ListProfiles returns the profiles declared in ~/.aws/config and
// ~/.aws/credentials (or AWS_CONFIG_FILE / AWS_SHARED_CREDENTIALS_FILE).
func ListProfiles() []Profile {
	home, _ := os.UserHomeDir()
	configFile := envOr("AWS_CONFIG_FILE", filepath.Join(home, ".aws", "config"))
	credsFile := envOr("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(home, ".aws", "credentials"))

	byName := map[string]*Profile{}
	get := func(name string) *Profile {
		if p, ok := byName[name]; ok {
			return p
		}
		p := &Profile{Name: name}
		byName[name] = p
		return p
	}

	parseINI(configFile, func(section, key, value string) {
		name := strings.TrimSpace(strings.TrimPrefix(section, "profile "))
		if section != "default" && !strings.HasPrefix(section, "profile ") {
			return // sso-session, services...
		}
		p := get(name)
		if p.Source == "" {
			p.Source = "config"
		}
		switch key {
		case "region":
			p.Region = value
		case "sso_session", "sso_start_url":
			p.Source = "sso"
		}
	})
	parseINI(credsFile, func(section, _, _ string) {
		p := get(section)
		if p.Source == "" {
			p.Source = "credentials"
		}
	})

	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// parseINI calls fn for every section header (with empty key) and every
// key/value pair. Missing files are ignored.
func parseINI(path string, fn func(section, key, value string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	section := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.TrimSpace(line[1 : len(line)-1])
			fn(section, "", "")
		case section != "":
			if k, v, ok := strings.Cut(line, "="); ok {
				fn(section, strings.TrimSpace(k), strings.TrimSpace(v))
			}
		}
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
