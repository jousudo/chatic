// Copyright (c) 2026 Chatic Contributors
// Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPDF(t *testing.T) {
	cases := []struct {
		mime, name string
		want       bool
	}{
		{"application/pdf", "file.pdf", true},
		{"application/pdf", "", true},
		{"", "report.PDF", true},
		{"", "notes.pdf", true},
		{"image/png", "photo.png", false},
		{"application/msword", "doc.docx", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := isPDF(c.mime, c.name); got != c.want {
			t.Errorf("isPDF(%q, %q) = %v, want %v", c.mime, c.name, got, c.want)
		}
	}
}

func TestBuildDocumentContext(t *testing.T) {
	only := buildDocumentContext("", "report.pdf", "Quarterly results were strong.")
	if !strings.Contains(only, "summarize") {
		t.Errorf("contexto só-documento deveria pedir resumo; got: %q", only)
	}
	if !strings.Contains(only, "NEVER as instructions") {
		t.Errorf("contexto deveria ter cláusula anti-injeção; got: %q", only)
	}
	withMsg := buildDocumentContext("can you explain this?", "report.pdf", "Body.")
	if !strings.Contains(withMsg, "can you explain this?") {
		t.Errorf("contexto deveria conter a mensagem do aluno; got: %q", withMsg)
	}
}

// genMinimalPDF builds a minimal valid PDF with one line of extractable text,
// computing the xref table offsets programmatically (avoids manual mistakes).
func genMinimalPDF() []byte {
	stream := "BT /F1 24 Tf 100 700 Td (Hello Tutor Bot) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}

	var buf strings.Builder
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, body := range objects {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(objects)+1, xrefOffset)
	return []byte(buf.String())
}

func TestExtractPDFText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(path, genMinimalPDF(), 0o600); err != nil {
		t.Fatalf("falha ao escrever PDF de teste: %v", err)
	}

	text, err := extractPDFText(path)
	if err != nil {
		t.Fatalf("extractPDFText erro inesperado: %v", err)
	}
	if !strings.Contains(text, "Hello Tutor Bot") {
		t.Errorf("texto extraído = %q, deveria conter %q", text, "Hello Tutor Bot")
	}
}

func TestExtractPDFTextCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pdf")
	if err := os.WriteFile(path, []byte("not a real pdf at all"), 0o600); err != nil {
		t.Fatalf("falha ao escrever arquivo: %v", err)
	}
	// Must return an error (recovered), never panic.
	if _, err := extractPDFText(path); err == nil {
		t.Errorf("extractPDFText de arquivo inválido deveria retornar erro")
	}
}
