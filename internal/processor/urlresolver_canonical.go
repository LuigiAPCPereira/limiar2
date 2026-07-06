package processor

import (
	"errors"
	"net/url"
	"sort"
	"strings"
)

var urlTrackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"fbclid":       true,
	"gclid":        true,
	"msclkid":      true,
	"ref":          true,
	"ref_":         true,
	"referrer":     true,
	"tag":          true,
	"ascsubtag":    true,
	"linkcode":     true,
	"creative":     true,
	"camp":         true,
	"affiliate_id": true,
	"aff_id":       true,
	"mmp_pid":      true,
	"mmp_sub1":     true,
	"mmp_sub2":     true,
	"spm":          true,
	"algo_pvid":    true,
	"algo_exp_id":  true,
	"uls_trackid":  true,
}

// CanonicalizeURL remove fragmentos e parâmetros de tracking/affiliate de uma URL.
// A função é pura e determinística: scheme/host em lowercase e query útil ordenada.
func CanonicalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", &url.Error{Op: "parse", URL: raw, Err: errors.New("missing scheme or host")}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &url.Error{Op: "parse", URL: raw, Err: errors.New("unsupported scheme")}
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""

	query := u.Query()
	clean := make(url.Values, len(query))
	for key, values := range query {
		if urlTrackingParams[strings.ToLower(key)] {
			continue
		}
		copied := append([]string(nil), values...)
		sort.Strings(copied)
		clean[key] = copied
	}
	u.RawQuery = clean.Encode()
	return u.String(), nil
}
