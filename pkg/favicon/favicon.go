package favicon

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ExtractURL finds the best favicon reference in an HTML document and
// resolves it against baseURL. It never performs a network request. When no
// link tag is present, the conventional /favicon.ico location is returned.
func ExtractURL(document, baseURL string) string {
	base, _ := url.Parse(strings.TrimSpace(baseURL))
	// Favicon hints belong in <head>. Bound parsing for pathological HTML
	// documents so report enrichment cannot spend linear time in megabytes
	// of body markup.
	const maxScanBytes = 256 * 1024
	if len(document) > maxScanBytes {
		document = document[:maxScanBytes]
	}
	href := extractHref(document, base)
	if href == "" {
		if base == nil || base.Host == "" {
			return ""
		}
		ref := &url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/favicon.ico"}
		return ref.String()
	}

	return ResolveURL(href, base)
}

// ResolveURL resolves maybeURL against base.
func ResolveURL(maybeURL string, base *url.URL) string {
	maybeURL = strings.TrimSpace(maybeURL)
	if maybeURL == "" {
		return ""
	}

	ref, err := url.Parse(maybeURL)
	if err != nil {
		return ""
	}

	if base != nil {
		return base.ResolveReference(ref).String()
	}

	return ref.String()
}

// NormalizeURL canonicalises a favicon URL for exact matching. Query strings
// and fragments are removed because version/cache-busting parameters commonly
// vary without changing the icon.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	if (parsed.Scheme == "http" && strings.HasSuffix(parsed.Host, ":80")) ||
		(parsed.Scheme == "https" && strings.HasSuffix(parsed.Host, ":443")) {
		parsed.Host = strings.Split(parsed.Host, ":")[0]
	}
	parsed.Host = strings.TrimPrefix(parsed.Host, "www.")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// HashBytes returns the stable content identifier used for favicon clustering.
func HashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func extractHref(document string, base *url.URL) string {
	if strings.TrimSpace(document) == "" {
		return ""
	}

	tokenizer := html.NewTokenizer(strings.NewReader(document))
	best := ""
	bestPriority := 99

	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			break
		}

		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken && tokenType != html.EndTagToken {
			continue
		}

		token := tokenizer.Token()
		if tokenType == html.EndTagToken && token.Data == "head" {
			break
		}
		if token.Data == "body" {
			break
		}
		if token.Data != "link" {
			continue
		}

		attrs := attributeMap(token.Attr)
		rel := strings.ToLower(strings.TrimSpace(attrs["rel"]))
		href := strings.TrimSpace(attrs["href"])
		if href == "" || !strings.Contains(rel, "icon") {
			continue
		}

		priority := iconPriority(rel)
		if priority < bestPriority {
			bestPriority = priority
			best = href
		}
	}

	return best
}

func attributeMap(attrs []html.Attribute) map[string]string {
	result := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		result[strings.ToLower(attr.Key)] = attr.Val
	}
	return result
}

func iconPriority(rel string) int {
	rel = strings.ToLower(rel)
	switch {
	case strings.Contains(rel, "shortcut"):
		return 0
	case rel == "icon" || strings.HasPrefix(rel, "icon "):
		return 1
	case strings.Contains(rel, "apple-touch-icon"):
		return 2
	case strings.Contains(rel, "mask-icon"):
		return 3
	default:
		return 4
	}
}
