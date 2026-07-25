package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// pathToURI converts a filesystem path to the file:// form servers expect.
// Windows needs the extra care: gopls rejects "file://E:\a\b" outright, and a
// drive letter without the leading slash is silently read as a hostname.
func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed // C:/x -> /C:/x
	}
	u := url.URL{Scheme: "file", Path: slashed}
	return u.String()
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	p := u.Path
	if strings.HasPrefix(p, "/") && len(p) > 2 && p[2] == ':' {
		p = p[1:] // /C:/x -> C:/x
	}
	return filepath.ToSlash(p)
}

func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
