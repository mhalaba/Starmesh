package hub

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"rsc.io/qr"
)

//go:embed web/*
var webContent embed.FS

func webFileSystem() http.FileSystem {
	sub, err := fs.Sub(webContent, "web")
	if err != nil {
		return http.FS(webContent)
	}
	return http.FS(sub)
}

func qrSVG(payload string) (string, error) {
	if payload == "" {
		return "", fmt.Errorf("empty")
	}
	code, err := qr.Encode(payload, qr.M)
	if err != nil {
		return "", err
	}
	n := code.Size
	const cell = 8
	dim := (n + 2) * cell
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, dim, dim)
	b.WriteString(`<rect width="100%" height="100%" fill="#fff"/>`)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="#0b0e14"/>`, (x+1)*cell, (y+1)*cell, cell, cell)
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func withToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-Starmesh-Token")
		if got == "" {
			if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
				got = strings.TrimPrefix(a, "Bearer ")
			}
		}
		if got == "" {
			got = r.URL.Query().Get("token")
		}
		if got == "" {
			if c, err := r.Cookie("starmesh_token"); err == nil {
				got = c.Value
			}
		}
		if got != token {
			w.Header().Set("WWW-Authenticate", `Bearer realm="starmesh"`)
			http.Error(w, "token required", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("token") == token {
			http.SetCookie(w, &http.Cookie{
				Name:     "starmesh_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		next.ServeHTTP(w, r)
	})
}
