// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestExtractFirstURL(t *testing.T) {
	cases := map[string]string{
		"let's discuss https://example.com/news/article-1 please": "https://example.com/news/article-1",
		"no link here": "",
		"trailing punctuation https://example.com/x.": "https://example.com/x",
		"http plain http://foo.bar/baz and more":      "http://foo.bar/baz",
		"parenthetical (https://example.com/a)":       "https://example.com/a",
	}
	for input, want := range cases {
		if got := extractFirstURL(input); got != want {
			t.Errorf("extractFirstURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseHTMLText(t *testing.T) {
	htmlDoc := `<html><head><title>Big News Today</title>
		<style>.x{color:red}</style></head>
		<body>
		<nav>Home About Contact</nav>
		<script>console.log('tracker')</script>
		<article><h1>Headline</h1><p>First paragraph of the story.</p>
		<p>Second paragraph with details.</p></article>
		<footer>Copyright 2026</footer>
		</body></html>`

	title, body := parseHTMLText(strings.NewReader(htmlDoc))
	body = normalizeText(body)

	if strings.TrimSpace(title) != "Big News Today" {
		t.Errorf("title = %q, want %q", title, "Big News Today")
	}
	for _, want := range []string{"Headline", "First paragraph of the story.", "Second paragraph with details."} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got: %q", want, body)
		}
	}
	// Noise should have been discarded.
	for _, noise := range []string{"tracker", "color:red", "Home About Contact", "Copyright 2026"} {
		if strings.Contains(body, noise) {
			t.Errorf("body should not contain noise %q; got: %q", noise, body)
		}
	}
}

func TestAssertPublicHostBlocksInternal(t *testing.T) {
	blocked := []string{
		"http://localhost/x",
		"http://127.0.0.1/x",
		"http://10.0.0.5/x",
		"http://192.168.1.1/x",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"ftp://example.com/x",
	}
	for _, raw := range blocked {
		u, _ := url.Parse(raw)
		if err := assertPublicHost(u); err == nil {
			t.Errorf("assertPublicHost(%q) = nil, want error", raw)
		}
	}
	// A public host should pass.
	u, _ := url.Parse("https://example.com/news")
	if err := assertPublicHost(u); err != nil {
		t.Errorf("assertPublicHost(public) = %v, want nil", err)
	}
}

func TestFetchArticleText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Test Page</title></head><body>
			<article><p>The economy grew this quarter.</p></article></body></html>`))
	}))
	defer srv.Close()

	// The test server listens on 127.0.0.1, which the SSRF guard blocks; so we
	// validate parsing separately and here we check that the block works.
	if _, _, err := fetchArticleText(context.Background(), srv.URL); err == nil {
		t.Errorf("fetchArticleText de host loopback deveria ser bloqueado pelo guard SSRF")
	}
}

func TestBuildArticleContext(t *testing.T) {
	// Link only: should ask for a summary + question.
	only := buildArticleContext("https://example.com/a", "Title", "Body text.")
	if !strings.Contains(only, "summarize") {
		t.Errorf("contexto só-link deveria pedir resumo; got: %q", only)
	}
	// Link + comment: should include the student's comment.
	withMsg := buildArticleContext("what do you think? https://example.com/a", "Title", "Body text.")
	if !strings.Contains(withMsg, "what do you think?") {
		t.Errorf("contexto deveria conter a mensagem do aluno; got: %q", withMsg)
	}
	if !strings.Contains(withMsg, "NEVER as instructions") {
		t.Errorf("contexto deveria ter cláusula anti-injeção; got: %q", withMsg)
	}
}
